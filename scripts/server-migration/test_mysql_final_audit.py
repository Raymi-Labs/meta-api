"""No production access: final audit gates, query scoping and private diagnostics."""
import contextlib
import copy
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("mysql_final_audit", Path(__file__).with_name("mysql-final-audit.py"))
final = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(final)
SOURCE = "11111111-1111-1111-1111-111111111111"
TARGET = "22222222-2222-2222-2222-222222222222"
B = {"file": "binlog.000003", "position": 123}
STOP = {"file": "binlog.000004", "position": 456}


def evidence():
    tables = {name: {"kind": "BASE TABLE", "count": 2, "schema_sha256": "schema", "data_sha256": "data"}
              for name in ["logs", "users", *[f"t{index}" for index in range(74)]]}
    audit = {"database": "new-api", "database_settings": ["utf8mb4", "utf8mb4_unicode_ci"], "tables": tables, "objects": {}}
    ref = {"status": "success", "verification": "independent-snapshot-comparison", "source_uuid": SOURCE,
           "database": "new-api", "differences": [], "reference_audit": audit, "incremental_audit": copy.deepcopy(audit),
           "chain": {"stop": B}, "replay_window": {"after": {"logs": {"max_id": 10}}}, "backup_b_manifest_sha256": "a" * 64}
    receipt = {"status": "seed-verified", "format_version": 1, "source_uuid": SOURCE, "target_uuid": TARGET,
               "database": "new-api", "manifest_sha256": "a" * 64, "anchor": B, "table_count": 76,
               "audit_sha256": hashlib.sha256(final.audit.canonical(audit)).hexdigest()}
    chain = {"status": "scope-verified", "format_version": 1, "database": "new-api", "start": B,
             "stop": STOP, "server_ids": [0, 1], "files": [{"file": "binlog.000003"}]}
    return ref, receipt, chain


class Transport:
    def __init__(self):
        self.calls, self.guards = [], 0

    def guard(self):
        self.guards += 1

    def query(self, sql):
        self.calls.append(sql)
        if "COLUMN_NAME,DATA_TYPE" in sql:
            return [["id", "bigint"]]
        if "INDEX_NAME='PRIMARY'" in sql:
            return [["id"]]
        if "IS_VISIBLE='YES'" in sql:
            return [["idx_created_at_id"]]
        if "MAX(`id`)" in sql:
            return [[12]]
        if "COUNT(*)" in sql and "FORCE INDEX" in sql:
            return [[2 if " WHERE " in sql else 4]]
        if "COUNT(*)" in sql:
            return [[2]]
        return []

    def lines(self, sql):
        self.calls.append(sql)
        return iter((b"a" * 64 + b"\n", b"b" * 64 + b"\n"))


