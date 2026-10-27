import copy
import importlib.util
import io
import json
import os
from types import SimpleNamespace
import struct
from pathlib import Path
import tempfile
import unittest
from unittest import mock

from pitr_binlog import HEADER, canonical_json, normalize, parse_chunk
from test_pitr_binlog import event, fde, fixture, MAGIC

spec = importlib.util.spec_from_file_location("binlog_archive", Path(__file__).with_name("mysql-binlog-archive.py"))
archive = importlib.util.module_from_spec(spec)
spec.loader.exec_module(archive)


class DatadirAliasTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.old, self.new = self.root / 'newapi', self.root / 'meta-api'
        self.new.mkdir(mode=0o700)
        (self.new / 'mysql_data').mkdir(mode=0o700)
        self.old.symlink_to(self.new)
        for name, value in [('LEGACY_APP_DIR', self.old), ('META_APP_DIR', self.new)]:
            patcher = mock.patch.object(archive, name, value)
            patcher.start()
            self.addCleanup(patcher.stop)

    def test_fixed_alias_and_canonical_bind_are_same_source(self):
        for configured in (self.old / 'mysql_data', self.new / 'mysql_data'):
            for mounted in (self.old / 'mysql_data', self.new / 'mysql_data'):
                with self.subTest(configured=configured, mounted=mounted):
                    processes = mock.Mock()
                    processes.capture.return_value = json.dumps([{
                        'Id': 'same-container', 'State': {'Running': True, 'StartedAt': 'same-start'},
                        'Mounts': [{'Destination': '/var/lib/mysql', 'Type': 'bind', 'Source': str(mounted)}],
                    }])
                    source = archive.Source(SimpleNamespace(datadir=str(configured), container='test'), processes)
                    self.assertEqual(source.datadir, self.new / 'mysql_data')
                    self.assertEqual(source.container_id, 'same-container')

    def test_other_links_wrong_target_and_writable_directory_rejected(self):
        other = self.root / 'other'
        other.symlink_to(self.new)
        with self.assertRaises(archive.ArchiveError):
            archive.canonical_datadir(other / 'mysql_data')
        self.new.chmod(0o777)
        with self.assertRaises(archive.ArchiveError):
            archive.canonical_datadir(self.old / 'mysql_data')
        self.new.chmod(0o700)
        self.old.unlink()
        self.old.symlink_to(other)
        with self.assertRaises(archive.ArchiveError):
            archive.canonical_datadir(self.old / 'mysql_data')

    def test_untrusted_alias_owner_and_nested_datadir_link_rejected(self):
        real = Path.lstat
        def lstat(path):
            value = real(path)
            if path == self.old:
                return SimpleNamespace(st_mode=value.st_mode, st_uid=os.geteuid() + 10000)
            return value
        with mock.patch.object(Path, 'lstat', lstat), self.assertRaises(archive.ArchiveError):
            archive.canonical_datadir(self.old / 'mysql_data')
        (self.new / 'mysql_data').rmdir()
        (self.new / 'mysql_data').symlink_to(self.root)
        with self.assertRaises(archive.ArchiveError):
            archive.canonical_datadir(self.old / 'mysql_data')


class StoreTests(unittest.TestCase):
    def remote(self, listing, code=0, data=b"verified"):
        remote = mock.Mock()
        remote.env = {}
        stat = mock.Mock(returncode=code)
        stat.communicate.return_value = (listing, None)
        remote.processes.start.return_value = stat
        reader = mock.Mock(returncode=0, stdout=io.BytesIO(data))
        remote.reader.return_value = reader

        def check(child, message):
            if child.returncode:
                raise archive.ArchiveError(message)

        remote.processes.check.side_effect = check
        return remote

    def test_existing_file_is_read_and_verified_as_file_only(self):
        remote = self.remote(b'{"IsDir":false,"Size":8}')
        self.assertEqual(archive.Store(remote).get("crypt:root.json", 8), b"verified")
        remote.command.assert_called_once_with("lsjson", "crypt:root.json", "--stat", "--files-only")
        remote.reader.assert_called_once_with("crypt:root.json")

    def test_bucket_missing_object_null_is_not_read(self):
        remote = self.remote(b"null\n")
        self.assertIsNone(archive.Store(remote).get("crypt:root.json", 8))
        remote.reader.assert_not_called()

    def test_explicit_missing_exit_codes_are_not_read(self):
        for code in (3, 4):
            with self.subTest(code=code):
                remote = self.remote(b"", code)
                self.assertIsNone(archive.Store(remote).get("crypt:root.json", 8))
                remote.reader.assert_not_called()

    def test_remote_errors_do_not_become_missing_or_trigger_upload(self):
        for code in (1, 5, 7):
            with self.subTest(code=code):
                remote = self.remote(b"null\n", code)
                with self.assertRaisesRegex(archive.ArchiveError, "existence check failed"):
                    archive.Store(remote).put("crypt:root.json", b"verified")
                remote.reader.assert_not_called()
                remote.writer.assert_not_called()

    def test_directory_and_invalid_size_fail_closed(self):
        for info in ({"IsDir": True, "Size": -1}, {"IsDir": True, "Size": 0},
                     {"IsDir": False, "Size": -1}, {"IsDir": False, "Size": 9},
                     {"IsDir": False, "Size": True}, {"IsDir": False, "Size": "8"},
                     {"IsDir": False}, {"Size": 8}):
            with self.subTest(info=info):
                remote = self.remote(json.dumps(info).encode())
                with self.assertRaisesRegex(archive.ArchiveError, "unexpected type or size"):
                    archive.Store(remote).put("crypt:root.json", b"verified")
                remote.reader.assert_not_called()
                remote.writer.assert_not_called()

    def test_malformed_metadata_does_not_trigger_upload(self):
        for listing in (b"[]", b"true", b"{}broken", b'"null"'):
            with self.subTest(listing=listing):
                remote = self.remote(listing)
                with self.assertRaisesRegex(archive.ArchiveError, "metadata is invalid"):
                    archive.Store(remote).put("crypt:root.json", b"verified")
                remote.writer.assert_not_called()


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
