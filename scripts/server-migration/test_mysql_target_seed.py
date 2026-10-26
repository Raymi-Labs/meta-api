"""No production connections: pure gate tests and opt-in disposable MySQL tests."""
import copy
import gzip
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import secrets
import io
import subprocess
import tempfile
import time
import unittest
from unittest.mock import patch
import uuid

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("target_seed", HERE / "mysql-target-seed.py")
seed = importlib.util.module_from_spec(spec)
spec.loader.exec_module(seed)
SOURCE = "11111111-1111-1111-1111-111111111111"
TARGET = "22222222-2222-2222-2222-222222222222"
ANCHOR = {"file": "binlog.000186", "position": 265107181}


def receipt():
    return {"format_version": 1, "status": "seed-verified", "source_uuid": SOURCE,
            "target_uuid": TARGET, "database": "new-api", "anchor": ANCHOR,
            "manifest_sha256": "a" * 64, "dump_sha256": "b" * 64}


class FakeTarget:
    source_uuid, target_uuid = SOURCE, TARGET
    def __init__(self, failure=None):
        self.calls, self.failure = [], failure
    def guard(self, **kwargs):
        self.calls.append(("guard", kwargs))
    def sql(self, sql):
        self.calls.append(("sql", sql))
    def load(self, path):
        self.calls.append(("load", str(path)))
        if self.failure:
            raise self.failure
    def lock_writes(self):
        self.calls.append(("lock_writes", None))
    def query(self, sql):
        return [[0]]
    def lines(self, sql):
        return iter(())