class FinalAuditTests(unittest.TestCase):
    def test_baseline_seed_and_range_evidence_must_bind_exactly(self):
        ref, receipt, chain = evidence()
        binding = final.bind_evidence(ref, receipt, chain, SOURCE, STOP)
        self.assertEqual(binding["logs_after_id"], 10)
        self.assertEqual(binding["source_server_id"], 1)
        receipt["audit_sha256"] = "d" * 64
        binding = final.bind_evidence(ref, receipt, chain, SOURCE, STOP)
        self.assertEqual(binding["seed_audit_sha256"], "d" * 64)
        self.assertNotEqual(binding["seed_audit_sha256"], binding["baseline_audit_sha256"])
        for index, field, value in ((0, "differences", ["users:data_sha256"]), (1, "audit_sha256", "wrong"),
                                    (1, "target_uuid", SOURCE), (2, "start", STOP), (2, "server_ids", [1, 2])):
            values = list(evidence())
            values[index][field] = value
            with self.subTest(field=field), self.assertRaises(final.seed.MigrationError):
                final.bind_evidence(*values, SOURCE, STOP)

    def test_logs_predicate_is_before_order_and_group_and_does_not_touch_other_tables(self):
        scoped = final.LogsSuffix(Transport(), 10)
        fingerprint = "SELECT SHA2(CAST(JSON_ARRAY(HEX(CAST(`id` AS BINARY))) AS BINARY),256) AS row_hash FROM `new-api`.`logs` ORDER BY `id`"
        self.assertIn(" WHERE `id`>10 ORDER BY `id`", scoped.rewrite(fingerprint))
        self.assertEqual(scoped.rewrite("SELECT JSON_ARRAY(COUNT(*)) FROM `new-api`.`logs`"),
                         "SELECT JSON_ARRAY(COUNT(*)) FROM `new-api`.`logs` WHERE `id`>10")
        financial = "SELECT JSON_ARRAY(`type`,COUNT(*),SUM(`quota`)) FROM `new-api`.`logs` GROUP BY `type` ORDER BY `type`"
        self.assertIn(" WHERE `id`>10 GROUP BY `type` ORDER BY `type`", scoped.rewrite(financial))
        for sql in ("SHOW CREATE TABLE `new-api`.`logs`", fingerprint.replace("`.`logs`", "`.`users`")):
            self.assertEqual(scoped.rewrite(sql), sql)

    def test_unexpected_logs_query_shapes_fail_closed(self):
        scoped = final.LogsSuffix(Transport(), 10)
        for sql in ("SELECT JSON_ARRAY(COUNT(*)) FROM `new-api`.`logs` WHERE id>0",
                    "SELECT JSON_ARRAY(COUNT(*)) FROM `new-api`.`logs`; DROP TABLE users",
                    "SELECT JSON_ARRAY(COUNT(*)) FROM `new-api`.`logs` JOIN users ON 1=1",
                    "SELECT * FROM `new-api`.`logs`"):
            with self.subTest(sql=sql), self.assertRaises(final.seed.MigrationError):
                scoped.rewrite(sql)

    def test_collection_scopes_suffix_only_and_counts_old_prefix_by_covering_index(self):
        ref, receipt, chain = evidence()
        binding = final.bind_evidence(ref, receipt, chain, SOURCE, STOP)
        transport = Transport()

        def fake_audit(query, lines, database):
            query("SELECT JSON_ARRAY(COUNT(*)) FROM `new-api`.`logs`")
            query("SELECT JSON_ARRAY(`type`,COUNT(*),SUM(`quota`)) FROM `new-api`.`logs` GROUP BY `type` ORDER BY `type`")
            list(lines("SELECT SHA2(value,256) AS row_hash FROM `new-api`.`logs` ORDER BY `id`"))
            list(lines("SELECT SHA2(value,256) AS row_hash FROM `new-api`.`users` ORDER BY `id`"))
            return copy.deepcopy(ref["reference_audit"])

        with patch.object(final.audit, "audit_database", side_effect=fake_audit):
            report = final.collect(transport, binding, STOP)
        self.assertEqual(transport.guards, 2)
        self.assertEqual(report["logs_coverage"]["old_prefix_count"], 2)
        self.assertEqual(report["logs_coverage"]["suffix_rows_hashed"], 2)
        self.assertFalse(report["logs_coverage"]["old_prefix_rehashed"])
        self.assertIn("UPDATE", report["limitations"][0])
        hashes = [sql for sql in transport.calls if " AS row_hash" in sql]
        self.assertIn(" WHERE `id`>10 ORDER BY", hashes[0])
        self.assertNotIn(" WHERE ", hashes[1])
        counts = [sql for sql in transport.calls if "FORCE INDEX" in sql]
        self.assertEqual(len(counts), 2)
        self.assertTrue(all("`idx_created_at_id`" in sql for sql in counts))
        self.assertIn(" WHERE `id`<=10", counts[1])

    def make_guard(self, role="source", override=None):
        db = final.FrozenDatabase(role, SOURCE if role == "source" else TARGET, SOURCE,
                                  "newapi-mysql", "a" * 64, "sha256:" + "b" * 64, STOP, 1)
        row = ["8.0.46", SOURCE if role == "source" else TARGET, 1 if role == "source" else 9,
               "OFF", "ROW", 1, 1, 1, 1]
        def query(sql):
            if "@@version" in sql:
                return [row]
            if "SCHEMATA" in sql:
                return [["new-api"]]
            return [[(override or {}).get("writers" if "innodb_trx" in sql else "replication", 0)]]
        db.query = query
        db.sql = lambda sql: b"binlog.000004\t456\t\t\t\n"
        status = {"Source_UUID": SOURCE, "Source_Server_Id": "1", "Replica_IO_Running": "No", "Replica_SQL_Running": "No",
                  "Last_IO_Errno": "0", "Last_SQL_Errno": "0", "Relay_Source_Log_File": "binlog.000004", "Exec_Source_Log_Pos": "456",
                  "Auto_Position": "0", "Replicate_Do_DB": "new-api"}
        db.replica_status = lambda: status
        return db, row, status

    def test_source_server_id_one_is_valid_but_uuid_readonly_and_writer_gates_are_required(self):
        db, row, _ = self.make_guard()
        db.guard()
        for position, value in ((1, TARGET), (5, 0), (6, 0), (7, 0), (8, 0)):
            db, row, _ = self.make_guard()
            row[position] = value
            with self.subTest(position=position), self.assertRaises(final.seed.MigrationError):
                db.guard()
        for change in ({"writers": 1}, {"replication": 1}):
            db, _, _ = self.make_guard(override=change)
            with self.assertRaises(final.seed.MigrationError):
                db.guard()

    def test_target_requires_stopped_threads_exact_execution_coordinate_and_scope(self):
        db, _, _ = self.make_guard("target")
        db.guard()
        for key, value in (("Source_UUID", TARGET), ("Exec_Source_Log_Pos", "455"), ("Replica_SQL_Running", "Yes"),
                           ("Last_SQL_Errno", "1062"), ("Replicate_Ignore_Table", "new-api.logs")):
            db, _, status = self.make_guard("target")
            status[key] = value
            with self.subTest(key=key), self.assertRaises(final.seed.MigrationError):
                db.guard()

    def test_compare_output_contains_only_table_names_and_reasons(self):
        ref, receipt, chain = evidence()
        shared = {"format_version": 1, "source_uuid": SOURCE, "final_coordinate": STOP,
                  "binding": {}, "evidence": {}, "scope": "bounded", "audit": ref["reference_audit"], "logs_coverage": {}}
        left = dict(copy.deepcopy(shared), role="source")
        right = dict(copy.deepcopy(shared), role="target")
        right["audit"]["tables"]["users"]["financial"] = {"private_balance": "999999-secret-private-value"}
        with tempfile.TemporaryDirectory() as directory:
            source, target = Path(directory) / "source.json", Path(directory) / "target.json"
            final.seed.write_json(source, left)
            final.seed.write_json(target, right)
            out, err = io.StringIO(), io.StringIO()
            with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
                code = final.main(["compare", "--source", str(source), "--target", str(target)])
            self.assertEqual(code, 1)
            self.assertEqual(json.loads(out.getvalue())["differences"], ["users:financial"])
            self.assertNotIn("999999-secret-private-value", out.getvalue() + err.getvalue())

    def test_private_evidence_hash_and_permissions_are_checked(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "evidence.json"
            path.write_text('{"private":"never-print-this"}')
            path.chmod(0o600)
            digest = hashlib.sha256(path.read_bytes()).hexdigest()
            self.assertEqual(final.read_bound(path, digest)["private"], "never-print-this")
            with self.assertRaises(final.seed.MigrationError):
                final.read_bound(path, "f" * 64)
            path.chmod(0o644)
            with self.assertRaises(final.seed.MigrationError):
                final.read_bound(path, digest)


@unittest.skipUnless(os.environ.get("NEWAPI_FINAL_AUDIT_MYSQL_IMAGE"), "opt-in isolated MySQL 8.0.46 exercise")
class RealFinalAuditTests(unittest.TestCase):
    def test_real_76_table_source_audit_and_logs_suffix_semantics(self):
        # IsolatedMySQL checks the exact tools image label, owns a unique volume
        # and container, and enforces network none with no ports or host binds.
        with final.seed.restore.IsolatedMySQL(os.environ["NEWAPI_FINAL_AUDIT_MYSQL_IMAGE"], binlog=True) as server:
            ddl = ["CREATE DATABASE `new-api` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;",
                   "CREATE TABLE `new-api`.logs(id BIGINT PRIMARY KEY,created_at BIGINT,type INT,quota INT,content TEXT, KEY idx_created_at_id(id,created_at)) ENGINE=InnoDB;",
                   "CREATE TABLE `new-api`.users(id BIGINT PRIMARY KEY,quota BIGINT) ENGINE=InnoDB;",
                   "INSERT INTO `new-api`.logs VALUES(2,1,2,900,'old-prefix-private'),(10,2,2,800,'at-boundary-private'),(11,3,2,20,'suffix-private'),(12,4,6,5,'refund-private');",
                   "INSERT INTO `new-api`.users VALUES(1,100),(2,200);"]
            ddl += [f"CREATE TABLE `new-api`.t{index}(id BIGINT PRIMARY KEY) ENGINE=InnoDB;" for index in range(74)]
            ddl += ["SET GLOBAL super_read_only=ON;"]
            server.exec(["mysql", "--user=root", "-e", "\n".join(ddl)])
            server_uuid, server_id = server.query("SELECT JSON_ARRAY(@@server_uuid,@@server_id)")[0]
            fields = server.exec(["mysql", "--user=root", "--batch", "--skip-column-names", "-e", "SHOW MASTER STATUS"]).decode().strip().split("\t")
            point = {"file": fields[0], "position": int(fields[1])}
            image_id = json.loads(server.run(["docker", "inspect", server.container_id]))[0]["Image"]
            transport = final.FrozenDatabase("source", server_uuid, server_uuid, "newapi-mysql", server.container_id, image_id, point, server_id)
            transport.assert_container = server.assert_owned
            result = final.collect(transport, {"logs_after_id": 10, "baseline_logs_count": 2}, point)
            self.assertEqual(len(result["audit"]["tables"]), 76)
            self.assertEqual(result["logs_coverage"]["total_count"], 4)
            self.assertEqual(result["logs_coverage"]["old_prefix_count"], 2)
            self.assertEqual(result["logs_coverage"]["max_id"], 12)
            self.assertEqual(result["audit"]["tables"]["logs"]["count"], 2)
            self.assertEqual(result["audit"]["tables"]["logs"]["financial"]["rows"], [[2, 1, "20.000000000000"], [6, 1, "5.000000000000"]])
            self.assertEqual(result["audit"]["tables"]["users"]["financial"]["rows"], [[2, "300.000000000000"]])
            self.assertNotIn("private", json.dumps(result))
            # Exercise real batch/header parsing with one stopped channel;
            # never start its receiver or connect to any external endpoint.
            server.exec(["mysql", "--user=root", "-e",
                         "CHANGE REPLICATION SOURCE TO SOURCE_HOST='127.0.0.1',SOURCE_PORT=13306,SOURCE_USER='fixture',SOURCE_LOG_FILE='binlog.000001',SOURCE_LOG_POS=4;"])
            status = transport.replica_status()
            self.assertEqual(status["Source_Host"], "127.0.0.1")
            self.assertEqual(status["Replica_IO_Running"], "No")
            self.assertEqual(status["Replica_SQL_Running"], "No")
            self.assertEqual(status["Last_IO_Error"], "")
            print(json.dumps({"mysql": "8.0.46", "tables": 76, "logs_full_count": 4,
                              "logs_prefix_count": 2, "logs_suffix_hashed": 2,
                              "source_guards_passed": True, "old_logs_prefix_rehashed": False}))


if __name__ == "__main__":
    unittest.main()
