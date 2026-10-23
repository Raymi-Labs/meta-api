#!/usr/bin/env python3
"""Stream a consistent MySQL dump to crypt storage and publish verified receipts.

No database dump is staged locally. A SUCCESS.json object is the commit point;
objects without it are incomplete and must never be treated as usable backups.
"""
import argparse
import configparser
import contextlib
import datetime as dt
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import uuid
import zlib

import importlib.util

# Load the adjacent optional helper without changing sys.path or running main.
_anchor_spec = importlib.util.spec_from_file_location("newapi_pitr_anchor", Path(__file__).with_name("pitr_anchor.py"))
anchor = importlib.util.module_from_spec(_anchor_spec) if _anchor_spec and _anchor_spec.loader else None
if anchor is not None and Path(_anchor_spec.origin).is_file():
    _anchor_spec.loader.exec_module(anchor)
else:
    anchor = None


BLOCK = 1024 * 1024
NOFOLLOW = getattr(os, "O_NOFOLLOW", 0)


class BackupError(Exception):
    """Only fixed, non-secret diagnostic messages belong in this exception."""


def timestamp():
    return dt.datetime.now(dt.timezone.utc).isoformat()


def safe_path(value):
    path = Path(value)
    if not path.is_absolute() or str(path) != str(value) or ".." in path.parts:
        raise BackupError("paths must be absolute and normalized")
    for part in reversed([path, *path.parents]):
        try:
            info = part.lstat()
        except FileNotFoundError:
            if part != path:
                raise BackupError("path parent does not exist") from None
            continue
        if stat.S_ISLNK(info.st_mode) or info.st_uid not in (0, os.geteuid()):
            raise BackupError("path has a symlink or untrusted owner")
        sticky_parent = part != path and info.st_uid == 0 and info.st_mode & stat.S_ISVTX
        if info.st_mode & 0o022 and not sticky_parent:
            raise BackupError("path is writable by another user")
    return path


def private_bytes(value):
    path = safe_path(value)
    fd = os.open(path, os.O_RDONLY | NOFOLLOW | os.O_CLOEXEC)
    with os.fdopen(fd, "rb") as stream:
        info = os.fstat(stream.fileno())
        if (not stat.S_ISREG(info.st_mode) or info.st_nlink != 1
                or info.st_uid != os.geteuid() or stat.S_IMODE(info.st_mode) != 0o600):
            raise BackupError("configuration and receipts must be owned regular files with mode 0600")
        data = stream.read(BLOCK + 1)
        if len(data) > BLOCK:
            raise BackupError("configuration or receipt is unexpectedly large")
        return data


def private_directory(value):
    path = safe_path(value)
    path.mkdir(mode=0o700, exist_ok=True)
    info = path.lstat()
    if (not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid()
            or stat.S_IMODE(info.st_mode) != 0o700):
        raise BackupError("state directories must be owned directories with mode 0700")
    return path


def json_bytes(value):
    return (json.dumps(value, sort_keys=True, indent=2) + "\n").encode()


def atomic_json(path, value):
    path = safe_path(path)
    if path.exists():
        private_bytes(path)
    fd, temporary = tempfile.mkstemp(prefix=".receipt-", dir=path.parent)
    try:
        with os.fdopen(fd, "wb") as stream:
            stream.write(json_bytes(value))
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
        directory_fd = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(directory_fd)
        finally:
            os.close(directory_fd)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


@contextlib.contextmanager
def locked(value):
    path = safe_path(value)
    fd = os.open(path, os.O_RDWR | os.O_CREAT | NOFOLLOW | os.O_CLOEXEC, 0o600)
    try:
        info = os.fstat(fd)
        if (not stat.S_ISREG(info.st_mode) or info.st_nlink != 1
                or info.st_uid != os.geteuid() or info.st_mode & 0o022):
            raise BackupError("lock file has unsafe ownership, type, or permissions")
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise BackupError("another backup or deployment holds the lock") from None
        current = path.lstat()
        if (current.st_dev, current.st_ino) != (info.st_dev, info.st_ino):
            raise BackupError("lock file changed")
        yield
    finally:
        os.close(fd)


