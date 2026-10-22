#!/usr/bin/env python3
"""Preview, encrypt/archive, or explicitly retire closed historical text logs."""
import argparse
import contextlib
import datetime as dt
import fcntl
import gzip
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import tempfile
import time
import zlib


NAME = re.compile(r"oneapi-[A-Za-z0-9_.-]+\.log\.gz\Z")
HASH = re.compile(r"[0-9a-f]{64}\Z")
DAY_NS = 86400 * 1_000_000_000
NOFOLLOW = getattr(os, "O_NOFOLLOW", 0)
FP_KEYS = ("dev", "ino", "size", "mtime_ns")
PROC = Path("/proc")


class OffloadError(Exception):
    pass


def timestamp():
    return dt.datetime.now(dt.timezone.utc).isoformat()


def safe_path(value):
    """Reject symlinks and writable ancestors instead of resolving through them."""
    path = Path(value)
    if not path.is_absolute() or str(path) != str(value) or ".." in path.parts:
        raise OffloadError("paths must be absolute, normalized, and contain no '..'")
    for part in reversed([path, *path.parents]):
        try:
            info = part.lstat()
        except FileNotFoundError:
            if part != path:
                raise OffloadError("path parent does not exist") from None
            continue
        if stat.S_ISLNK(info.st_mode):
            raise OffloadError("symlink paths are not allowed")
        if info.st_uid not in (0, os.geteuid()):
            raise OffloadError("path has an untrusted owner")
        # A root-owned sticky temporary directory cannot replace our owned child.
        sticky_parent = part != path and info.st_uid == 0 and info.st_mode & stat.S_ISVTX
        if info.st_mode & 0o022 and not sticky_parent:
            raise OffloadError("path is writable by another user")
    return path


def private_file(path):
    safe_path(path)
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1 or info.st_uid != os.geteuid():
        raise OffloadError("state/config must be an owned, unlinked regular file")
    if stat.S_IMODE(info.st_mode) != 0o600:
        raise OffloadError("state/config files must have mode 0600")


@contextlib.contextmanager
def locked(path, private=False):
    safe_path(path)
    fd = os.open(path, os.O_RDWR | os.O_CREAT | NOFOLLOW | os.O_CLOEXEC, 0o600)
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1 or info.st_uid != os.geteuid():
            raise OffloadError("unsafe lock file")
        if info.st_mode & 0o022 or (private and stat.S_IMODE(info.st_mode) != 0o600):
            raise OffloadError("unsafe lock permissions")
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise OffloadError("another deployment/maintenance operation holds the lock") from None
        current = Path(path).lstat()
        if (current.st_dev, current.st_ino) != (info.st_dev, info.st_ino):
            raise OffloadError("lock path changed")
        yield
    finally:
        os.close(fd)


def fingerprint(info):
    return dict(zip(FP_KEYS, (info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns)))


def eligible(name, info, now_ns, retention_days):
    return (bool(NAME.fullmatch(name)) and stat.S_ISREG(info.st_mode)
            and info.st_nlink == 1 and info.st_uid == os.geteuid()
            and not info.st_mode & 0o022
            and info.st_mtime_ns < now_ns - retention_days * DAY_NS)


def ensure_closed(info, ignored_fd=None):
    """Fail closed unless every live process's file descriptors can be inspected."""
    proc = PROC
    if not (proc / "self/fd").is_dir():
        raise OffloadError("Linux host /proc is required to verify closed files")
    try:
        for process in proc.iterdir():
            if not process.name.isdigit():
                continue
            try:
                for descriptor in (process / "fd").iterdir():
                    if int(process.name) == os.getpid() and descriptor.name == str(ignored_fd):
                        continue
                    try:
                        opened = descriptor.stat()
                    except (FileNotFoundError, ProcessLookupError):
                        continue
                    if (opened.st_dev, opened.st_ino) == (info.st_dev, info.st_ino):
                        raise OffloadError("source file is open by a process")
            except (FileNotFoundError, ProcessLookupError):
                continue
    except PermissionError:
        raise OffloadError("cannot inspect every process; refusing to assume file is closed") from None


