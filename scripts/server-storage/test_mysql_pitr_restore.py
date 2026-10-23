import copy
import gzip
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import pitr_audit
import pitr_binlog
from test_pitr_binlog import fixture

spec = importlib.util.spec_from_file_location("pitr_restore", Path(__file__).with_name("mysql-pitr-restore.py"))
restore = importlib.util.module_from_spec(spec)
spec.loader.exec_module(restore)

SOURCE = "12345678-1234-1234-1234-123456789abc"


class Reader:
    def __init__(self, data):
        self.stdout = io.BytesIO(data)


class Remote:
    env = {}
    def __init__(self, objects):
        self.objects = objects
        self.processes = self

    def reader(self, path):
        if path not in self.objects:
            raise restore.RestoreError("object is missing")
        return Reader(self.objects[path])

    def check(self, *args):
        pass

    def command(self, *args):
        return args

    def capture(self, args, **kwargs):
        return ("\n".join(sorted(path.split("/")[-1] for path in self.objects if path.startswith(args[1] + "/"))) + "\n").encode()


def make_archive(data=None):
    data = data or fixture(2)
    root = {"schema": 1, "uuid": SOURCE, "start_file": "binlog.000001", "normalization": "fde-clear-in-use-v1"}
    parsed = pitr_binlog.parse_chunk(data, root["start_file"], 0)
    digest = hashlib.sha256(parsed["data"]).hexdigest()
    receipt = {"seq": 1, "source_uuid": SOURCE, "file": root["start_file"], "start": 0, "end": len(data), "size": len(data),
               "sha256": digest, "object": pitr_binlog.segment_name(1, root["start_file"], 0, len(data), digest),
               "previous_receipt_sha256": None, "previous_file": None, "parser_state": parsed["state"],
               "source_sample": {"file": root["start_file"], "position": len(data), "at": "2026-10-23T00:00:00Z"},
               "last_complete_transaction_frontier": parsed["last_complete_transaction_frontier"], "verified_at": "2026-10-23T00:00:00Z"}
    objects = {"crypt:epoch/root.json": pitr_binlog.canonical_json(root),
               "crypt:epoch/receipts/000000000001.json": pitr_binlog.canonical_json(receipt),
               "crypt:epoch/" + receipt["object"]: parsed["data"]}
    start = {"file": root["start_file"], "position": parsed["boundaries"][0]}
    stop = {"file": root["start_file"], "position": parsed["boundaries"][-1]}
    return objects, receipt, start, stop


def make_archive_before_anchor():
    objects, previous, _, _ = make_archive()
    file = "binlog.000002"
    parsed = pitr_binlog.parse_chunk(fixture(2), file, 0, previous["parser_state"])
    data = parsed["data"]
    digest = hashlib.sha256(data).hexdigest()
    receipt = dict(previous, seq=2, file=file, end=len(data), size=len(data), sha256=digest,
                   object=pitr_binlog.segment_name(2, file, 0, len(data), digest),
                   previous_receipt_sha256=pitr_binlog.receipt_digest(previous), previous_file=previous["file"],
                   parser_state=parsed["state"], last_complete_transaction_frontier=parsed["last_complete_transaction_frontier"],
                   source_sample=dict(previous["source_sample"], file=file, position=len(data)))
    objects["crypt:epoch/receipts/000000000002.json"] = pitr_binlog.canonical_json(receipt)
    objects["crypt:epoch/" + receipt["object"]] = data
    return objects, previous, receipt, {"file": file, "position": parsed["boundaries"][0]}, {"file": file, "position": parsed["boundaries"][-1]}


class RestoreTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name).resolve()

    def tearDown(self):
        self.temp.cleanup()

    def reconstruct(self, objects, start, stop, source=SOURCE):
        return restore.reconstruct(Remote(objects), "crypt:epoch", self.root, source, start, stop)

    def test_real_parser_crc_receipt_and_boundaries(self):
        objects, receipt, start, stop = make_archive()
        files, report = self.reconstruct(objects, start, stop)
        self.assertEqual(files[0].read_bytes(), objects["crypt:epoch/" + receipt["object"]])
        self.assertEqual(report["stop"], stop)

    def test_before_anchor_files_are_verified_without_retaining_history(self):
        objects, previous, receipt, start, stop = make_archive_before_anchor()
        files, report = self.reconstruct(objects, start, stop)
        self.assertEqual([path.name for path in files], [start["file"]])
        self.assertFalse((self.root / previous["file"]).exists())
        self.assertEqual(files[0].read_bytes(), objects["crypt:epoch/" + receipt["object"]])
        self.assertEqual(len(report["verified_receipts"]), 2)
        self.assertEqual(list(self.root.glob("segment-*")), [])

    def test_rehashed_crc_corruption_before_anchor_is_still_rejected(self):
        objects, previous, receipt, start, stop = make_archive_before_anchor()
        data = bytearray(objects.pop("crypt:epoch/" + previous["object"]))
        data[-8] ^= 1
        previous["sha256"] = hashlib.sha256(data).hexdigest()
        previous["object"] = pitr_binlog.segment_name(1, previous["file"], 0, len(data), previous["sha256"])
        objects["crypt:epoch/" + previous["object"]] = bytes(data)
        objects["crypt:epoch/receipts/000000000001.json"] = pitr_binlog.canonical_json(previous)
        receipt["previous_receipt_sha256"] = pitr_binlog.receipt_digest(previous)
        objects["crypt:epoch/receipts/000000000002.json"] = pitr_binlog.canonical_json(receipt)
        with self.assertRaisesRegex(restore.RestoreError, "CRC"):
            self.reconstruct(objects, start, stop)

    def test_corrupt_data_is_rejected_before_mysql(self):
        objects, receipt, start, stop = make_archive()
        objects["crypt:epoch/" + receipt["object"]] += b"x"
        with self.assertRaises(restore.RestoreError):
            self.reconstruct(objects, start, stop)

    def test_rehashed_crc_corruption_still_rejected(self):
        objects, receipt, start, stop = make_archive()
        data = bytearray(objects.pop("crypt:epoch/" + receipt["object"]))
        data[-8] ^= 1
        receipt["sha256"] = hashlib.sha256(data).hexdigest()
        receipt["object"] = pitr_binlog.segment_name(1, receipt["file"], 0, len(data), receipt["sha256"])
        objects["crypt:epoch/" + receipt["object"]] = bytes(data)
        objects["crypt:epoch/receipts/000000000001.json"] = pitr_binlog.canonical_json(receipt)
        with self.assertRaisesRegex(restore.RestoreError, "CRC"):
            self.reconstruct(objects, start, stop)

    def test_middle_of_transaction_and_byte_rejected(self):
        for subtraction in (1, 31):
            with self.subTest(subtraction=subtraction):
                with tempfile.TemporaryDirectory() as work:
                    objects, _, start, stop = make_archive()
                    stop["position"] -= subtraction
                    with self.assertRaises(restore.RestoreError):
                        restore.reconstruct(Remote(objects), "crypt:epoch", Path(work), SOURCE, start, stop)

    def test_missing_receipt_rejected(self):
        objects, _, start, stop = make_archive()
        objects["crypt:epoch/receipts/000000000002.json"] = objects.pop("crypt:epoch/receipts/000000000001.json")
        with self.assertRaisesRegex(restore.RestoreError, "gap"):
            self.reconstruct(objects, start, stop)

    def test_missing_segment_rejected(self):
        objects, receipt, start, stop = make_archive()
        objects.pop("crypt:epoch/" + receipt["object"])
        with self.assertRaises(restore.RestoreError):
            self.reconstruct(objects, start, stop)

    def test_source_uuid_mismatch_rejected(self):
        objects, _, start, stop = make_archive()
        with self.assertRaisesRegex(restore.RestoreError, "source"):
            self.reconstruct(objects, start, stop, "00000000-0000-0000-0000-000000000000")

    def test_database_scope_mismatch_rejected(self):
        objects, _, start, stop = make_archive()
        with self.assertRaisesRegex(restore.RestoreError, "scope"):
            restore.reconstruct(Remote(objects), "crypt:epoch", self.root, SOURCE, start, stop, "new-api")

    def test_explicit_position_requires_complete_unambiguous_pair(self):
        common = ["--backup-a", "vault:a", "--epoch", "vault:epoch", "--rclone-config", "/private/config", "--work-dir", "/private/new"]
        with mock.patch("sys.stderr", new=io.StringIO()):
            for extra in (["--stop-file", "binlog.000001"], ["--stop-position", "123"],
                          ["--backup-b", "vault:b", "--stop-file", "binlog.000001", "--stop-position", "123"]):
                with self.assertRaises(SystemExit):
                    restore.parse_args(common + extra)
        parsed = restore.parse_args(common + ["--stop-file", "binlog.000001", "--stop-position", "123"])
        self.assertEqual(parsed.stop_position, 123)

    def test_forged_parser_state_rejected(self):
        objects, receipt, start, stop = make_archive()
        receipt["parser_state"]["in_transaction"] = True
        objects["crypt:epoch/receipts/000000000001.json"] = pitr_binlog.canonical_json(receipt)
        with self.assertRaisesRegex(restore.RestoreError, "parser state"):
            self.reconstruct(objects, start, stop)

    def test_release_preflight_ddl_is_narrowly_recognized(self):
        database = b"newapi_preflight_20261023000000_12345"
        for sql in (b"CREATE DATABASE `NEWAPI_PREFLIGHT_20261023000000_12345` CHARACTER SET UTF8MB4 COLLATE UTF8MB4_UNICODE_CI",
                    b"DROP DATABASE IF EXISTS `NEWAPI_PREFLIGHT_20261023000000_12345`",
                    b"ALTER TABLE `FIXTURE` ADD COLUMN NOTE VARCHAR(100) DEFAULT 'NEW-API'",
                    b"CREATE UNIQUE INDEX `I` ON `FIXTURE` (`ID`)"):
            self.assertTrue(restore.safe_preflight_ddl(sql, database, "new-api"), sql)
        for sql in (b"ALTER TABLE `NEW-API`.`USERS` ADD COLUMN BAD INT",
                    b"CREATE TABLE X AS SELECT * FROM MYSQL.USER",
                    b"CREATE VIEW X AS SELECT 1", b"ALTER TABLE X /* HIDDEN */ ADD Y INT",
                    b'ALTER TABLE "NEW-API"."USERS" ADD Y INT', b"DROP DATABASE `NEW-API`",
                    b"CREATE DATABASE NEWAPI_PREFLIGHT_1_2", b"CREATE DATABASE NEWAPI_PREFLIGHT_20261023000000_0",
                    b"ALTER TABLE X ADD Y INT; DROP DATABASE `NEW-API`"):
            self.assertFalse(restore.safe_preflight_ddl(sql, database, "new-api"), sql)

    def test_business_ddl_rejects_qualified_other_database_and_ambiguity(self):
        for sql in (b"ALTER TABLE `NEW-API`.`USERS` ADD Y INT", b"CREATE TABLE USERS (ID INT,NOTE VARCHAR(100) DEFAULT 'mysql.user')"):
            self.assertTrue(restore.scoped_table_ddl(sql, b"new-api"), sql)
        for sql in (b"ALTER TABLE `MYSQL`.`USER` ADD Y INT", b"RENAME TABLE USERS TO MYSQL.USERS",
                    b"CREATE TABLE USERS AS SELECT * FROM MYSQL.USER", b"CREATE DATABASE EVIL",
                    b"ALTER TABLE USERS /* HIDDEN */ ADD Y INT", b"CREATE PROCEDURE P() BEGIN SELECT 1; END"):
            self.assertFalse(restore.scoped_table_ddl(sql, b"new-api"), sql)

    def test_restore_target_has_no_existing_container_option(self):
        with mock.patch("sys.stderr", new=io.StringIO()), self.assertRaises(SystemExit):
            restore.parse_args(["--container", "newapi-mysql"])

    def test_existing_work_directory_is_rejected_before_docker(self):
        args = mock.Mock(work_dir=str(self.root))
        with mock.patch.object(restore.IsolatedMySQL, "run") as docker:
            with self.assertRaises(restore.RestoreError):
                restore.execute(args)
            docker.assert_not_called()

    def test_failed_preflight_does_not_change_process_umask(self):
        previous = os.umask(0o022)
        try:
            with self.assertRaises(restore.RestoreError):
                restore.execute(mock.Mock(work_dir=str(self.root)))
            actual = os.umask(0o022)
            self.assertEqual(actual, 0o022)
        finally:
            os.umask(previous)

    def test_container_network_or_bind_change_blocks_operation(self):
        server = restore.IsolatedMySQL()
        server.container_id = "owned-id"
        good = {"Id": "owned-id", "Config": {"Labels": {restore.LABEL: server.run_id}},
                "HostConfig": {"NetworkMode": "none", "PortBindings": {}},
                "Mounts": [{"Type": "volume", "Name": server.volume, "Destination": "/var/lib/mysql"}]}
        for mutate in (lambda x: x["HostConfig"].update(NetworkMode="bridge"),
                       lambda x: x["Mounts"][0].update(Type="bind"),
                       lambda x: x["Config"]["Labels"].update({restore.LABEL: "other"}),
                       lambda x: x["HostConfig"].update(PortBindings={"3306/tcp": [{}]})):
            bad = copy.deepcopy(good)
            mutate(bad)
            with mock.patch.object(server, "run", return_value=json.dumps([bad]).encode()):
                with self.assertRaises(restore.RestoreError):
                    server.assert_owned()

    def test_missing_success_hash_and_header_mismatch_rejected(self):
        sql = b"-- CHANGE MASTER TO MASTER_LOG_FILE='binlog.000001', MASTER_LOG_POS=123;\n\n-- Current Database: `new-api`\n"
        data = gzip.compress(sql)
        source = {"binlog_anchor": {"file": "binlog.000001", "position": 123, "server_uuid": SOURCE},
                  "server_uuid": SOURCE, "mysql_version": "8.0.46", "database_scope": ["new-api"], "database": "new-api"}
        manifest = {"run_id": "run", "source": source, "scope": "database-only", "dump": {"name": "mysql.sql.gz", "size": len(data), "sha256": hashlib.sha256(data).hexdigest()}}
        raw = json.dumps(manifest).encode()
        marker = {"run_id": "run", "format_version": 1, "status": "success", "manifest_sha256": hashlib.sha256(raw).hexdigest()}
        objects = {"crypt:a/manifest.json": raw, "crypt:a/mysql.sql.gz": data, "crypt:a/SUCCESS.json": json.dumps(marker).encode()}
        loaded = restore.load_backup(Remote(objects), "crypt:a", self.root)
        self.assertEqual(loaded["anchor"]["position"], 123)
        self.root.joinpath("mysql.sql.gz").unlink()
        source["binlog_anchor"]["position"] = 124
        objects["crypt:a/manifest.json"] = json.dumps(manifest).encode()
        marker["manifest_sha256"] = hashlib.sha256(objects["crypt:a/manifest.json"]).hexdigest()
        objects["crypt:a/SUCCESS.json"] = json.dumps(marker).encode()
        with self.assertRaisesRegex(restore.RestoreError, "coordinate"):
            restore.load_backup(Remote(objects), "crypt:a", self.root)


