import copy
import gzip
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import struct
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

import pitr_audit
import pitr_binlog
from test_pitr_binlog import event, fixture

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

    def test_server_generated_drop_comment_is_only_allowed_in_preflight_scope(self):
        database = b"newapi_preflight_20261026014932_1457482"
        sql = b"DROP TABLE IF EXISTS `_TMP_UNI_TOKENS_REF` /* GENERATED BY SERVER */"
        self.assertTrue(restore.safe_preflight_ddl(sql, database, "new-api"))
        # The general lexer and business-schema classifier remain unchanged.
        self.assertIsNone(restore.ddl_tokens(sql))
        self.assertFalse(restore.scoped_table_ddl(sql, b"new-api"))
        for default in (b"new-api", b"mysql", b"newapi_preflight_1_2",
                        b"newapi_preflight_20261026014932_0",
                        database + b"_extra", b"prefix_" + database):
            with self.subTest(default=default):
                self.assertFalse(restore.safe_preflight_ddl(sql, default, "new-api"))
        # Do not bypass the existing forbidden-business-identifier guard.
        self.assertFalse(restore.safe_preflight_ddl(sql, database, "_tmp_uni_tokens_ref"))

    def test_server_generated_drop_comment_does_not_generalize_sql_allowlist(self):
        database = b"newapi_preflight_20261026014932_1457482"
        comment = b" /* GENERATED BY SERVER */"
        statements = (
            b"DROP TABLE IF EXISTS `NEW-API`.`USERS`" + comment,
            b"DROP TABLE IF EXISTS `OTHER`.`USERS`" + comment,
            b"DROP TABLE IF EXISTS `" + database.upper() + b"`.`USERS`" + comment,
            b"DROP TABLE IF EXISTS `USERS`, `TOKENS`" + comment,
            b"DROP TABLE `USERS`" + comment,
            b"DROP TEMPORARY TABLE IF EXISTS `USERS`" + comment,
            b"DROP TABLE IF EXISTS USERS" + comment,
            b"DROP TABLE IF EXISTS `USERS-WITH-DASH`" + comment,
            b"DROP TABLE IF EXISTS `USERS``ESCAPED`" + comment,
            b"DROP TABLE IF EXISTS `USERS` /* GENERATED BY SERVER */; DROP DATABASE `NEW-API`",
            b"DROP TABLE IF EXISTS `USERS` /* GENERATED BY SERVER */;",
            b"DROP TABLE IF EXISTS /* GENERATED BY SERVER */ `USERS`",
            b"DROP TABLE IF EXISTS `USERS` /*! GENERATED BY SERVER */",
            b"DROP TABLE IF EXISTS `USERS` /*+ GENERATED BY SERVER */",
            b"DROP TABLE IF EXISTS `USERS` /* GENERATED BY SERVER EXTRA */",
            b"DROP TABLE IF EXISTS `USERS` /* OTHER */",
            b"DROP TABLE IF EXISTS `USERS`" + comment + comment,
            b"DROP TABLE IF EXISTS `USERS`" + comment + b" -- OTHER",
            b"DROP TABLE IF EXISTS `USERS`" + comment + b" # OTHER",
            b"CREATE TABLE `USERS` (ID INT)" + comment,
            b"ALTER TABLE `USERS` ADD COLUMN X INT" + comment,
            b"DROP DATABASE IF EXISTS `NEW-API`" + comment,
        )
        for sql in statements:
            with self.subTest(sql=sql):
                self.assertFalse(restore.safe_preflight_ddl(sql, database, "new-api"))

    def test_server_generated_drop_query_event_preserves_scope_validation(self):
        sql = b"DROP TABLE IF EXISTS `_tmp_uni_tokens_ref` /* generated by server */"
        for database, allowed in ((b"newapi_preflight_20261026014932_1457482", True),
                                  (b"new-api", False), (b"other", False)):
            with self.subTest(database=database):
                data = fixture(0)
                anchor = {"file": "binlog.000001", "position": len(data)}
                data += event(34, b"\0" * 42, len(data))
                payload = struct.pack("<IIBHH", 7, 0, len(database), 0, 0) + database + b"\0" + sql
                data += event(2, payload, len(data))
                target = dict(anchor, position=len(data))
                parsed = pitr_binlog.parse_chunk(data, anchor["file"], 0)
                self.assertIn(target["position"], parsed["boundaries"])
                if allowed:
                    restore.validate_query_scope(data, anchor["file"], 0, anchor, target, "new-api")
                else:
                    with self.assertRaises(restore.RestoreError):
                        restore.validate_query_scope(data, anchor["file"], 0, anchor, target, "new-api")

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