class DigestReader:
    def __init__(self, stream, maximum):
        self.stream, self.maximum = stream, maximum
        self.digest, self.size = hashlib.sha256(), 0

    def read(self, size=-1):
        data = self.stream.read(size)
        self.size += len(data)
        if self.size > self.maximum:
            raise OffloadError("file grew or remote content exceeds the recorded size")
        self.digest.update(data)
        return data


def gzip_digest(stream, size):
    reader = DigestReader(stream, size)
    try:
        with gzip.GzipFile(fileobj=reader, mode="rb") as contents:
            while contents.read(1024 * 1024):
                pass
    except (OSError, EOFError, zlib.error):
        raise OffloadError("gzip integrity verification failed") from None
    if reader.size != size or size == 0:
        raise OffloadError("compressed file size does not match")
    return reader.digest.hexdigest()


def remote_prefix(value):
    match = re.fullmatch(r"([A-Za-z0-9][A-Za-z0-9_-]*):([A-Za-z0-9_./-]*)", value or "")
    if not match or match[2].startswith("/") or any(p in (".", "..") for p in match[2].split("/")):
        raise OffloadError("remote must be a configured remote:path without traversal or overrides")
    return value.rstrip("/"), match[1]


class CryptRemote:
    def __init__(self, target, config):
        self.prefix, self.name = remote_prefix(target)
        if not config:
            raise OffloadError("archive/retire require --rclone-config")
        self.config = safe_path(config)
        private_file(self.config)
        self.config_identity = self.config_fingerprint()
        # Do not let RCLONE_CONFIG_<REMOTE>_TYPE or options silently bypass crypt.
        self.env = {k: v for k, v in os.environ.items() if not k.upper().startswith("RCLONE_")}
        result = self.run("config", "dump", capture=True)
        try:
            settings = json.loads(result.stdout)
        except (ValueError, UnicodeError):
            raise OffloadError("rclone configuration could not be inspected") from None
        selected = settings.get(self.name, {}) if isinstance(settings, dict) else {}
        if (not isinstance(selected, dict) or selected.get("type") != "crypt"
                or not selected.get("password") or not selected.get("remote")):
            raise OffloadError("destination must be a configured crypt remote with a password and backing remote")
        if str(selected.get("no_data_encryption", "false")).lower() not in ("false", "0", ""):
            raise OffloadError("crypt data encryption must be enabled")

    def config_fingerprint(self):
        private_file(self.config)
        with self.config.open("rb") as stream:
            info = os.fstat(stream.fileno())
            digest = hashlib.sha256(stream.read()).hexdigest()
        return fingerprint(info), digest

    def check_config(self):
        if self.config_fingerprint() != self.config_identity:
            raise OffloadError("rclone configuration changed during operation")

    def command(self, *args):
        return ["rclone", "--config", str(self.config), "--ask-password=false",
                "--log-level", "ERROR", "--contimeout", "15s", "--timeout", "60s",
                "--retries", "1", "--low-level-retries", "1", *args]

    def run(self, *args, capture=False):
        self.check_config()
        try:
            result = subprocess.run(self.command(*args), stdin=subprocess.DEVNULL,
                                    stdout=subprocess.PIPE if capture else subprocess.DEVNULL,
                                    stderr=subprocess.DEVNULL, env=self.env,
                                    timeout=60 if capture else None, check=False)
        except (OSError, subprocess.TimeoutExpired):
            raise OffloadError("rclone invocation failed; source files were retained") from None
        self.check_config()
        if result.returncode:
            # rclone diagnostics can include credentials/backend URLs: never echo them.
            raise OffloadError("rclone operation failed; source files were retained")
        return result

    def destination(self, name, digest):
        return self.prefix + ("" if self.prefix.endswith(":") else "/") + digest + "/" + name

    def upload(self, source, destination):
        self.run("copyto", str(source), destination, "--immutable", "--transfers", "1", "--checkers", "1")

    def verify(self, destination, expected):
        self.check_config()
        try:
            process = subprocess.Popen(self.command("cat", destination), stdin=subprocess.DEVNULL,
                                       stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, env=self.env)
        except OSError:
            raise OffloadError("rclone read-back failed; source files were retained") from None
        try:
            digest = gzip_digest(process.stdout, expected["size"])
            if process.wait(timeout=90) != 0 or digest != expected["sha256"]:
                raise OffloadError("decrypted remote content failed SHA-256 verification")
        finally:
            process.stdout.close()
            if process.poll() is None:
                process.kill()
            process.wait()
        self.check_config()