class AuditTests(unittest.TestCase):
    def test_identifier_quotes_and_binary_row_schema(self):
        self.assertEqual(pitr_audit.ident("a`b"), "`a``b`")
        self.assertEqual(pitr_audit.normalize_create("id INT) ENGINE=InnoDB AUTO_INCREMENT=123 DEFAULT CHARSET=utf8mb4"), "id INT) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4")
        self.assertIn("AUTO_INCREMENT=123", pitr_audit.normalize_create("note VARCHAR(60) DEFAULT ' AUTO_INCREMENT=123'"))

    def test_financial_and_schema_drift_is_detected(self):
        base = {"database": "new-api", "tables": {"users": {"kind": "BASE TABLE", "schema_sha256": "a", "count": 1, "data_sha256": "b", "financial": {"rows": [[1, "100"]]}}}, "objects": {}}
        other = copy.deepcopy(base)
        self.assertEqual(pitr_audit.compare_audits(base, other), [])
        other["tables"]["users"]["financial"]["rows"][0][1] = "101"
        self.assertEqual(pitr_audit.compare_audits(base, other), ["users:financial"])
        other = copy.deepcopy(base)
        other["database_settings"] = ["utf8mb4", "utf8mb4_unicode_ci"]
        self.assertEqual(pitr_audit.compare_audits(base, other), ["database-settings"])


if __name__ == "__main__":
    unittest.main()