class ReaderResourceTests(unittest.TestCase):
    def remote(self, data, error=None):
        child = mock.Mock(stdout=io.BytesIO(data))
        remote = mock.Mock()
        remote.reader.return_value = child
        remote.processes.check.side_effect = error
        return remote, child

    def test_metadata_success_closes_pipe_after_validation(self):
        remote, child = self.remote(b'{"verified":true}')
        self.assertEqual(restore.read_json(remote, "fixture"), ({"verified": True}, b'{"verified":true}'))
        self.assertTrue(child.stdout.closed)
        remote.processes.check.assert_called_once_with(child, "remote metadata read failed")

    def test_metadata_errors_close_pipe_without_changing_diagnostics(self):
        cases = ((b"invalid", None, restore.RestoreError, "invalid remote JSON metadata"),
                 (b"[]", None, restore.RestoreError, "invalid remote JSON metadata"),
                 (b"x" * (restore.BLOCK + 1), None, restore.RestoreError, "exceeds the limit"),
                 (b"{}", restore.backup.BackupError("remote metadata read failed"),
                  restore.backup.BackupError, "remote metadata read failed"))
        for data, error, expected, message in cases:
            with self.subTest(message=message, size=len(data)):
                remote, child = self.remote(data, error)
                with self.assertRaisesRegex(expected, message):
                    restore.read_json(remote, "fixture")
                self.assertTrue(child.stdout.closed)
                if len(data) > restore.BLOCK:
                    child.terminate.assert_called_once()

    def test_download_success_and_integrity_or_process_errors_close_pipe(self):
        expected = {"size": 3, "sha256": hashlib.sha256(b"abc").hexdigest()}
        cases = ((b"abc", None, None, None),
                 (b"abcd", None, restore.RestoreError, "exceeds recorded size"),
                 (b"ab", None, restore.RestoreError, "fingerprint mismatch"),
                 (b"xyz", None, restore.RestoreError, "fingerprint mismatch"),
                 (b"abc", restore.backup.BackupError("remote object read failed"),
                  restore.backup.BackupError, "remote object read failed"))
        with tempfile.TemporaryDirectory() as directory:
            for number, (data, error, failure, message) in enumerate(cases):
                with self.subTest(number=number):
                    remote, child = self.remote(data, error)
                    path = Path(directory) / str(number)
                    if failure is None:
                        restore.download(remote, "fixture", path, expected)
                        self.assertEqual(path.read_bytes(), data)
                        self.assertEqual(path.stat().st_mode & 0o777, 0o600)
                    else:
                        with self.assertRaisesRegex(failure, message):
                            restore.download(remote, "fixture", path, expected)
                    self.assertTrue(child.stdout.closed)

    def test_download_local_file_errors_close_pipe(self):
        expected = {"size": 3, "sha256": hashlib.sha256(b"abc").hexdigest()}
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "existing"
            path.write_bytes(b"keep")
            remote, child = self.remote(b"abc")
            with self.assertRaises(FileExistsError):
                restore.download(remote, "fixture", path, expected)
            self.assertTrue(child.stdout.closed)
            self.assertEqual(path.read_bytes(), b"keep")
            remote, child = self.remote(b"abc")
            with mock.patch.object(restore.os, "fsync", side_effect=OSError("fixture fsync failure")):
                with self.assertRaisesRegex(OSError, "fixture fsync failure"):
                    restore.download(remote, "fixture", Path(directory) / "new", expected)
            self.assertTrue(child.stdout.closed)

    def test_read_exception_and_interruption_close_both_reader_types(self):
        for failure in (OSError("fixture pipe failure"), restore.backup.BackupError("backup was interrupted")):
            for operation in ("metadata", "download"):
                with self.subTest(operation=operation, error=str(failure)), tempfile.TemporaryDirectory() as directory:
                    class BrokenStream(io.BytesIO):
                        def read(self, _size):
                            raise failure
                    remote, child = self.remote(b"")
                    child.stdout = BrokenStream()
                    with self.assertRaises(type(failure)) as caught:
                        if operation == "metadata":
                            restore.read_json(remote, "fixture")
                        else:
                            restore.download(remote, "fixture", Path(directory) / "segment",
                                             {"size": 3, "sha256": hashlib.sha256(b"abc").hexdigest()})
                    self.assertIs(caught.exception, failure)
                    self.assertTrue(child.stdout.closed)

    @unittest.skipUnless(sys.platform.startswith("linux"), "requires Linux RLIMIT_NOFILE")
    def test_long_chain_real_subprocess_readers_under_low_fd_limit(self):
        # Keep the resource limit in a subprocess so unrelated tests are not
        # affected. Retain every Popen just as the real Processes owner does.
        script = r'''
import hashlib, importlib.util, json, resource, subprocess, sys, tempfile
from pathlib import Path
sys.path.insert(0, sys.argv[1])
spec = importlib.util.spec_from_file_location("restore_fd_test", Path(sys.argv[1]) / "mysql-pitr-restore.py")
restore = importlib.util.module_from_spec(spec)
spec.loader.exec_module(restore)
resource.setrlimit(resource.RLIMIT_NOFILE, (64, resource.getrlimit(resource.RLIMIT_NOFILE)[1]))
data = b"{}\n"
expected = {"size": len(data), "sha256": hashlib.sha256(data).hexdigest()}
with tempfile.TemporaryDirectory() as directory, restore.backup.Processes() as processes:
    class Remote:
        def __init__(self):
            self.processes = processes
        def reader(self, _path):
            return processes.start([sys.executable, "-B", "-c", "print('{}')"],
                                   stdin=subprocess.DEVNULL, stdout=subprocess.PIPE)
    remote = Remote()
    for number in range(256):
        assert restore.read_json(remote, "fixture:receipt") == ({}, data)
        path = Path(directory) / str(number)
        restore.download(remote, "fixture:segment", path, expected)
        assert path.read_bytes() == data
        path.unlink()
    assert len(processes.children) == 512
    assert all(child.stdout.closed and child.poll() == 0 for child in processes.children)
    print(json.dumps({"readers": len(processes.children), "open_stdout": 0, "soft_nofile": 64}))
'''
        result = subprocess.run([sys.executable, "-B", "-c", script, str(Path(restore.__file__).parent)],
                                stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=60)
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assertEqual(json.loads(result.stdout), {"readers": 512, "open_stdout": 0, "soft_nofile": 64})