class Offloader:
    def __init__(self, args, now_ns=None):
        if args.retention_days < 30:
            raise OffloadError("phase one requires a retention period of at least 30 days")
        self.root = safe_path(args.root)
        if not self.root.is_dir():
            raise OffloadError("log root is not a directory")
        self.state = safe_path(args.state_dir)
        if self.state == self.root or self.root in self.state.parents or self.state in self.root.parents:
            raise OffloadError("state and source directories must be separate")
        self.release_lock = safe_path(args.release_lock)
        self.days = args.retention_days
        self.now_ns = time.time_ns() if now_ns is None else now_ns
        self.args = args

    def candidates(self):
        result = []
        for path in sorted(self.root.iterdir()):
            info = path.lstat()
            if NAME.fullmatch(path.name) and stat.S_ISLNK(info.st_mode):
                raise OffloadError("symlink source files are not allowed")
            if eligible(path.name, info, self.now_ns, self.days):
                result.append((path.name, info))
        return result

    def local_fingerprint(self, name, expected=None):
        path = self.root / name
        with os.fdopen(os.open(path, os.O_RDONLY | NOFOLLOW | os.O_CLOEXEC), "rb") as stream:
            before = os.fstat(stream.fileno())
            if not eligible(name, before, self.now_ns, self.days):
                raise OffloadError("source no longer satisfies scope/age/ownership requirements")
            ensure_closed(before, stream.fileno())
            result = fingerprint(before)
            result["sha256"] = gzip_digest(stream, before.st_size)
            if (fingerprint(os.fstat(stream.fileno())) != fingerprint(before)
                    or fingerprint(path.lstat()) != fingerprint(before)):
                raise OffloadError("source changed during local verification")
            if expected is not None and result != expected:
                raise OffloadError("source fingerprint changed; retaining local file")
            return result

    def prepare_state(self):
        with locked(self.release_lock):
            if not self.state.exists():
                self.state.mkdir(mode=0o700)
            info = self.state.lstat()
            if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid() or stat.S_IMODE(info.st_mode) != 0o700:
                raise OffloadError("state directory must be owned and have mode 0700")

    def receipt_id(self, receipt):
        identity = {key: receipt[key] for key in ("root", "name", "fingerprint", "remote")}
        return hashlib.sha256(json.dumps(identity, sort_keys=True).encode()).hexdigest()

    def receipt_path(self, receipt):
        return self.state / (self.receipt_id(receipt) + ".json")

    def save(self, receipt):
        """Caller holds both task and release locks; fsync precedes each transition."""
        path = self.receipt_path(receipt)
        if path.exists() or path.is_symlink():
            private_file(path)
        fd, temporary = tempfile.mkstemp(prefix=".receipt-", dir=self.state)
        try:
            with os.fdopen(fd, "w") as stream:
                json.dump(receipt, stream, sort_keys=True, indent=2)
                stream.write("\n")
                stream.flush()
                os.fsync(stream.fileno())
            os.replace(temporary, path)
            self.sync_directory(self.state)
        finally:
            if os.path.exists(temporary):
                os.unlink(temporary)

    @staticmethod
    def sync_directory(directory):
        fd = os.open(directory, os.O_RDONLY | os.O_DIRECTORY | NOFOLLOW)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)

    def read(self, path, remote):
        private_file(path)
        try:
            receipt = json.loads(path.read_text())
            fp = receipt["fingerprint"]
            valid = (receipt["version"] == 1 and receipt["status"] in ("pending", "verified", "retiring", "retired")
                     and receipt["root"] == str(self.root) and isinstance(receipt["name"], str)
                     and NAME.fullmatch(receipt["name"]) and set(fp) == {*FP_KEYS, "sha256"}
                     and all(type(fp[key]) is int and fp[key] >= 0 for key in FP_KEYS)
                     and isinstance(fp["sha256"], str) and HASH.fullmatch(fp["sha256"])
                     and receipt["remote"] == remote.destination(receipt["name"], fp["sha256"])
                     and path == self.receipt_path(receipt)
                     and (receipt["status"] == "pending" or isinstance(receipt["verified_at"], str)))
        except (ValueError, KeyError, TypeError):
            valid = False
        if not valid:
            raise OffloadError("invalid or out-of-scope receipt; refusing retirement")
        return receipt

    def archive(self, remote):
        count = 0
        for name, _ in self.candidates():
            fp = self.local_fingerprint(name)
            receipt = {"version": 1, "status": "pending", "root": str(self.root), "name": name,
                       "fingerprint": fp, "remote": remote.destination(name, fp["sha256"]), "created_at": timestamp()}
            path = self.receipt_path(receipt)
            if path.exists() or path.is_symlink():
                receipt = self.read(path, remote)
                if receipt["status"] in ("retiring", "retired"):
                    raise OffloadError("receipt already entered retirement; use retire to reconcile it")
            else:
                with locked(self.release_lock):
                    self.local_fingerprint(name, fp)
                    self.save(receipt)
            # Neither network transfer nor decrypted read-back holds the release lock.
            if receipt["status"] == "pending":
                remote.upload(self.root / name, receipt["remote"])
            remote.verify(receipt["remote"], fp)
            with locked(self.release_lock):
                self.local_fingerprint(name, fp)
                receipt.update(status="verified", verified_at=timestamp())
                self.save(receipt)
            print(f"verified {name} bytes={fp['size']} receipt={path.name}", flush=True)
            count += 1
        return count

    def retire(self, remote):
        # Validate all receipts before the first deletion, including terminal records.
        receipts = [self.read(path, remote) for path in sorted(self.state.glob("*.json"))]
        count = 0
        for receipt in receipts:
            if receipt["status"] not in ("verified", "retiring"):
                continue
            remote.verify(receipt["remote"], receipt["fingerprint"])
            with locked(self.release_lock):
                name, fp = receipt["name"], receipt["fingerprint"]
                source = self.root / name
                try:
                    source.lstat()
                except FileNotFoundError:
                    if receipt["status"] != "retiring":
                        raise OffloadError("verified source unexpectedly missing") from None
                    receipt["retirement_result"] = "absent_after_interruption"
                else:
                    self.local_fingerprint(name, fp)
                    receipt.update(status="retiring", retirement_verified_at=timestamp())
                    self.save(receipt)
                    # Recheck after the durable intent record and immediately before unlink.
                    info = source.lstat()
                    if not eligible(name, info, self.now_ns, self.days) or fingerprint(info) != {k: fp[k] for k in FP_KEYS}:
                        raise OffloadError("source changed before retirement")
                    ensure_closed(info)
                    os.unlink(source)  # One verified regular file; never recurse or truncate.
                    self.sync_directory(self.root)
                    receipt["retirement_result"] = "removed"
                receipt.update(status="retired", retired_at=timestamp())
                self.save(receipt)
            print(f"retired {receipt['name']} bytes={receipt['fingerprint']['size']}", flush=True)
            count += 1
        return count

    def execute(self):
        if self.args.action == "preview":
            selected = self.candidates()
            for name, info in selected:
                print(f"candidate {name} bytes={info.st_size} mtime_ns={info.st_mtime_ns}")
            print(f"preview candidates={len(selected)} bytes={sum(info.st_size for _, info in selected)}; no files changed")
            return
        self.prepare_state()
        with locked(self.state / "task.lock", private=True):
            remote = CryptRemote(self.args.remote, self.args.rclone_config)
            count = self.archive(remote) if self.args.action == "archive" else self.retire(remote)
            print(f"completed action={self.args.action} files={count}")


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", nargs="?", choices=("preview", "archive", "retire"), default="preview")
    parser.add_argument("--root", default="/opt/newapi/newapi_data/logs")
    parser.add_argument("--state-dir", default="/opt/newapi/.log-offload")
    parser.add_argument("--retention-days", type=int, default=30)
    parser.add_argument("--release-lock", default="/run/newapi-release.lock")
    parser.add_argument("--remote", help="configured crypt remote and fixed archive prefix")
    parser.add_argument("--rclone-config", help="absolute path to a private 0600 rclone configuration")
    args = parser.parse_args(argv)
    try:
        Offloader(args).execute()
    except (OffloadError, OSError, subprocess.TimeoutExpired) as error:
        # Only controlled errors are printed; OS errors may contain backend/private data.
        print(f"ERROR: {error if isinstance(error, OffloadError) else 'operation failed; inspect local permissions and availability'}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
