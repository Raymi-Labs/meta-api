#!/usr/bin/env python3
"""Archive verified immutable MySQL binlog ranges without rotation or publication locks."""
import argparse
import datetime as dt
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import stat
import struct
import subprocess
import sys
import uuid

from pitr_binlog import (HEADER, MAX_EVENT, canonical_json, follows, normalize,
                        parse_chunk, receipt_digest, segment_name, valid_file,
                        validate_receipt)


def load_helpers():
    # Import only the administrator-owned sibling, never a cwd/PYTHONPATH helper.
    path = Path(__file__).resolve().with_name("mysql-r2-backup.py")
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid not in (0, os.geteuid()) or info.st_mode & 0o022:
        raise RuntimeError("unsafe backup helper")
    spec = importlib.util.spec_from_file_location("newapi_mysql_backup_helpers", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


backup = load_helpers()
ArchiveError = backup.BackupError


def sha(value):
    return hashlib.sha256(value).hexdigest()


def load_json(path):
    return json.loads(backup.private_bytes(path))


def seconds_since(value):
    if value is None:
        return None
    return max(0, (dt.datetime.now(dt.timezone.utc) - dt.datetime.fromisoformat(value)).total_seconds())


def business_scope(databases, database):
    if database not in databases:
        raise ArchiveError("configured business database is missing")
    unexpected = [name for name in databases if name != database
                  and not re.fullmatch(r"newapi_preflight_[0-9]{14}_[1-9][0-9]*", name)]
    if unexpected:
        raise ArchiveError("unrecognized database scope requires a new verified backup anchor")
    return [database]


class Store:
    """Existing crypt transport, with content-based immutable retries."""
    def __init__(self, remote):
        self.remote = remote

    def get(self, destination, maximum):
        process = self.remote.processes.start(
            self.remote.command("lsjson", destination, "--stat", "--files-only"),
            stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, env=self.remote.env)
        listing, _ = process.communicate()
        if process.returncode in (3, 4):
            return None
        self.remote.processes.check(process, "remote object existence check failed")
        try:
            info = json.loads(listing)
            # Bucket backends otherwise report a missing key as a virtual
            # directory (IsDir=true, Size=-1). File-only stat returns JSON null.
            if info is None:
                return None
            if not isinstance(info, dict):
                raise ValueError()
            if (info.get("IsDir") is not False or type(info.get("Size")) is not int
                    or not 0 <= info["Size"] <= maximum):
                raise ArchiveError("remote object has an unexpected type or size")
        except (ValueError, KeyError, TypeError):
            raise ArchiveError("remote object metadata is invalid") from None
        reader = self.remote.reader(destination)
        data = reader.stdout.read(maximum + 1)
        self.remote.processes.check(reader, "archive readback failed")
        if len(data) > maximum:
            raise ArchiveError("archive readback exceeds expected size")
        return data

    def put(self, destination, data):
        existing = self.get(destination, len(data))
        if existing is not None:
            if existing != data:
                raise ArchiveError("immutable remote object differs; archive chain stopped")
            return
        writer = self.remote.writer(destination)
        writer.communicate(data)
        self.remote.processes.check(writer, "archive upload failed")
        if self.get(destination, len(data)) != data:
            raise ArchiveError("archive readback did not match uploaded content")
        self.remote.check_config()


class Source:
    def __init__(self, args, processes):
        self.args, self.processes = args, processes
        self.datadir = Path(args.datadir)
        if not self.datadir.is_absolute() or self.datadir.resolve() != self.datadir:
            raise ArchiveError("datadir must be an absolute path without symlinks")
        inspected = json.loads(processes.capture(["docker", "inspect", args.container]))
        if len(inspected) != 1 or not inspected[0].get("State", {}).get("Running"):
            raise ArchiveError("MySQL source container is not running")
        matches = [m for m in inspected[0]["Mounts"]
                   if m.get("Destination", "").rstrip("/") == "/var/lib/mysql"]
        if (len(matches) != 1 or matches[0].get("Type") != "bind"
                or matches[0].get("Source") != str(self.datadir)):
            raise ArchiveError("source datadir is not the inspected MySQL bind mount")
        info = self.datadir.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_mode & 0o022:
            raise ArchiveError("source datadir has unsafe permissions")
        self.owner = info.st_uid
        self.container_id = inspected[0]["Id"]
        self.container_started_at = inspected[0]["State"]["StartedAt"]

    def sql(self, query):
        options = ["--protocol=SOCKET", "--skip-reconnect", "--batch", "--skip-column-names", "--raw", "--execute=" + query]
        value = self.processes.capture(backup.mysql_command(self.args, "mysql", options))
        return value.decode().strip().splitlines()

    def sample(self):
        at = backup.timestamp()
        settings = self.sql("SELECT @@server_uuid,@@log_bin,@@binlog_format,@@binlog_checksum,"
                            "@@binlog_encryption,@@binlog_transaction_compression,@@sync_binlog,"
                            "@@innodb_flush_log_at_trx_commit,@@datadir,@@gtid_mode,@@binlog_row_image")[0].split("\t")
        if (len(settings) != 11 or settings[1:8] != ["1", "ROW", "CRC32", "0", "0", "1", "1"]
                or settings[8].rstrip("/") != "/var/lib/mysql" or settings[9:] != ["OFF", "FULL"]):
            raise ArchiveError("source requires GTID OFF, ROW FULL CRC32 unencrypted uncompressed binlog and durable commits")
        try:
            source_uuid = str(uuid.UUID(settings[0]))
        except ValueError:
            raise ArchiveError("invalid source UUID") from None
        master = self.sql("SHOW MASTER STATUS")
        if len(master) != 1:
            raise ArchiveError("source binary log status is unavailable")
        fields = master[0].split("\t")
        file, position = valid_file(fields[0]), int(fields[1])
        logs = []
        for line in self.sql("SHOW BINARY LOGS"):
            name, size, encrypted = line.split("\t")
            if encrypted != "No":
                raise ArchiveError("source binlog encryption is unsupported for direct ranges")
            logs.append({"file": valid_file(name), "size": int(size)})
        names = [entry["file"] for entry in logs]
        if file not in names or any(not follows(a, b) for a, b in zip(names, names[1:])):
            raise ArchiveError("source binlog index has a missing file")
        databases = self.sql("SELECT SCHEMA_NAME FROM information_schema.SCHEMATA WHERE "
                             "SCHEMA_NAME NOT IN ('mysql','sys','performance_schema','information_schema') "
                             "ORDER BY SCHEMA_NAME")
        return {"uuid": source_uuid, "file": file, "position": position, "at": at,
                "logs": logs[:names.index(file) + 1], "database_scope": business_scope(databases, self.args.database),
                "observed_schemas": databases,
                "gtid_mode": settings[9], "container_id": self.container_id,
                "container_started_at": self.container_started_at}

    def read(self, file, start, length):
        valid_file(file)
        path = self.datadir / file
        try:
            fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
        except OSError:
            raise ArchiveError("source binlog is missing or cannot be opened") from None
        with os.fdopen(fd, "rb") as stream:
            info = os.fstat(stream.fileno())
            if (not stat.S_ISREG(info.st_mode) or info.st_nlink != 1
                    or info.st_uid != self.owner or info.st_mode & 0o022):
                raise ArchiveError("source binlog has unsafe type or ownership")
            if info.st_size < start + length:
                raise ArchiveError("source binlog shrank below the archived position")
            stream.seek(start)
            data = stream.read(length)
            current = path.lstat()
            if ((info.st_dev, info.st_ino) != (current.st_dev, current.st_ino)
                    or len(data) != length):
                raise ArchiveError("source binlog changed while reading")
        return data

    def chunk(self, file, start, limit, target):
        """Bound memory and disk use; a single event may exceed target, never MAX_EVENT."""
        data = self.read(file, start, min(limit - start, target + HEADER.size))
        pos = 0
        if start == 0:
            if data[:4] != b"\xfebin":
                raise ArchiveError("source binlog has invalid magic")
            pos = 4
        while start + pos < limit:
            if limit - start - pos < HEADER.size:
                raise ArchiveError("source position cuts through an event header")
            if len(data) - pos < HEADER.size:
                break
            length = struct.unpack_from("<I", data, pos + 9)[0]
            if length < HEADER.size + 4 or length > MAX_EVENT:
                raise ArchiveError("source event exceeds the supported size limit")
            if start + pos + length > limit:
                raise ArchiveError("source position cuts through an event payload")
            if pos > (4 if start == 0 else 0) and pos + length > target:
                break
            if pos + length > len(data):
                data = self.read(file, start, pos + length)
            pos += length
            if pos >= target:
                break
        return data[:pos]

    def fde_hash(self, file):
        header = self.read(file, 0, 23)
        length = struct.unpack_from("<I", header, 13)[0]
        if length < 23 or length > 4096:
            raise ArchiveError("invalid source format description size")
        return sha(normalize(self.read(file, 0, length + 4), 0)[4:])


class Archive:
    def __init__(self, state_dir, store, source, remote_prefix, start_file=None, epoch=None, target=16 * 1024 * 1024):
        self.state_dir = backup.private_directory(str(state_dir))
        self.store, self.source, self.prefix = store, source, remote_prefix.rstrip("/")
        self.start_file, self.epoch, self.target = start_file, epoch, target
        self.root = None
        self.checkpoint = None

    def save(self, name, value):
        backup.atomic_json(self.state_dir / name, value)

    def remote(self, relative):
        return self.checkpoint["root"] + "/" + relative

    def initialize(self, sample):
        root_path = self.state_dir / "root.json"
        if root_path.exists():
            self.root = load_json(root_path)
        else:
            start = valid_file(self.start_file or sample["file"])
            if start not in [log["file"] for log in sample["logs"]]:
                raise ArchiveError("requested starting file is no longer available")
            epoch = self.epoch or dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%SZ") + "-" + uuid.uuid4().hex[:12]
            if not re.fullmatch(r"[A-Za-z0-9_-]{1,80}", epoch):
                raise ArchiveError("invalid archive epoch")
            self.root = {"schema": 1, "uuid": sample["uuid"], "epoch": epoch,
                         "start_file": start, "created_at": backup.timestamp(),
                         "database_scope": sample["database_scope"], "gtid_mode": sample.get("gtid_mode"),
                         "normalization": "fde-clear-in-use-v1", "maximum_event_bytes": MAX_EVENT}
            self.save("root.json", self.root)
        if (self.root["uuid"] != sample["uuid"] or self.root["database_scope"] != sample["database_scope"]
                or (self.epoch and self.root["epoch"] != self.epoch)):
            raise ArchiveError("source identity or database scope changed; new verified anchor is required")
        destination = self.prefix + "/" + self.root["uuid"] + "/" + self.root["epoch"]
        self.store.put(destination + "/root.json", canonical_json(self.root))
        path = self.state_dir / "checkpoint.json"
        if path.exists():
            self.checkpoint = load_json(path)
            if self.checkpoint["root"] != destination:
                raise ArchiveError("checkpoint destination differs from configured archive root")
        else:
            # A lost local checkpoint must not silently start a second branch.
            if self.store.get(destination + "/receipts/000000000001.json", backup.BLOCK) is not None:
                raise ArchiveError("remote chain already exists; recover state explicitly before resuming")
            self.checkpoint = {"schema": 1, "status": "initializing", "root": destination,
                               "source_uuid": sample["uuid"], "epoch": self.root["epoch"],
                               "cursor": {"file": self.root["start_file"], "position": 0},
                               "receipt": None, "parser_state": None, "covered_sample_at": None,
                               "last_complete_transaction_frontier": None, "verified_at": None,
                               "latest_receipt_object": None, "latest_receipt_sha256": None}
            self.save("checkpoint.json", self.checkpoint)
        previous = self.checkpoint.get("receipt")
        if previous:
            data = normalize(self.source.read(previous["file"], previous["start"], previous["size"]), previous["start"])
            if sha(data) != previous["sha256"] or self.source.fde_hash(previous["file"]) != previous["parser_state"]["fde_sha256"]:
                raise ArchiveError("source binlog differs from verified checkpoint; possible reset or recovery truncation")

    def stage(self, file, start, limit, sample):
        state = self.checkpoint["parser_state"]
        if state and state["file"] == file and self.source.fde_hash(file) != state["fde_sha256"]:
            raise ArchiveError("source format description changed; binlog filename may have been reused")
        data = self.source.chunk(file, start, limit, self.target)
        if not data:
            raise ArchiveError("empty source range")
        parsed = parse_chunk(data, file, start, state)
        data = parsed["data"]
        previous = self.checkpoint["receipt"]
        seq = previous["seq"] + 1 if previous else 1
        end = start + len(data)
        record = {"schema": 1, "seq": seq, "source_uuid": self.root["uuid"],
                  "file": file, "start": start, "end": end, "size": len(data), "sha256": sha(data),
                  "object": segment_name(seq, file, start, end, sha(data)),
                  "previous_receipt_sha256": receipt_digest(previous) if previous else None,
                  "previous_file": previous["file"] if previous else None,
                  "source_sample": {k: sample[k] for k in ("file", "position", "at")},
                  "parser_state": parsed["state"],
                  "last_complete_transaction_frontier": parsed["last_complete_transaction_frontier"],
                  "verified_at": None}
        self.save("pending.json", record)
        return record

    def complete(self, pending):
        previous = self.checkpoint["receipt"]
        # A crash after checkpoint fsync but before pending unlink is harmless.
        if previous and pending["seq"] == previous["seq"]:
            if pending != previous:
                raise ArchiveError("stale pending receipt conflicts with checkpoint")
            (self.state_dir / "pending.json").unlink()
            return
        validate_receipt(pending, previous, self.root)
        data = normalize(self.source.read(pending["file"], pending["start"], pending["size"]), pending["start"])
        if sha(data) != pending["sha256"]:
            raise ArchiveError("pending source range changed; refusing archive continuation")
        parsed = parse_chunk(data, pending["file"], pending["start"], self.checkpoint["parser_state"])
        if parsed["state"] != pending["parser_state"]:
            raise ArchiveError("pending transaction state does not match source events")
        self.store.put(self.remote(pending["object"]), data)
        if pending["verified_at"] is None:
            pending["verified_at"] = backup.timestamp()
            self.save("pending.json", pending)
        receipt_object = f"receipts/{pending['seq']:012d}.json"
        self.store.put(self.remote(receipt_object), canonical_json(pending))
        self.checkpoint.update(receipt=pending, parser_state=pending["parser_state"],
                               cursor={"file": pending["file"], "position": pending["end"]},
                               latest_receipt_object=self.remote(receipt_object),
                               latest_receipt_sha256=receipt_digest(pending), verified_at=pending["verified_at"],
                               last_complete_transaction_frontier=pending["last_complete_transaction_frontier"])
        source_sample = pending["source_sample"]
        if (pending["file"] == source_sample["file"] and pending["end"] == source_sample["position"]
                and not parsed["state"]["in_transaction"] and not parsed["state"]["pending_gtid"]):
            self.checkpoint["covered_sample_at"] = source_sample["at"]
        self.save("checkpoint.json", self.checkpoint)
        (self.state_dir / "pending.json").unlink()

    def status(self, sample, error=None):
        value = {k: v for k, v in self.checkpoint.items() if k not in ("receipt", "parser_state")}
        parser_state = self.checkpoint.get("parser_state") or {}
        value.update(status="error" if error else "ok", error=error,
                     verified_cursor=self.checkpoint["cursor"],
                     transaction_open=bool(parser_state.get("in_transaction") or parser_state.get("pending_gtid")),
                     source_sample={k: sample[k] for k in ("file", "position", "at")},
                     source_sample_at=sample["at"], last_observed_at=backup.timestamp(),
                     lag_seconds=seconds_since(self.checkpoint.get("covered_sample_at")))
        self.save("status.json", value)
        return value

    def run(self):
        sample = self.source.sample()
        try:
            self.initialize(sample)
            pending = self.state_dir / "pending.json"
            if pending.exists():
                self.complete(load_json(pending))
            names = [entry["file"] for entry in sample["logs"]]
            while True:
                cursor = self.checkpoint["cursor"]
                file, start = cursor["file"], cursor["position"]
                if file not in names:
                    raise ArchiveError("checkpoint binlog is missing from source index")
                index = names.index(file)
                limit = sample["position"] if file == sample["file"] else sample["logs"][index]["size"]
                if start > limit:
                    raise ArchiveError("source position moved behind the verified archive")
                if start == limit:
                    if file == sample["file"]:
                        state = self.checkpoint["parser_state"] or {}
                        if state.get("in_transaction") or state.get("pending_gtid"):
                            raise ArchiveError("sampled source frontier is not a complete transaction boundary")
                        self.checkpoint["covered_sample_at"] = sample["at"]
                        self.save("checkpoint.json", self.checkpoint)
                        break
                    file, start = names[index + 1], 0
                    limit = sample["position"] if file == sample["file"] else sample["logs"][index + 1]["size"]
                self.complete(self.stage(file, start, limit, sample))
            return self.status(sample)
        except Exception as error:
            if self.checkpoint is not None:
                self.status(sample, str(error) if isinstance(error, (ArchiveError, ValueError)) else "archive operation failed")
            raise


def arguments():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--container", default="newapi-mysql")
    parser.add_argument("--database", default="new-api")
    parser.add_argument("--datadir", default="/opt/newapi/mysql_data")
    parser.add_argument("--state-dir", default="/var/lib/newapi-binlog-archive")
    parser.add_argument("--lock", default="/run/newapi-binlog-archive.lock")
    parser.add_argument("--remote", default="r2-backup:pitr")
    parser.add_argument("--rclone-config", default="/root/.config/rclone/rclone.conf")
    parser.add_argument("--start-file")
    parser.add_argument("--epoch")
    parser.add_argument("--chunk-bytes", type=int, default=16 * 1024 * 1024)
    parser.add_argument("--deadline", type=int, default=45)
    return parser.parse_args()


def main():
    args = arguments()
    if not 1024 <= args.chunk_bytes <= MAX_EVENT or not 5 <= args.deadline <= 3600:
        raise ArchiveError("invalid chunk size or deadline")
    with backup.locked(args.lock), backup.deadline(args.deadline), backup.Processes() as processes:
        remote = backup.CryptRemote(args.remote, args.rclone_config, processes)
        worker = Archive(args.state_dir, Store(remote), Source(args, processes), remote.prefix,
                         args.start_file, args.epoch, args.chunk_bytes)
        print(json.dumps(worker.run(), sort_keys=True))


if __name__ == "__main__":
    try:
        main()
    except (ArchiveError, ValueError, KeyError, OSError, RuntimeError) as error:
        print("binlog archive failed: " + (str(error) if isinstance(error, (ArchiveError, ValueError, RuntimeError))
                                          else "archive state or source is unavailable"), file=sys.stderr)
        sys.exit(1)