class AuditTests(unittest.TestCase):
    def test_identifier_quotes_and_binary_row_schema(self):
        self.assertEqual(pitr_audit.ident("a`b"), "`a``b`")
        self.assertEqual(pitr_audit.normalize_create("id INT) ENGINE=InnoDB AUTO_INCREMENT=123 DEFAULT CHARSET=utf8mb4"), "id INT) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4")
        self.assertIn("AUTO_INCREMENT=123", pitr_audit.normalize_create("note VARCHAR(60) DEFAULT ' AUTO_INCREMENT=123'"))

    def test_redundant_column_charset_matches_restore_without_hiding_collation(self):
        source = "CREATE TABLE `t` (\n `name` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,\n `note` text COLLATE latin1_swedish_ci\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci"
        restored = source.replace("varchar(255) COLLATE", "varchar(255) CHARACTER SET utf8mb4 COLLATE").replace("text COLLATE", "text CHARACTER SET latin1 COLLATE")
        self.assertEqual(pitr_audit.normalize_create(source), pitr_audit.normalize_create(restored))
        self.assertNotEqual(hashlib.sha256(source.encode()).hexdigest(), hashlib.sha256(restored.encode()).hexdigest())
        changed = restored.replace("utf8mb4_unicode_ci DEFAULT", "utf8mb4_bin DEFAULT")
        self.assertNotEqual(pitr_audit.normalize_create(source), pitr_audit.normalize_create(changed))
        self.assertEqual(pitr_audit.normalize_create("c TEXT character set UTF8MB4 COLLATE utf8mb4_unicode_ci"), "c TEXT COLLATE utf8mb4_unicode_ci")

    def test_charset_normalization_requires_adjacent_matching_unquoted_collation(self):
        unchanged = (
            "c TEXT CHARACTER SET utf8mb4",
            "c TEXT CHARACTER SET latin1 COLLATE utf8mb4_unicode_ci",
            "c TEXT CHARACTER SET utf8mb3 COLLATE utf8_general_ci",
            "c TEXT CHARACTER SET utf8mb4 NOT NULL COLLATE utf8mb4_unicode_ci",
            "c TEXT CHARACTER SET utf8mb4 /* retained */ COLLATE utf8mb4_unicode_ci",
            "c TEXT CHARACTER SET `utf8mb4` COLLATE utf8mb4_unicode_ci",
            "c TEXT CHARACTER SET utf8mb4 COLLATE `utf8mb4_unicode_ci`",
            "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci",
        )
        for ddl in unchanged:
            with self.subTest(ddl=ddl):
                self.assertEqual(pitr_audit.normalize_create(ddl), ddl)
        a = "c TEXT CHARACTER SET latin1 COLLATE latin1_swedish_ci"
        b = "c TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"
        self.assertNotEqual(pitr_audit.normalize_create(a), pitr_audit.normalize_create(b))

    def test_charset_normalization_preserves_literals_identifiers_and_comments(self):
        phrase = "CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"
        allocator = "ENGINE=InnoDB AUTO_INCREMENT=123"
        protected = (
            "DEFAULT '" + phrase + "'",
            'DEFAULT "' + phrase + '"',
            "COMMENT '" + phrase + " " + allocator + "'",
            "enum('" + phrase + "', 'other')",
            "set('" + phrase + "', 'other')",
            "GENERATED ALWAYS AS (concat('" + phrase + "', `other`))",
            "`identifier " + phrase + "` varchar(30)",
            "`doubled`` " + phrase + "` varchar(30)",
            "DEFAULT 'doubled'' " + phrase + "'",
            "DEFAULT 'escaped\\' " + phrase + "'",
            'DEFAULT "escaped\\\" ' + phrase + '"',
            "/* " + phrase + " " + allocator + " */",
            "/*!80000 " + phrase + " */",
            "-- " + phrase + " " + allocator + "\n",
            "# " + phrase + " " + allocator + "\r\n",
            "DEFAULT 'unterminated " + phrase,
            "/* unterminated " + phrase,
        )
        for value in protected:
            with self.subTest(value=value):
                ddl = "c TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci " + value
                self.assertEqual(pitr_audit.normalize_create(ddl), "c TEXT COLLATE utf8mb4_unicode_ci " + value)

    def test_normalization_continues_after_opaque_spans_and_is_idempotent(self):
        ddl = "CREATE TABLE `t` (`a` varchar(10) DEFAULT 'ENGINE=InnoDB AUTO_INCREMENT=123', /* opaque */\n `b` text CHARACTER SET utf8mb4 COLLATE utf8mb4_bin,\n -- CHARACTER SET latin1 COLLATE latin1_bin\n `c` text CHARACTER SET latin1 COLLATE latin1_bin) ENGINE=InnoDB AUTO_INCREMENT=987 DEFAULT CHARSET=utf8mb4"
        normalized = pitr_audit.normalize_create(ddl)
        self.assertIn("DEFAULT 'ENGINE=InnoDB AUTO_INCREMENT=123'", normalized)
        self.assertIn("-- CHARACTER SET latin1 COLLATE latin1_bin", normalized)
        self.assertIn("`b` text COLLATE utf8mb4_bin", normalized)
        self.assertIn("`c` text COLLATE latin1_bin", normalized)
        self.assertNotIn("AUTO_INCREMENT=987", normalized)
        self.assertEqual(pitr_audit.normalize_create(normalized), normalized)

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
