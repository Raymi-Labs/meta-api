import importlib.util
from pathlib import Path
import struct
import tempfile
import unittest
import zlib

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("source_audit", HERE / "audit-source-binlog.py")
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)
from test_pitr_binlog import event, fixture, fde


def ddl(data, sql, database=b"new-api"):
    data += event(34, b"\0" * 42, len(data))
    payload = struct.pack("<IIBHH", 7, 0, len(database), 0, 0) + database + b"\0" + sql
    return data + event(2, payload, len(data))


class SourceBinlogAuditTests(unittest.TestCase):
    def run_audit(self, data, start, stop=None):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "binlog.000186"
            path.write_bytes(data)
            before = path.read_bytes()
            result = audit.audit(directory, {"file": path.name, "position": start},
                                 {"file": path.name, "position": len(data) if stop is None else stop})
            self.assertEqual(path.read_bytes(), before)
            self.assertEqual(list(Path(directory).iterdir()), [path])
            return result

    def test_active_file_stops_at_snapshot_and_ignores_future_partial_event(self):
        initial = fixture(1)
        value = ddl(initial, b"ALTER TABLE `users` ADD COLUMN x INT")
        result = self.run_audit(value + b"partial later write", len(initial), len(value))
        self.assertEqual(result["selected_transactions"], 1)
        self.assertEqual(result["selected_query_events"], 1)
        self.assertEqual(result["status"], "scope-verified")
        self.assertNotIn("ALTER", str(result))
        self.assertFalse(result["source_uuid_verified"])

    def test_no_default_account_and_preflight_ddl_are_allowed(self):
        initial = fixture(1)
        value = ddl(initial, b"CREATE USER 'fixture'@'%' IDENTIFIED BY 'not-real'", b"")
        value = ddl(value, b"CREATE DATABASE newapi_preflight_20261026000000_9", b"")
        value = ddl(value, b"CREATE TABLE `users` (id INT)", b"newapi_preflight_20261026000000_9")
        result = self.run_audit(value, len(initial))
        self.assertEqual(result["selected_transactions"], 3)
        self.assertNotIn("fixture", str(result))

    def test_business_default_account_and_cross_database_ddl_rejected(self):
        initial = fixture(1)
        for sql, database in ((b"CREATE USER 'fixture'@'%'", b"new-api"),
                              (b"ALTER TABLE `other`.`users` ADD x INT", b"new-api"),
                              (b"ALTER TABLE `new-api`.`users` ADD x INT", b"other")):
            with self.subTest(sql=sql), self.assertRaises(audit.restore.RestoreError):
                self.run_audit(ddl(initial, sql, database), len(initial))

    def test_bad_crc_anchor_and_snapshot_boundaries_fail_closed(self):
        initial = fixture(1)
        value = bytearray(ddl(initial, b"ALTER TABLE users ADD x INT"))
        value[-5] ^= 1
        with self.assertRaises(ValueError):
            self.run_audit(bytes(value), len(initial))
        value = ddl(initial, b"ALTER TABLE users ADD x INT")
        for start, stop in ((len(initial) - 1, len(value)), (len(initial), len(value) - 1),
                            (len(initial) - 31, len(value))):
            with self.subTest(start=start, stop=stop), self.assertRaises(audit.AuditError):
                self.run_audit(value, start, stop)

    def test_sequence_requires_all_files_and_matching_rotate(self):
        initial = fixture(1)
        first = initial + event(4, struct.pack("<Q", 4) + b"binlog.000187", len(initial))
        second = fixture(1)
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory)
            (path / "binlog.000186").write_bytes(first)
            start, stop = {"file": "binlog.000186", "position": len(initial)}, {"file": "binlog.000187", "position": len(second)}
            with self.assertRaises(OSError):
                audit.audit(path, start, stop)
            (path / "binlog.000187").write_bytes(second)
            result = audit.audit(path, start, stop)
            self.assertEqual(len(result["files"]), 2)
            self.assertEqual(result["selected_transactions"], 1)

    def test_huge_event_length_rejected_before_payload_read(self):
        initial = fixture(1)
        header = audit.pitr_binlog.HEADER.pack(1, 30, 1, audit.pitr_binlog.MAX_EVENT + 1, len(initial) + 19, 0)
        with self.assertRaises(audit.AuditError):
            self.run_audit(initial + header, len(initial))


if __name__ == "__main__":
    unittest.main()