class GateTests(unittest.TestCase):
    def bundle(self, directory):
        path = Path(directory)
        dump = gzip.compress(b"-- CHANGE MASTER TO MASTER_LOG_FILE='binlog.000186', MASTER_LOG_POS=265107181;\n\n-- Current Database: `new-api`\nCREATE DATABASE `new-api`;\n")
        (path / "mysql.sql.gz").write_bytes(dump)
        (path / "mysql.sql.gz").chmod(0o600)
        manifest = {"run_id": "fixture", "scope": "database-only", "source": {
            "server_uuid": SOURCE, "mysql_version": "8.0.46", "database": "new-api",
            "database_scope": ["new-api"], "binlog_anchor": dict(ANCHOR, server_uuid=SOURCE)},
            "dump": {"name": "mysql.sql.gz", "size": len(dump), "sha256": hashlib.sha256(dump).hexdigest()}}
        seed.write_json(path / "manifest.json", manifest)
        digest = hashlib.sha256((path / "manifest.json").read_bytes()).hexdigest()
        seed.write_json(path / "SUCCESS.json", {"format_version": 1, "status": "success", "run_id": "fixture", "manifest_sha256": digest})
        return digest

    def test_verified_bundle_binds_marker_gzip_and_dump_anchor(self):
        with tempfile.TemporaryDirectory() as directory:
            digest = self.bundle(directory)
            value = seed.validate_bundle(directory, SOURCE, digest)
            self.assertEqual(value["anchor"], ANCHOR)
            self.assertEqual(value["source_uuid"], SOURCE)
            with self.assertRaises(seed.MigrationError):
                seed.validate_bundle(directory, TARGET, digest)
            with self.assertRaises(seed.MigrationError):
                seed.validate_bundle(directory, SOURCE, "c" * 64)

    def test_dump_corruption_and_false_marker_fail_before_import(self):
        with tempfile.TemporaryDirectory() as directory:
            digest = self.bundle(directory)
            marker = Path(directory) / "SUCCESS.json"
            data = json.loads(marker.read_bytes()); data["run_id"] = "different"
            marker.write_text(json.dumps(data))
            with self.assertRaises(seed.MigrationError):
                seed.validate_bundle(directory, SOURCE, digest)
            data["run_id"] = "fixture"; marker.write_text(json.dumps(data))
            dump = Path(directory) / "mysql.sql.gz"
            dump.write_bytes(dump.read_bytes()[:-3])
            with self.assertRaises((seed.MigrationError, seed.restore.backup.BackupError, OSError, EOFError)):
                seed.validate_bundle(directory, SOURCE, digest)

    def test_same_source_target_is_rejected(self):
        with self.assertRaises(seed.MigrationError):
            seed.Target(SOURCE, SOURCE)

    def test_account_safe_filter_and_fixed_filepos_without_reset(self):
        connection = {"host": "172.30.0.1", "port": 13306, "user": "newapi_migrate_fixture",
                      "password": "secret'\\test"}
        sql = seed.replication_sql(receipt(), connection)
        self.assertIn("REPLICATE_DO_DB=(`new-api`)", sql)
        self.assertNotIn("REPLICATE_WILD_DO_TABLE", sql)
        self.assertIn("SOURCE_LOG_POS=265107181", sql)
        self.assertIn("SOURCE_AUTO_POSITION=0", sql)
        self.assertIn("replica_parallel_workers=0", sql)
        self.assertIn("NO_BACKSLASH_ESCAPES", sql)
        self.assertIn("SOURCE_PASSWORD='secret''\\test'", sql)
        self.assertNotIn("RESET", sql)
        self.assertNotIn("super_read_only=OFF", sql)

    def test_public_endpoint_or_arbitrary_user_rejected(self):
        base = {"host": "172.30.0.1", "port": 13306, "user": "newapi_migrate_fixture", "password": "private"}
        for change in ({"host": "8.8.8.8"}, {"host": "0.0.0.0"}, {"host": "source.example"},
                       {"user": "root"}, {"port": True}, {"password": "bad\nvalue"}, {"unexpected": 1}):
            with self.subTest(change=change), self.assertRaises((seed.MigrationError, ValueError)):
                seed.replication_sql(receipt(), dict(base, **change))

    def test_import_failure_and_interrupt_restore_readonly(self):
        ref = {"status": "success", "backup_b_manifest_sha256": "a" * 64, "source_uuid": SOURCE,
               "database": "new-api", "differences": [], "reference_audit": {"database": "new-api", "tables": {"users": {}}}}
        for failure in (seed.MigrationError("failure"), KeyboardInterrupt()):
            with tempfile.TemporaryDirectory() as directory:
                target = FakeTarget(failure)
                with self.assertRaises(type(failure)):
                    seed.seed(target, {"manifest_sha256": "a" * 64, "path": Path("unused")}, ref, Path(directory) / "receipt.json")
                self.assertEqual(target.calls[-1][0], "lock_writes")
                self.assertFalse((Path(directory) / "receipt.json").exists())

    def test_mismatch_does_not_publish_success_receipt(self):
        audit = {"database": "new-api", "tables": {"users": {"count": 1}}, "objects": {}}
        ref = {"status": "success", "backup_b_manifest_sha256": "a" * 64, "source_uuid": SOURCE,
               "database": "new-api", "differences": [], "reference_audit": audit}
        with tempfile.TemporaryDirectory() as directory:
            target = FakeTarget()
            with patch.object(seed.pitr_audit, "audit_database", return_value=dict(audit, tables={})), self.assertRaises(seed.MigrationError):
                seed.seed(target, {"manifest_sha256": "a" * 64, "path": Path("unused")}, ref, Path(directory) / "receipt.json")
            self.assertFalse((Path(directory) / "receipt.json").exists())
            self.assertIn(("lock_writes", None), target.calls)

    def test_reference_binding_rejected_before_any_write(self):
        target = FakeTarget()
        with tempfile.TemporaryDirectory() as directory, self.assertRaises(seed.MigrationError):
            seed.seed(target, {"manifest_sha256": "a" * 64}, {}, Path(directory) / "receipt.json")
        self.assertEqual(target.calls, [])

    def test_existing_replication_is_never_rewound(self):
        target = FakeTarget()
        target.query = lambda sql: [[1]]
        with self.assertRaises(seed.MigrationError):
            seed.start_replica(target, receipt(), {}, True)
        self.assertFalse(any(kind == "sql" for kind, _ in target.calls))

    def test_source_ddl_freeze_required(self):
        target = FakeTarget()
        with self.assertRaises(seed.MigrationError):
            seed.start_replica(target, receipt(), {}, False)
        self.assertEqual(target.calls, [])

    def test_wrong_source_uuid_stops_receiver_before_applier(self):
        target = FakeTarget()
        target.query = lambda sql: [[0]] if "COUNT(*)" in sql else [[TARGET, "ON", 0]]
        connection = {"host": "172.30.0.1", "port": 13306, "user": "newapi_migrate_fixture", "password": "private"}
        with self.assertRaises(seed.MigrationError):
            seed.start_replica(target, receipt(), connection, True)
        statements = [sql for kind, sql in target.calls if kind == "sql"]
        self.assertFalse(any(sql == "START REPLICA SQL_THREAD;" for sql in statements))
        self.assertEqual(statements[-1], "STOP REPLICA;")

    def test_import_interrupt_kills_token_bound_server_session(self):
        class Child:
            def __init__(self):
                self.stdin, self.stdout = io.BytesIO(), io.BytesIO(b"37\n")
                self.terminated = False
            def poll(self):
                return None
            def terminate(self):
                self.terminated = True
            def wait(self, **kwargs):
                return 0
        child = Child()
        target = seed.Target(TARGET, SOURCE)
        target.command = lambda: ["fixture-only"]
        target.query = lambda sql: [[1]]
        statements = []
        target.sql = statements.append
        with patch.object(seed.subprocess, "Popen", return_value=child), patch.object(seed.select, "select", return_value=([child.stdout], [], [])), patch.object(seed.gzip, "open", side_effect=KeyboardInterrupt()):
            with self.assertRaises(KeyboardInterrupt):
                target.load(Path("fixture"))
        self.assertEqual(statements, ["KILL CONNECTION 37;"])
        self.assertTrue(child.terminated)

    def test_private_input_and_output_permissions(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "receipt.json"
            seed.write_json(path, receipt())
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            self.assertEqual(seed.read_json(path), receipt())
            path.chmod(0o644)
            with self.assertRaises(seed.MigrationError):
                seed.read_json(path)


@unittest.skipUnless(os.getenv("NEWAPI_MIGRATION_DOCKER_TEST") == "1", "opt-in isolated MySQL 8.0.46 test")
class RealMySQLFilterTests(unittest.TestCase):
    """Two owned containers, random volume/network names, no host published ports."""
    def test_database_filter_replays_rows_and_ddl_but_not_preflight_or_accounts(self):
        name = "newapi-migration-test-" + uuid.uuid4().hex
        network, source, target = name + "-net", name + "-source", name + "-target"
        password, replication_password = secrets.token_hex(24), secrets.token_hex(16)
        containers, volumes = [], []
        def run(args, *, input=None):
            result = subprocess.run(args, input=input, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            if result.returncode:
                diagnostic = result.stderr.decode(errors="replace").replace(password, "[fixture]").replace(replication_password, "[fixture]")
                raise AssertionError("isolated Docker/MySQL command failed: " + diagnostic[:1200])
            return result.stdout.decode().strip()
        def query(container, sql, *, headers=False):
            return run(["docker", "exec", "-i", container, "sh", "-c",
                        'export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"; exec mysql -uroot --batch --raw' + ('' if headers else ' --skip-column-names')], input=sql.encode())
        try:
            run(["docker", "network", "create", "--internal", network])
            for index, container in enumerate((source, target), 1):
                volume = container + "-data"
                run(["docker", "volume", "create", volume]); volumes.append(volume)
                run(["docker", "create", "--name", container, "--network", network,
                     "--mount", "type=volume,src=" + volume + ",dst=/var/lib/mysql",
                     "--env", "MYSQL_ROOT_PASSWORD=" + password, "mysql:8.0.46",
                     "--server-id=" + str(index), "--log-bin=binlog", "--relay-log=relay-bin",
                     "--binlog-format=ROW", "--binlog-row-image=FULL", "--gtid-mode=OFF",
                     "--skip-replica-start=ON", "--event-scheduler=OFF", "--innodb-buffer-pool-size=128M"])
                containers.append(container)
                run(["docker", "start", container])
            for container in containers:
                for _ in range(120):
                    try:
                        version = query(container, "SELECT VERSION()")
                        final = run(["docker", "exec", container, "cat", "/proc/1/comm"])
                        if version == "8.0.46" and final == "mysqld":
                            break
                    except AssertionError:
                        pass
                    time.sleep(0.5)
                else:
                    self.fail("isolated fixture did not become ready")
            query(source, "CREATE DATABASE `new-api`; USE `new-api`; CREATE TABLE funds(id INT PRIMARY KEY, balance INT);")
            # Actual single-session seed transport works with read_only still
            # enabled for ordinary clients and does not fill the target binlog.
            fixture_target = seed.Target(query(target, "SELECT @@server_uuid"), query(source, "SELECT @@server_uuid"))
            def fixture_container():
                info = json.loads(run(["docker", "inspect", target]))[0]
                self.assertEqual(info["Name"], "/" + target)
                self.assertFalse(info["HostConfig"]["PortBindings"])
                fixture_target.container_id = info["Id"]
            fixture_target.assert_container = fixture_container
            query(target, "SET GLOBAL read_only=ON;")
            before = query(target, "SHOW MASTER STATUS")
            with tempfile.TemporaryDirectory() as directory:
                dump = Path(directory) / "fixture.sql.gz"
                with gzip.open(dump, "wb") as stream:
                    stream.write(b"CREATE DATABASE `new-api`; USE `new-api`; CREATE TABLE funds(id INT PRIMARY KEY, balance INT);\n")
                fixture_target.load(dump)
            self.assertEqual(before, query(target, "SHOW MASTER STATUS"))
            self.assertEqual(query(target, "SELECT @@read_only"), "1")
            query(source, "CREATE USER 'newapi_migrate_fixture'@'%' IDENTIFIED BY '" + replication_password + "'; GRANT REPLICATION SLAVE ON *.* TO 'newapi_migrate_fixture'@'%';")
            file, pos, *_ = query(source, "SHOW MASTER STATUS").split("\t")
            state = receipt(); state["anchor"] = {"file": file, "position": int(pos)}
            address = json.loads(run(["docker", "inspect", source]))[0]["NetworkSettings"]["Networks"][network]["IPAddress"]
            query(target, "SET GLOBAL super_read_only=ON; " + seed.replication_sql(state, {
                "host": address, "port": 3306, "user": "newapi_migrate_fixture", "password": replication_password}))
            query(target, "START REPLICA SQL_THREAD;")
            # Deployment preflight root SQL has no selected database. Its own
            # schema DDL/DML and account grants must never reach the replica.
            query(source, "CREATE USER 'excluded_fixture'@'%' IDENTIFIED BY 'fixture'; GRANT SELECT ON `new-api`.* TO 'excluded_fixture'@'%'; CREATE DATABASE newapi_preflight_20261026000000_7;")
            query(source, "USE newapi_preflight_20261026000000_7; CREATE TABLE scratch(id INT); INSERT INTO scratch VALUES(7);")
            query(source, "USE `new-api`; ALTER TABLE funds ADD COLUMN memo VARCHAR(30); INSERT INTO funds VALUES(1,500,'credited'); UPDATE funds SET balance=450 WHERE id=1;")
            # ROW replication uses the affected table, even under another USE.
            query(source, "USE newapi_preflight_20261026000000_7; INSERT INTO `new-api`.funds VALUES(2,200,'row-scope');")
            file, pos, *_ = query(source, "SHOW MASTER STATUS").split("\t")
            scope_spec = importlib.util.spec_from_file_location("real_source_scope", HERE / "audit-source-binlog.py")
            scope = importlib.util.module_from_spec(scope_spec)
            scope_spec.loader.exec_module(scope)
            with tempfile.TemporaryDirectory() as directory:
                raw = subprocess.run(["docker", "exec", source, "cat", "/var/lib/mysql/" + file],
                                     stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, check=True).stdout
                (Path(directory) / file).write_bytes(raw)
                evidence = scope.audit(directory, state["anchor"], {"file": file, "position": int(pos)})
                self.assertEqual(evidence["status"], "scope-verified")
                self.assertGreater(evidence["selected_transactions"], 0)
            waited = query(target, "SELECT SOURCE_POS_WAIT('" + file + "'," + pos + ",30)")
            self.assertNotIn(waited, ("NULL", "-1"))
            self.assertEqual(query(target, "SELECT SUM(balance),COUNT(*) FROM `new-api`.funds"), "650\t2")
            self.assertEqual(query(target, "SELECT COUNT(*) FROM mysql.user WHERE User='excluded_fixture'"), "0")
            self.assertEqual(query(target, "SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME LIKE 'newapi_preflight%'"), "0")
            self.assertEqual(query(target, "SELECT @@super_read_only,@@gtid_mode,@@replica_parallel_workers"), "1\tOFF\t0")
            self.assertEqual(query(source, "SELECT COUNT(*) FROM mysql.user WHERE User='excluded_fixture'"), "1")
            self.assertEqual(query(source, "SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME LIKE 'newapi_preflight%'"), "1")
            # Final gate uses executed position, then stops replication. It does
            # not mistake receiver bytes or Seconds_Behind_Source for proof.
            query(target, "STOP REPLICA;")
            status = query(target, "SHOW REPLICA STATUS", headers=True).splitlines()
            values = dict(zip(status[0].split("\t"), status[1].split("\t")))
            self.assertEqual(values["Relay_Source_Log_File"], file)
            self.assertEqual(values["Exec_Source_Log_Pos"], pos)
            self.assertEqual(values["Last_SQL_Errno"], "0")
            self.assertEqual(values["Last_IO_Errno"], "0")
        finally:
            for container in reversed(containers):
                subprocess.run(["docker", "rm", "-f", container], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            for volume in reversed(volumes):
                subprocess.run(["docker", "volume", "rm", volume], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            subprocess.run(["docker", "network", "rm", network], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


if __name__ == "__main__":
    unittest.main()
