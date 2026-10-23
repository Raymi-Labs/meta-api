"""Test-only Docker volume transport for the actual production archive engine.

Source SQL, framing, chunking, crypt upload/readback and checkpoint logic are
unchanged. Only the Linux volume byte transport differs on Docker Desktop.
"""
import importlib.util
import json
from pathlib import Path
from types import SimpleNamespace

from pitr_binlog import valid_file

_spec = importlib.util.spec_from_file_location("pitr_archive_integration", Path(__file__).with_name("mysql-binlog-archive.py"))
archive = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(archive)


class DockerVolumeSource(archive.Source):
    def __init__(self, server, processes, database):
        server.assert_owned()
        self.server, self.processes = server, processes
        self.args = SimpleNamespace(container=server.container_id, database=database)
        inspected = json.loads(server.run(["docker", "inspect", server.container_id]))[0]
        self.container_id = inspected["Id"]
        self.container_started_at = inspected["State"]["StartedAt"]

    def read(self, file, start, length):
        valid_file(file)
        self.server.assert_owned()
        data = self.server.exec(["dd", "if=/var/lib/mysql/" + file, "bs=1048576",
                                 "iflag=skip_bytes,count_bytes", "skip=" + str(start),
                                 "count=" + str(length), "status=none"])
        if len(data) != length:
            raise archive.ArchiveError("isolated source file was truncated")
        return data


def archive_real(server, remote, work, start_file, database="pitr_fixture"):
    """Run actual Archive/Store with one interruption and a subsequent idle run."""
    source = DockerVolumeSource(server, remote.processes, database)
    state_dir = Path(work) / "archive-state"

    class InterruptAfterVerifiedSegment(archive.Store):
        interrupted = False

        def put(self, destination, data):
            super().put(destination, data)
            if "/segments/" in destination and not self.interrupted:
                self.interrupted = True
                raise archive.ArchiveError("injected interruption after real crypt segment verification")

    first = archive.Archive(state_dir, InterruptAfterVerifiedSegment(remote), source,
                            remote.prefix + "/pitr", start_file, "synthetic", 300)
    try:
        first.run()
    except archive.ArchiveError as error:
        if "injected interruption" not in str(error):
            raise
    else:
        raise AssertionError("archive interruption was not exercised")
    pending = archive.load_json(state_dir / "pending.json")
    assert pending["seq"] == 1
    store = archive.Store(remote)
    worker = archive.Archive(state_dir, store, source, remote.prefix + "/pitr", start_file, "synthetic", 300)
    result = worker.run()
    checkpoint = archive.load_json(state_dir / "checkpoint.json")
    receipt_before = store.get(result["latest_receipt_object"], archive.backup.BLOCK)
    assert result["status"] == "ok" and not result["transaction_open"]
    assert result["verified_cursor"] == {"file": result["source_sample"]["file"], "position": result["source_sample"]["position"]}
    assert not (state_dir / "pending.json").exists()
    idle = archive.Archive(state_dir, store, source, remote.prefix + "/pitr", start_file, "synthetic", 300).run()
    assert idle["latest_receipt_sha256"] == result["latest_receipt_sha256"]
    assert store.get(idle["latest_receipt_object"], archive.backup.BLOCK) == receipt_before
    return {"root": result["root"], "segments": checkpoint["receipt"]["seq"], "status": result}