@contextlib.contextmanager
def deadline(seconds):
    def interrupted(signum, _frame):
        raise BackupError("backup exceeded its time limit" if signum == signal.SIGALRM
                          else "backup was interrupted")
    previous = {sig: signal.signal(sig, interrupted)
                for sig in (signal.SIGALRM, signal.SIGTERM, signal.SIGINT)}
    signal.setitimer(signal.ITIMER_REAL, seconds)
    try:
        yield
    finally:
        signal.setitimer(signal.ITIMER_REAL, 0)
        for sig, handler in previous.items():
            signal.signal(sig, handler)


class Processes:
    """Bound each invocation and reap every child even when a pipeline fails."""
    def __init__(self):
        self.children = []

    def start(self, command, **kwargs):
        try:
            child = subprocess.Popen(command, stderr=subprocess.DEVNULL,
                                     start_new_session=True, **kwargs)
        except OSError:
            raise BackupError("required backup program could not be started") from None
        self.children.append(child)
        return child

    def capture(self, command, *, env=None):
        child = self.start(command, stdin=subprocess.DEVNULL,
                           stdout=subprocess.PIPE, env=env)
        data, _ = child.communicate()
        self.check(child, "preflight command failed")
        return data

    @staticmethod
    def check(child, message):
        if child.wait() != 0:
            raise BackupError(message)

    def close(self):
        for child in self.children:
            if child.poll() is None:
                try:
                    os.killpg(child.pid, signal.SIGTERM)
                except ProcessLookupError:
                    pass
        for child in self.children:
            try:
                child.wait(timeout=2)
            except subprocess.TimeoutExpired:
                try:
                    os.killpg(child.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                child.wait()
            for stream in (child.stdin, child.stdout, child.stderr):
                if stream is not None:
                    try:
                        stream.close()
                    except OSError:
                        pass

    def __enter__(self):
        return self

    def __exit__(self, *_exc):
        self.close()


def remote_prefix(value):
    match = re.fullmatch(r"([A-Za-z0-9][A-Za-z0-9_-]*):([A-Za-z0-9_./-]*)", value)
    if (not match or match[2].startswith("/")
            or any(part in (".", "..") for part in match[2].split("/"))):
        raise BackupError("remote must be a configured remote:path without overrides")
    return value.rstrip("/"), match[1]


class CryptRemote:
    def __init__(self, target, config, processes):
        self.prefix, self.name = remote_prefix(target)
        self.config, self.processes = safe_path(config), processes
        self.identity = private_bytes(self.config)
        parser = configparser.ConfigParser(interpolation=None)
        try:
            parser.read_string(self.identity.decode("utf-8"))
            selected = parser[self.name]
        except (ValueError, UnicodeError, KeyError, configparser.Error):
            raise BackupError("rclone configuration is not a readable plain INI configuration") from None
        if (selected.get("type") != "crypt" or not selected.get("password")
                or not selected.get("remote")):
            raise BackupError("destination must be a configured crypt remote with a password")
        if selected.get("no_data_encryption", "false").lower() not in ("false", "0", ""):
            raise BackupError("crypt data encryption must be enabled")
        # rclone can execute external programs for some backends/options. This
        # tool supports a crypt wrapper over an existing S3 or local test remote.
        _, backing = remote_prefix(selected["remote"])
        if backing not in parser or parser[backing].get("type") not in ("s3", "local"):
            raise BackupError("crypt backing remote must be configured S3 or local storage")
        for section in parser.sections():
            if any(key in parser[section] for key in ("password_command", "config_pass_command", "command")):
                raise BackupError("executable rclone configuration options are not supported")
        self.env = {key: value for key, value in os.environ.items()
                    if not key.upper().startswith("RCLONE_")}

    def check_config(self):
        if private_bytes(self.config) != self.identity:
            raise BackupError("rclone configuration changed during backup")

    def command(self, *args):
        self.check_config()
        return ["rclone", "--config", str(self.config), "--ask-password=false",
                "--log-level", "ERROR", "--contimeout", "15s", "--timeout", "60s",
                "--retries", "1", "--low-level-retries", "1", *args]

    def writer(self, destination):
        return self.processes.start(self.command("rcat", destination, "--immutable"),
                                    stdin=subprocess.PIPE, stdout=subprocess.DEVNULL, env=self.env)

    def reader(self, destination):
        return self.processes.start(self.command("cat", destination),
                                    stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, env=self.env)

    def put_verified(self, destination, data):
        writer = self.writer(destination)
        try:
            writer.communicate(data)
        except BrokenPipeError:
            raise BackupError("metadata upload failed") from None
        self.processes.check(writer, "metadata upload failed")
        reader = self.reader(destination)
        received = reader.stdout.read(len(data) + 1)
        if received != data:
            raise BackupError("metadata readback did not match")
        self.processes.check(reader, "metadata readback failed")
        self.check_config()

    def verify_dump(self, destination, expected):
        reader = self.reader(destination)
        actual = gzip_digest(reader.stdout, expected["size"])
        self.processes.check(reader, "database backup readback failed")
        self.check_config()
        if actual != expected:
            raise BackupError("database backup readback hash or size did not match")


def gzip_digest(stream, maximum):
    digest, size = hashlib.sha256(), 0
    decoder = zlib.decompressobj(16 + zlib.MAX_WBITS)
    try:
        while True:
            block = stream.read(BLOCK)
            if not block:
                break
            size += len(block)
            if size > maximum:
                raise BackupError("database backup readback exceeds the uploaded size")
            digest.update(block)
            if decoder.eof:
                raise BackupError("gzip contains trailing bytes or multiple members")
            # Bound decompression memory even for highly compressible SQL.
            while block:
                decoder.decompress(block, BLOCK)
                if decoder.unused_data:
                    raise BackupError("gzip contains trailing bytes or multiple members")
                block = decoder.unconsumed_tail
        if not decoder.eof or size == 0:
            raise BackupError("gzip backup is truncated or empty")
    except zlib.error:
        raise BackupError("gzip backup integrity verification failed") from None
    return {"sha256": digest.hexdigest(), "size": size}


def mysql_command(args, program, options):
    if getattr(args, 'pitr_anchor', False):
        options = ['--protocol=SOCKET', *options]
    command = ('MYSQL_PWD="${MYSQL_ROOT_PASSWORD:?}" exec ' + program
               + " --user=root " + " ".join(shlex.quote(option) for option in options))
    return ["docker", "exec", args.container, "sh", "-c", command]


def preflight(args, processes):
    if shutil.disk_usage(args.disk_path).free < args.min_free_bytes:
        raise BackupError("insufficient free disk space to start a database snapshot")
    query = ("SELECT VERSION(); SELECT COUNT(*),COALESCE(SUM(ENGINE <> 'InnoDB' OR ENGINE IS NULL),0) "
             "FROM information_schema.TABLES WHERE TABLE_SCHEMA='" + args.database
             + "' AND TABLE_TYPE='BASE TABLE';")
    data = processes.capture(mysql_command(args, "mysql", ["--batch", "--skip-column-names",
                                                         "--execute=" + query]))
    try:
        version, counts = data.decode("ascii").strip().splitlines()
        total, non_innodb = map(int, counts.split("\t"))
    except (ValueError, UnicodeError):
        raise BackupError("MySQL preflight output was not recognized") from None
    if not re.fullmatch(r"[0-9][A-Za-z0-9._+-]{0,100}", version):
        raise BackupError("MySQL version output was not recognized")
    if total <= 0 or non_innodb != 0:
        raise BackupError("database must exist and all base tables must use InnoDB")
    result = {"mysql_version": version, "database": args.database, "base_table_count": total,
            "consistency": "single-transaction; all base tables checked as InnoDB",
            "binlog_coordinates": "not collected; PITR chain not verified"}
    if args.pitr_anchor:
        if not re.fullmatch(r"8\.0\.(?:[3-9][0-9]|2[6-9])(?:[.-][A-Za-z0-9_.-]+)?", version):
            raise BackupError("PITR anchors require a tested MySQL 8.0.26 or newer 8.0 client/server")
        query = ("SELECT @@server_uuid,@@log_bin,@@binlog_format,@@binlog_row_image,"
                 "@@gtid_mode,@@sync_binlog,@@innodb_flush_log_at_trx_commit,"
                 "@@binlog_checksum,@@binlog_expire_logs_seconds,@@server_id,"
                 "@@performance_schema,@@performance_schema_session_connect_attrs_size,"
                 "@@binlog_transaction_compression,@@binlog_encryption; "
                 "SELECT COUNT(*) FROM performance_schema.setup_instruments WHERE "
                 "NAME='wait/lock/metadata/sql/mdl' AND ENABLED='YES'; "
                 "SELECT COUNT(*) FROM performance_schema.session_connect_attrs WHERE "
                 "PROCESSLIST_ID=CONNECTION_ID() AND ATTR_NAME='_pid' AND ATTR_VALUE REGEXP '^[1-9][0-9]*$'; "
                 "SELECT SCHEMA_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME NOT IN "
                 "('mysql','information_schema','performance_schema','sys') ORDER BY SCHEMA_NAME;")
        data = processes.capture(mysql_command(args, "mysql", ["--batch", "--skip-column-names", "--execute=" + query]))
        try:
            rows = data.decode("ascii").strip().splitlines()
            fields = rows[0].split("\t")
            keys = ("server_uuid", "log_bin", "binlog_format", "binlog_row_image", "gtid_mode",
                    "sync_binlog", "innodb_flush_log_at_trx_commit", "binlog_checksum", "binlog_expire_logs_seconds", "server_id",
                    "performance_schema", "performance_schema_session_connect_attrs_size", "binlog_transaction_compression", "binlog_encryption")
            if len(fields) != len(keys) or rows[1:3] != ["1", "1"] or rows[3:] != [args.database]:
                raise ValueError()
            values = dict(zip(keys, fields))
            if not re.fullmatch(r"[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}", values["server_uuid"]):
                raise ValueError()
            for key in ("log_bin", "sync_binlog", "innodb_flush_log_at_trx_commit", "binlog_expire_logs_seconds", "server_id",
                        "performance_schema", "performance_schema_session_connect_attrs_size", "binlog_transaction_compression", "binlog_encryption"):
                values[key] = int(values[key])
        except (ValueError, UnicodeError, IndexError):
            raise BackupError("PITR source metadata or database scope is unsupported") from None
        if (values["log_bin"] != 1 or values["binlog_format"] != "ROW" or values["binlog_row_image"] != "FULL"
                or values["sync_binlog"] != 1 or values["innodb_flush_log_at_trx_commit"] != 1
                or values["gtid_mode"] != "OFF" or values["binlog_checksum"] != "CRC32"
                or values["binlog_transaction_compression"] != 0 or values["binlog_encryption"] != 0
                or values["performance_schema"] != 1 or values["performance_schema_session_connect_attrs_size"] <= 0):
            raise BackupError("PITR requires ROW/FULL CRC32 binlogs, stable GTID mode and durable commits")
        client = processes.capture(["docker", "exec", args.container, "mysqldump", "--version"]).decode("ascii")
        if "Ver " + version.split("-")[0] + " " not in client:
            raise BackupError("PITR mysqldump version must match the source MySQL version")
        result.update(values, database_scope=[args.database], ddl_protection="LOCK INSTANCE FOR BACKUP",
                      metadata_lock_instrument_enabled=True, session_connect_pid_visible=True)
        result.pop("binlog_coordinates")
    return result


def dump_command(args, run_id, options):
    if args.pitr_anchor:
        return ["docker", "exec", args.container, "bash", "-c",
                anchor.dump_script(run_id, options, args.dump_timeout_seconds)]
    # timeout owns a process group; the private run directory identifies only
    # this invocation when an interrupted Docker client leaves exec running.
    directory = "/tmp/newapi-r2-backup-" + run_id
    command = ("NEWAPI_BACKUP_RUN=" + run_id + "\n"
               "set -u\n"
               "umask 077\n"
               "run_dir=" + shlex.quote(directory) + "\n"
               "[ -d \"$run_dir\" ] && [ ! -L \"$run_dir\" ] || exit 70\n"
               "child=\n"
               "cleanup() {\n"
               "  if [ -n \"$child\" ]; then\n"
               "    kill -TERM -- \"-$child\" 2>/dev/null || kill -TERM \"$child\" 2>/dev/null || :\n"
               "  fi\n"
               "  rm -f -- \"$run_dir/pids\" \"$run_dir/cancelled\"; rmdir -- \"$run_dir\" 2>/dev/null || :\n"
               "}\n"
               "trap cleanup EXIT\n"
               "trap 'exit 143' TERM INT\n"
               'MYSQL_PWD="${MYSQL_ROOT_PASSWORD:?}" timeout --signal=TERM --kill-after=10s '
               + str(args.dump_timeout_seconds) + "s mysqldump --user=root "
               + " ".join(shlex.quote(option) for option in options) + " &\n"
               "child=$!\n"
               "printf '%s %s\\n' \"$$\" \"$child\" > \"$run_dir/pids\"\n"
               "[ ! -e \"$run_dir/cancelled\" ] || exit 143\n"
               "wait \"$child\"\n"
               "result=$?\n"
               "child=\n"
               "exit \"$result\"\n")
    return ["docker", "exec", args.container, "bash", "-c", command]


def cancel_dump(args, run_id):
    """Cancel only the wrapper whose /proc command line contains our run ID."""
    if args.pitr_anchor:
        try:
            result = subprocess.run(["docker", "exec", args.container, "bash", "-c", anchor.cancel_script(run_id)],
                                    stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                                    stderr=subprocess.DEVNULL, timeout=15, check=False)
            if result.returncode != 0:
                raise BackupError("PITR dump SQL connection cancellation could not be confirmed")
        except (OSError, subprocess.TimeoutExpired):
            raise BackupError("PITR dump SQL connection cancellation could not be confirmed") from None
        return
    directory = "/tmp/newapi-r2-backup-" + run_id
    command = ("run_dir=" + shlex.quote(directory) + "\n"
               '[ -d "$run_dir" ] && [ ! -L "$run_dir" ] || exit 0\n'
               'umask 077; : > "$run_dir/cancelled" || exit 74\n'
               '[ -r "$run_dir/pids" ] || exit 0\n'
               'read -r wrapper child < "$run_dir/pids" || exit 71\n'
               'case "$wrapper:$child" in *[!0-9:]*|:*) exit 72;; esac\n'
               '[ -r "/proc/$wrapper/cmdline" ] || exit 0\n'
               'case "$(tr \'\\0\' \' \' < "/proc/$wrapper/cmdline")" in\n'
               "  *NEWAPI_BACKUP_RUN=" + run_id + "*) ;;\n"
               '  *) exit 73;;\n'
               'esac\n'
               'kill -TERM -- "-$child" 2>/dev/null || kill -TERM "$child" 2>/dev/null || :\n'
               'kill -TERM "$wrapper" 2>/dev/null || :\n'
               'for attempt in {1..20}; do\n'
               '  kill -0 -- "-$child" 2>/dev/null || exit 0\n'
               '  sleep 0.2\n'
               'done\n'
               'kill -KILL -- "-$child" 2>/dev/null || :\n')
    try:
        # This independent bound still applies after the job deadline fired.
        result = subprocess.run(["docker", "exec", args.container, "bash", "-c", command],
                                stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                                stderr=subprocess.DEVNULL, timeout=15, check=False)
        if result.returncode != 0:
            raise BackupError("container dump cancellation could not be confirmed; container timeout remains active")
    except (OSError, subprocess.TimeoutExpired):
        raise BackupError("container dump cancellation could not be confirmed; container timeout remains active") from None


def stream_dump(args, remote, destination, processes, run_id, source=None, instance_lock=None):
    options = ["--single-transaction", "--quick", "--routines", "--triggers", "--events",
               "--hex-blob", "--set-gtid-purged=OFF",
               "--no-tablespaces", "--default-character-set=utf8mb4", "--databases", args.database]
    if args.pitr_anchor:
        options.insert(0, "--source-data=2")
        return stream_anchored_dump(args, remote, destination, processes, run_id, options, source, instance_lock)
    processes.capture(["docker", "exec", args.container, "mkdir", "-m", "700", "--",
                       "/tmp/newapi-r2-backup-" + run_id])
    digest, size = hashlib.sha256(), 0
    finished = False
    try:
        dump = processes.start(dump_command(args, run_id, options),
                               stdin=subprocess.DEVNULL, stdout=subprocess.PIPE)
        compressed = processes.start(["gzip", "-1", "-n", "-c"], stdin=dump.stdout,
                                     stdout=subprocess.PIPE)
        dump.stdout.close()
        upload = remote.writer(destination)
        while True:
            if shutil.disk_usage(args.disk_path).free < args.stop_free_bytes:
                raise BackupError("free disk space fell below the snapshot low-water mark")
            block = compressed.stdout.read(BLOCK)
            if not block:
                break
            upload.stdin.write(block)
            digest.update(block)
            size += len(block)
        upload.stdin.close()
        # Every pipeline stage must succeed. A valid gzip of a failed dump is unsafe.
        results = [(child.wait(), message) for child, message in (
            (dump, "mysqldump failed"), (compressed, "gzip compression failed"),
            (upload, "database backup upload failed"))]
        for status, message in results:
            if status:
                raise BackupError(message)
        finished = True
    except (BrokenPipeError, OSError):
        raise BackupError("database backup streaming upload failed") from None
    finally:
        if not finished:
            cancel_dump(args, run_id)
    remote.check_config()
    if size == 0:
        raise BackupError("database backup upload was empty")
    return {"sha256": digest.hexdigest(), "size": size}


def stream_anchored_dump(args, remote, destination, processes, run_id, options, source, instance_lock):
    directory = "/tmp/newapi-r2-backup-" + run_id
    processes.capture(["docker", "exec", args.container, "mkdir", "-m", "700", "--", directory])
    parser, compressor = anchor.Header(), zlib.compressobj(1, zlib.DEFLATED, 31)
    digest, size, finished = hashlib.sha256(), 0, False
    try:
        dump = processes.start(dump_command(args, run_id, options), stdin=subprocess.DEVNULL, stdout=subprocess.PIPE)
        upload = remote.writer(destination)
        while True:
            instance_lock.check()
            if shutil.disk_usage(args.disk_path).free < args.stop_free_bytes:
                raise BackupError("free disk space fell below the snapshot low-water mark")
            block = dump.stdout.read(BLOCK)
            if not block:
                break
            parser.feed(block)
            compressed = compressor.compress(block)
            if compressed:
                upload.stdin.write(compressed)
                digest.update(compressed)
                size += len(compressed)
        final = compressor.flush()
        upload.stdin.write(final)
        digest.update(final)
        size += len(final)
        upload.stdin.close()
        processes.check(dump, "PITR mysqldump failed or its startup guard expired")
        instance_lock.check()
        value = parser.finish()
        elapsed = processes.capture(["docker", "exec", args.container, "cat", directory + "/ready"])
        if not re.fullmatch(rb"[0-9]+\n", elapsed) or not 0 < int(elapsed) <= 5000000000:
            raise BackupError("PITR startup lock window exceeded five seconds")
        value.update(server_uuid=source["server_uuid"],
                     method="mysqldump --single-transaction --source-data=2",
                     startup_lock_window_upper_bound_seconds=int(elapsed) / 1000000000)
        source["binlog_anchor"] = value
        # The consistent read is now complete; release DDL protection before
        # waiting for the uploader's completion or starting remote readback.
        instance_lock.close()
        finished = True
        processes.check(upload, "database backup upload failed")
    except (anchor.AnchorError, BrokenPipeError, OSError) as exc:
        raise BackupError(str(exc) if isinstance(exc, anchor.AnchorError) else "PITR backup streaming failed") from None
    finally:
        if not finished:
            cancel_dump(args, run_id)
        # Only ephemeral coordination files are removed, never a database file.
        subprocess.run(["docker", "exec", args.container, "rm", "-rf", "--", directory],
                       stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL, timeout=5, check=False)
    return {"sha256": digest.hexdigest(), "size": size}


def require_root():
    if os.geteuid() != 0:
        raise BackupError("run requires root on the MySQL Docker host")


def execute(args):
    if args.action == "run":
        require_root()
    with deadline(args.timeout_seconds), Processes() as processes:
        remote = CryptRemote(args.remote, args.rclone_config, processes)
        if args.action == "preview":
            return {"action": "preview", "remote": remote.prefix, "database": args.database,
                    "writes": False, "scope": "database-only", "pitr_verified": False}
        with locked(args.job_lock):
            state = private_directory(args.state_dir)
            now = dt.datetime.now(dt.timezone.utc)
            run_id = now.strftime("%Y%m%dT%H%M%SZ-") + uuid.uuid4().hex
            directory = private_directory(state / run_id)
            receipt_path = directory / "receipt.json"
            destination = (remote.prefix + ("" if remote.prefix.endswith(":") else "/")
                           + now.strftime("%Y/%m/") + run_id)
            receipt = {"format_version": 1, "run_id": run_id, "status": "started",
                       "started_at": timestamp(), "destination": destination,
                       "scope": "database-only", "pitr_verified": False}
            atomic_json(receipt_path, receipt)
            try:
                if args.pitr_anchor:
                    instance_lock = anchor.InstanceLock(args.container)
                    try:
                        with locked(args.release_lock):
                            instance_lock.acquire()
                        receipt["stage"] = "dump-and-upload"
                        receipt["source"] = preflight(args, processes)
                        atomic_json(receipt_path, receipt)
                        receipt["dump"] = stream_dump(args, remote, destination + "/mysql.sql.gz", processes, run_id,
                                                      receipt["source"], instance_lock)
                    except anchor.AnchorError as exc:
                        raise BackupError(str(exc)) from None
                    finally:
                        try:
                            instance_lock.close()
                        except anchor.AnchorError as exc:
                            raise BackupError(str(exc)) from None
                else:
                    with locked(args.release_lock):
                        receipt["stage"] = "dump-and-upload"
                        receipt["source"] = preflight(args, processes)
                        atomic_json(receipt_path, receipt)
                        receipt["dump"] = stream_dump(args, remote, destination + "/mysql.sql.gz", processes, run_id)
                receipt["stage"] = "readback"
                atomic_json(receipt_path, receipt)
                remote.verify_dump(destination + "/mysql.sql.gz", receipt["dump"])
                receipt["verified_at"] = timestamp()
                receipt["stage"] = "manifest"
                atomic_json(receipt_path, receipt)
                manifest = {key: value for key, value in receipt.items() if key not in ("status", "stage")}
                manifest["dump"]["name"] = "mysql.sql.gz"
                manifest["finished_at"] = timestamp()
                manifest_data = json_bytes(manifest)
                remote.put_verified(destination + "/manifest.json", manifest_data)
                receipt["stage"] = "completion-marker"
                atomic_json(receipt_path, receipt)
                marker = {"format_version": 1, "run_id": run_id, "status": "success",
                          "manifest_sha256": hashlib.sha256(manifest_data).hexdigest(),
                          "verified_at": receipt["verified_at"], "finished_at": timestamp()}
                remote.put_verified(destination + "/SUCCESS.json", json_bytes(marker))
                receipt.update(status="success", stage="complete", finished_at=marker["finished_at"])
                atomic_json(receipt_path, receipt)
                atomic_json(state / "latest-success.json", receipt)
                return receipt
            except BaseException as exc:
                receipt.update(status="failed", failed_at=timestamp(),
                               error=str(exc) if isinstance(exc, BackupError) else "backup operation failed")
                atomic_json(receipt_path, receipt)
                raise


def parse_args(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("preview", "run"), nargs="?", default="preview")
    parser.add_argument("--rclone-config", default="/root/.config/rclone/rclone.conf")
    parser.add_argument("--remote", default="r2-backup:full")
    parser.add_argument("--state-dir", default="/var/lib/newapi-r2-backup")
    parser.add_argument("--release-lock", default="/run/newapi-release.lock")
    parser.add_argument("--job-lock", default="/run/newapi-r2-backup.lock")
    parser.add_argument("--container", default="newapi-mysql")
    parser.add_argument("--database", default="new-api")
    parser.add_argument("--timeout-seconds", type=int, default=10200)
    parser.add_argument("--dump-timeout-seconds", type=int, default=2700)
    parser.add_argument("--disk-path", default="/opt/newapi")
    parser.add_argument("--min-free-bytes", type=int, default=5 * 1024 ** 3)
    parser.add_argument("--stop-free-bytes", type=int, default=3 * 1024 ** 3)
    parser.add_argument("--pitr-anchor", action="store_true", help="opt in to a guarded MySQL 8.0 binlog snapshot anchor")
    args = parser.parse_args(argv)
    if args.pitr_anchor and anchor is None:
        parser.error("PITR mode requires the adjacent pitr_anchor.py helper")
    for value in (args.container, args.database):
        if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_-]*", value):
            parser.error("container and database must be simple identifiers")
    if args.timeout_seconds <= 0 or args.dump_timeout_seconds <= 0:
        parser.error("timeouts must be positive")
    if not 0 < args.stop_free_bytes <= args.min_free_bytes:
        parser.error("disk thresholds must be positive; stop threshold must not exceed start threshold")
    return args


def main(argv=None):
    try:
        result = execute(parse_args(argv))
        print(json.dumps(result, sort_keys=True))
        return 0
    except BackupError as exc:
        print("backup failed: " + str(exc), file=sys.stderr)
    except (OSError, ValueError, subprocess.SubprocessError):
        print("backup failed: operating system or subprocess operation failed", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
