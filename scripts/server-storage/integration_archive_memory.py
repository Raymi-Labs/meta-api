#!/usr/bin/env python3
"""Exercise the archive's exact 64 MiB framing limit under a Linux memory cap.

Run in Docker with --memory=768m --memory-swap=768m and no network. This test
uses synthetic CRC-valid ROW framing, real source file reads, Archive, Store
and rclone crypt over temporary local storage. The independent integration_pitr
test covers real MySQL ROW semantics and recovery; this tests resource bounds.
"""
import hashlib
import json
import os
from pathlib import Path
import secrets
import struct
import subprocess
import tempfile
import zlib

from integration_archive import archive
from pitr_binlog import HEADER, MAGIC, MAX_EVENT
from test_pitr_binlog import event, fde, query

LIMIT = 768 * 1024 * 1024
FILE = "binlog.000001"


class FileSource(archive.Source):
    """Use the production byte reader without needing a Docker socket in the test."""
    def __init__(self, datadir):
        self.datadir = datadir
        self.owner = os.geteuid()

    def sample(self):
        size = (self.datadir / FILE).stat().st_size
        return {"uuid": "00000000-0000-0000-0000-000000000064", "file": FILE,
                "position": size, "at": archive.backup.timestamp(),
                "database_scope": ["new-api"], "gtid_mode": "OFF",
                "logs": [{"file": FILE, "size": size}]}


def fixture(path):
    prefix = MAGIC + fde()
    prefix += event(35, b"\0" * 8, len(prefix))
    prefix += event(34, b"\0" * 42, len(prefix))
    prefix += query(b"BEGIN", len(prefix))
    prefix += event(19, b"\0" * 30, len(prefix))
    row_start = len(prefix)
    header = HEADER.pack(1700000000, 30, 123, MAX_EVENT, row_start + MAX_EVENT, 0)
    digest = hashlib.sha256(header)
    crc = zlib.crc32(header)
    block = b"\0" * (1024 * 1024)
    with path.open("wb") as output:
        output.write(prefix)
        output.write(header)
        remaining = MAX_EVENT - len(header) - 4
        while remaining:
            value = block[:min(remaining, len(block))]
            output.write(value)
            digest.update(value)
            crc = zlib.crc32(value, crc)
            remaining -= len(value)
        checksum = struct.pack("<I", crc)
        output.write(checksum)
        digest.update(checksum)
        output.write(event(16, struct.pack("<Q", 1), row_start + MAX_EVENT))
    path.chmod(0o600)
    return row_start, digest.hexdigest()


def run(work):
    cgroup = Path("/sys/fs/cgroup")
    if (cgroup / "memory.max").read_text().strip() != str(LIMIT):
        raise RuntimeError("test requires a cgroup v2 memory.max of exactly 768 MiB")
    if (cgroup / "memory.swap.max").read_text().strip() != "0":
        raise RuntimeError("test requires swap disabled for this cgroup")
    os.chdir(work)
    datadir = work / "source"
    datadir.mkdir(mode=0o700)
    row_start, expected_sha = fixture(datadir / FILE)
    source = FileSource(datadir)
    obscured = subprocess.run(["rclone", "obscure", "--", secrets.token_urlsafe(32)],
                              check=True, stdout=subprocess.PIPE).stdout.decode().strip()
    config = work / "rclone.conf"
    config.write_text("[local]\ntype=local\n[crypt]\ntype=crypt\nremote=local:objects\npassword=" + obscured + "\n")
    config.chmod(0o600)
    state = work / "state"

    class InterruptedStore(archive.Store):
        def put(self, destination, data):
            super().put(destination, data)
            if "/segments/" in destination and len(data) == MAX_EVENT:
                raise archive.ArchiveError("injected interruption after maximum event readback")

    with archive.backup.deadline(45), archive.backup.Processes() as processes:
        remote = archive.backup.CryptRemote("crypt:memory-fixture", config, processes)
        try:
            archive.Archive(state, InterruptedStore(remote), source, remote.prefix,
                            FILE, "memory-test").run()
        except archive.ArchiveError as error:
            if "injected interruption" not in str(error):
                raise
        else:
            raise AssertionError("maximum event interruption was not exercised")
        pending = archive.load_json(state / "pending.json")
        assert pending["start"] == row_start and pending["size"] == MAX_EVENT
        assert pending["sha256"] == expected_sha
        store = archive.Store(remote)
        status = archive.Archive(state, store, source, remote.prefix, FILE, "memory-test").run()
        assert status["status"] == "ok" and not status["transaction_open"]
        assert status["verified_cursor"] == status["last_complete_transaction_frontier"]
        assert status["verified_cursor"]["position"] == (datadir / FILE).stat().st_size
        assert not (state / "pending.json").exists()
        checkpoint = archive.load_json(state / "checkpoint.json")
        assert checkpoint["receipt"]["seq"] == 3
        idle = archive.Archive(state, store, source, remote.prefix, FILE, "memory-test").run()
        assert idle["latest_receipt_sha256"] == status["latest_receipt_sha256"]
        # The upper bound must still fail before allocating the advertised payload.
        oversized = datadir / "binlog.000002"
        oversized.write_bytes(b"x" * 4 + HEADER.pack(0, 30, 123, MAX_EVENT + 1, MAX_EVENT + 5, 0))
        oversized.chmod(0o600)
        try:
            source.chunk("binlog.000002", 4, 4 + HEADER.size, 16 * 1024 * 1024)
        except archive.ArchiveError as error:
            assert "supported size limit" in str(error)
        else:
            raise AssertionError("MAX_EVENT + 1 was accepted")
    peak = int((cgroup / "memory.peak").read_text())
    events = dict(line.split() for line in (cgroup / "memory.events").read_text().splitlines())
    assert events["oom"] == "0" and events["oom_kill"] == "0"
    assert peak <= LIMIT
    return {"status": "success", "event_bytes": MAX_EVENT, "memory_limit_bytes": LIMIT,
            "memory_peak_bytes": peak, "memory_peak_mib": round(peak / (1024 * 1024), 2),
            "coverage": ["exact-64MiB-event", "real-source-reader", "production-archive-store",
                         "crypt-upload-decrypted-readback", "maximum-event-interruption-resume",
                         "idempotent-idle", "oversized-event-rejected", "no-swap", "no-oom"]}


if __name__ == "__main__":
    previous_cwd = Path.cwd()
    try:
        with tempfile.TemporaryDirectory(prefix="newapi-pitr-memory-") as temporary:
            result = run(Path(temporary).resolve())
            os.chdir(previous_cwd)
            print(json.dumps(result, sort_keys=True))
    finally:
        os.chdir(previous_cwd)
