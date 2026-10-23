import copy
import importlib.util
import json
import struct
from pathlib import Path
import tempfile
import unittest

from pitr_binlog import HEADER, canonical_json, normalize, parse_chunk
from test_pitr_binlog import event, fde, fixture, MAGIC

spec = importlib.util.spec_from_file_location("binlog_archive", Path(__file__).with_name("mysql-binlog-archive.py"))
archive = importlib.util.module_from_spec(spec)
spec.loader.exec_module(archive)


class FakeStore:
    def __init__(self):
        self.objects = {}
        self.fail = None

    def get(self, destination, maximum):
        return self.objects.get(destination)

    def put(self, destination, data):
        if self.fail == "before":
            self.fail = None
            raise archive.ArchiveError("simulated network failure")
        if destination in self.objects and self.objects[destination] != data:
            raise archive.ArchiveError("immutable object conflict")
        self.objects[destination] = data
        if self.fail and self.fail in destination:
            self.fail = None
            raise archive.ArchiveError("simulated interruption after remote write")


class FakeSource:
    chunk = archive.Source.chunk
    fde_hash = archive.Source.fde_hash

    def __init__(self, data=None):
        self.files = {"binlog.000001": fixture(4) if data is None else data}
        self.source_uuid = "00000000-0000-0000-0000-000000000001"

    def sample(self):
        file = sorted(self.files)[-1]
        return {"uuid": self.source_uuid, "file": file, "position": len(self.files[file]),
                "at": archive.backup.timestamp(), "database_scope": ["new-api"], "gtid_mode": "OFF",
                "logs": [{"file": name, "size": len(self.files[name])} for name in sorted(self.files)]}

    def read(self, file, start, length):
        if file not in self.files or len(self.files[file]) < start + length:
            raise archive.ArchiveError("source file missing or truncated")
        return self.files[file][start:start + length]


class ArchiveTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.state = Path(self.directory.name).resolve() / "state"
        self.store = FakeStore()
        self.source = FakeSource()

    def tearDown(self):
        self.directory.cleanup()

    def worker(self, **kwargs):
        return archive.Archive(self.state, self.store, self.source, "test:pitr", epoch="testepoch", target=300, **kwargs)

    def test_complete_chain_is_remote_discoverable(self):
        result = self.worker().run()
        self.assertEqual(result["status"], "ok")
        self.assertLess(result["lag_seconds"], 2)
        prefix = result["root"]
        root = json.loads(self.store.objects[prefix + "/root.json"])
        receipts = sorted(name for name in self.store.objects if "/receipts/" in name)
        self.assertGreater(len(receipts), 1)
        previous, state, reconstructed = None, None, b""
        for name in receipts:
            record = json.loads(self.store.objects[name])
            archive.validate_receipt(record, previous, root)
            data = self.store.objects[prefix + "/" + record["object"]]
            parsed = parse_chunk(data, record["file"], record["start"], state)
            self.assertEqual(record["parser_state"], parsed["state"])
            reconstructed += data
            state, previous = parsed["state"], record
        self.assertEqual(reconstructed, self.source.files["binlog.000001"])

    def test_release_preflight_databases_do_not_change_business_scope(self):
        self.assertEqual(archive.business_scope(["new-api", "newapi_preflight_20261023000000_12345"], "new-api"), ["new-api"])
        for extra in ("another-business", "newapi_preflight_x", "newapi_preflight_20261023_12345", "newapi_preflight_20261023000000_0"):
            with self.assertRaises(archive.ArchiveError):
                archive.business_scope(["new-api", extra], "new-api")

    def test_idle_round_does_not_upload_new_segments(self):
        first = self.worker().run()
        objects = copy.deepcopy(self.store.objects)
        second = self.worker().run()
        self.assertEqual(objects, self.store.objects)
        self.assertGreaterEqual(second["covered_sample_at"], first["covered_sample_at"])

    def test_metadata_only_file_has_coverage_without_fabricated_transaction(self):
        self.source.files = {"binlog.000001": MAGIC + fde()}
        result = self.worker().run()
        self.assertIsNone(result["last_complete_transaction_frontier"])
        self.assertFalse(result["transaction_open"])
        self.assertEqual(result["verified_cursor"], {"file": "binlog.000001", "position": len(MAGIC + fde())})
        self.assertIsNotNone(result["covered_sample_at"])

    def test_interruption_after_segment_upload_resumes_exact_pending(self):
        self.store.fail = "/segments/"
        with self.assertRaises(archive.ArchiveError):
            self.worker().run()
        pending = archive.load_json(self.state / "pending.json")
        original_name = pending["object"]
        self.worker().run()
        self.assertFalse((self.state / "pending.json").exists())
        self.assertEqual(sum(name.endswith(original_name) for name in self.store.objects), 1)

    def test_interruption_after_receipt_upload_resumes_same_receipt(self):
        self.store.fail = "/receipts/"
        with self.assertRaises(archive.ArchiveError):
            self.worker().run()
        name = next(name for name in self.store.objects if "/receipts/" in name)
        prior = self.store.objects[name]
        self.worker().run()
        self.assertEqual(prior, self.store.objects[name])

    def test_corrupted_existing_remote_never_advances_checkpoint(self):
        self.store.fail = "/segments/"
        with self.assertRaises(archive.ArchiveError):
            self.worker().run()
        name = next(name for name in self.store.objects if "/segments/" in name)
        self.store.objects[name] = b"corrupt"
        with self.assertRaisesRegex(archive.ArchiveError, "conflict"):
            self.worker().run()
        self.assertIsNone(archive.load_json(self.state / "checkpoint.json")["receipt"])

    def test_source_uuid_change_stops_chain(self):
        self.worker().run()
        self.source.source_uuid = "00000000-0000-0000-0000-000000000002"
        with self.assertRaisesRegex(archive.ArchiveError, "identity"):
            self.worker().run()

    def test_interruption_after_checkpoint_before_pending_unlink(self):
        self.worker().run()
        checkpoint = archive.load_json(self.state / "checkpoint.json")
        archive.backup.atomic_json(self.state / "pending.json", checkpoint["receipt"])
        before = copy.deepcopy(self.store.objects)
        self.worker().run()
        self.assertFalse((self.state / "pending.json").exists())
        self.assertEqual(before, self.store.objects)

    def test_same_uuid_same_filename_with_changed_fde_stops(self):
        self.worker().run()
        data = bytearray(self.source.files["binlog.000001"])
        data[50] ^= 1
        self.source.files["binlog.000001"] = bytes(data)
        with self.assertRaisesRegex(archive.ArchiveError, "differs"):
            self.worker().run()

    def test_cross_file_metadata_does_not_invent_transaction(self):
        self.worker().run()
        prior = archive.load_json(self.state / "checkpoint.json")["last_complete_transaction_frontier"]
        data = self.source.files["binlog.000001"]
        data += event(4, struct.pack("<Q", 4) + b"binlog.000002", len(data))
        self.source.files["binlog.000001"] = data
        self.source.files["binlog.000002"] = MAGIC + fde()
        status = self.worker().run()
        self.assertEqual(status["last_complete_transaction_frontier"], prior)
        self.assertFalse(status["transaction_open"])
        self.assertEqual(status["verified_cursor"]["file"], "binlog.000002")
        self.assertLess(status["lag_seconds"], 2)

    def test_source_truncation_stops_chain(self):
        self.worker().run()
        self.source.files["binlog.000001"] = self.source.files["binlog.000001"][:-1]
        with self.assertRaises(archive.ArchiveError):
            self.worker().run()

    def test_rotation_normalization_survives_retry(self):
        data = bytearray(self.source.files["binlog.000001"])
        data[21] |= 1
        self.source.files["binlog.000001"] = bytes(data)
        self.store.fail = "/segments/"
        with self.assertRaises(archive.ArchiveError):
            self.worker().run()
        data[21] &= ~1
        self.source.files["binlog.000001"] = bytes(data)
        self.worker().run()

    def test_missing_next_file_stops_chain(self):
        self.worker().run()
        self.source.files["binlog.000003"] = MAGIC + fde()
        with self.assertRaises(ValueError):
            self.worker().run()

    def test_local_checkpoint_loss_requires_explicit_recovery(self):
        self.worker().run()
        (self.state / "checkpoint.json").unlink()
        with self.assertRaisesRegex(archive.ArchiveError, "recover state"):
            self.worker().run()

    def test_pending_source_changed_stops_chain(self):
        self.store.fail = "/segments/"
        with self.assertRaises(archive.ArchiveError):
            self.worker().run()
        data = bytearray(self.source.files["binlog.000001"])
        data[100] ^= 1
        self.source.files["binlog.000001"] = bytes(data)
        with self.assertRaisesRegex(archive.ArchiveError, "pending source"):
            self.worker().run()


if __name__ == "__main__":
    unittest.main()
