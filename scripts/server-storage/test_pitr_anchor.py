#!/usr/bin/env python3
"""Unit tests plus opt-in real Linux MySQL tests (Docker, no network/real data).

NEWAPI_PITR_TEST_MYSQL_IMAGE=mysql:8.0 python3 -m unittest discover \
  -s scripts/server-storage -p test_pitr_anchor.py -v
"""
import gzip
import importlib.util
import io
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import time
import unittest
import uuid
from unittest import mock


SPEC = importlib.util.spec_from_file_location("anchor_backup", Path(__file__).with_name("mysql-r2-backup.py"))
backup = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(backup)
anchor = backup.anchor


class HeaderTests(unittest.TestCase):
    def test_chunked_same_stream_coordinate(self):
        parser = anchor.Header()
        data = b"-- header\n-- CHANGE MASTER TO MASTER_LOG_FILE='mysql-bin.000123', MASTER_LOG_POS=456;\n\n-- Current Database: `new-api`\n"
        for byte in data:
            parser.feed(bytes([byte]))
        self.assertEqual(parser.finish(), {"file": "mysql-bin.000123", "position": 456})

    def test_missing_duplicate_traversal_and_invalid_position_rejected(self):
        valid = b"-- CHANGE MASTER TO MASTER_LOG_FILE='mysql-bin.000123', MASTER_LOG_POS=456;\n"
        for header in (b"", valid + valid, valid.replace(b"mysql-bin", b"../mysql-bin"), valid.replace(b"456", b"3")):
            with self.subTest(header=header):
                with self.assertRaises(anchor.AnchorError):
                    parser = anchor.Header()
                    parser.feed(header + b"\n-- Current Database: `new-api`\n")
                    parser.finish()

    def test_fake_anchor_in_business_data_is_not_examined(self):
        parser = anchor.Header()
        parser.feed(b"-- CHANGE MASTER TO MASTER_LOG_FILE='binlog.000001', MASTER_LOG_POS=157;\n\n-- Current Database: x\n")
        parser.feed(b"-- CHANGE MASTER TO MASTER_LOG_FILE='binlog.000002', MASTER_LOG_POS=456;\n")
        self.assertEqual(parser.finish()["file"], "binlog.000001")

    def test_bounded_header_and_missing_anchor_fail(self):
        with self.assertRaises(anchor.AnchorError):
            anchor.Header().feed(b"x" * 65537)
        with self.assertRaises(anchor.AnchorError):
            anchor.Header().finish()

    def test_original_mode_remains_default(self):
        self.assertFalse(backup.parse_args([]).pitr_anchor)
        self.assertTrue(backup.parse_args(["--pitr-anchor"]).pitr_anchor)

    def test_guard_has_server_cancel_and_separate_startup_deadline(self):
        script = anchor.dump_script("test-run", ["--single-transaction", "--source-data=2"], 2700)
        self.assertIn("KILL CONNECTION", script)
        self.assertIn("5000000000", script)
        self.assertIn("2700000000000", script)
        self.assertIn("-- Setting savepoint...", script)

    def test_source_scope_and_durability_fail_closed(self):
        args = backup.parse_args(["--pitr-anchor", "--database", "new-api", "--disk-path", "/tmp", "--min-free-bytes", "2", "--stop-free-bytes", "1"])
        valid = b"12345678-1234-1234-1234-123456789abc\t1\tROW\tFULL\tOFF\t1\t1\tCRC32\t604800\t1\t1\t512\t0\t0\n1\n1\n5\n2\n1\nnew-api\n"
        for metadata in (valid + b"other-db\n", valid.replace(b"ROW", b"STATEMENT"), valid.replace(b"\t512", b"\t0"), valid.replace(b"\tOFF\t1\t1", b"\tOFF\t0\t1"), valid.replace(b"\tOFF", b"\tON"), valid.replace(b"\t0\t0\n", b"\t1\t0\n"), valid.replace(b"\n1\n1\n", b"\n0\n1\n"), valid.replace(b"\n5\n2\n", b"\n4\n2\n"), valid.replace(b"\n2\n1\n", b"\n1\n1\n"), valid.replace(b"\n1\nnew-api", b"\n0\nnew-api")):
            process = mock.Mock()
            process.capture.side_effect = [b"8.0.46\n2\t0\n", metadata]
            with self.assertRaises(backup.BackupError):
                backup.preflight(args, process)


@unittest.skipUnless(os.environ.get("NEWAPI_PITR_TEST_MYSQL_IMAGE"), "set NEWAPI_PITR_TEST_MYSQL_IMAGE for isolated real MySQL tests")
class RealMySQLTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.container = "newapi-pitr-anchor-test-" + uuid.uuid4().hex[:12]
        image = os.environ["NEWAPI_PITR_TEST_MYSQL_IMAGE"]
        subprocess.run(["docker", "run", "-d", "--name", cls.container, "--network", "none",
                        "-e", "MYSQL_ROOT_PASSWORD=fixture-anchor-password", image,
                        "--server-id=40123", "--log-bin=mysql-bin", "--binlog-format=ROW",
                        "--gtid-mode=OFF", "--log-error-verbosity=1"], check=True, stdout=subprocess.DEVNULL)
        cls.addClassCleanup(lambda: subprocess.run(["docker", "rm", "-fv", cls.container],
                                                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL))
        deadline = time.monotonic() + 90
        while True:
            try:
                if anchor.query(cls.container, "SELECT @@skip_networking") == "0":
                    break
                if time.monotonic() > deadline:
                    raise anchor.AnchorError("final MySQL server did not start")
                time.sleep(1)
            except anchor.AnchorError:
                if time.monotonic() > deadline:
                    raise
                time.sleep(1)
        anchor.query(cls.container, "CREATE DATABASE pitr_anchor_test; CREATE TABLE pitr_anchor_test.balance(id INT PRIMARY KEY,amount BIGINT) ENGINE=InnoDB; INSERT INTO pitr_anchor_test.balance VALUES(1,100); CREATE TABLE pitr_anchor_test.pad(id INT PRIMARY KEY,v LONGTEXT) ENGINE=InnoDB; INSERT INTO pitr_anchor_test.pad VALUES(1,REPEAT('x',1000000));")

    def setUp(self):
        self.run_id = uuid.uuid4().hex
        self.directory = "/tmp/newapi-r2-backup-" + self.run_id
        self.args = backup.parse_args(["--pitr-anchor", "--container", self.container, "--database", "pitr_anchor_test",
                                       "--disk-path", "/tmp", "--min-free-bytes", "2", "--stop-free-bytes", "1"])

    def tearDown(self):
        subprocess.run(["docker", "exec", self.container, "rm", "-rf", "--", self.directory],
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    def start_blocker(self):
        sql = "SELECT SLEEP(30) FROM pitr_anchor_test.pad"
        command = anchor.mysql_shell() + " --execute=" + __import__("shlex").quote(sql)
        blocker = subprocess.Popen(["docker", "exec", self.container, "sh", "-c", command], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        self.addCleanup(lambda: blocker.wait(timeout=5))
        def stop():
            ids = anchor.query(self.container, "SELECT ID FROM information_schema.PROCESSLIST WHERE INFO='" + sql + "'")
            for identity in ids.splitlines():
                anchor.query(self.container, "KILL CONNECTION " + identity)
        self.addCleanup(stop)
        for _ in range(50):
            if anchor.query(self.container, "SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE INFO='" + sql + "'") == "1":
                return blocker
            time.sleep(.05)
        self.fail("blocker did not start")

    def dump(self, transform=None):
        subprocess.run(["docker", "exec", self.container, "mkdir", "-m", "700", self.directory], check=True)
        options = ["--single-transaction", "--source-data=2", "--set-gtid-purged=OFF", "--no-tablespaces", "--databases", "pitr_anchor_test"]
        command = backup.dump_command(self.args, self.run_id, options)
        if transform:
            command[-1] = transform(command[-1])
        return subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE)

    def coordination_file(self, name):
        result = subprocess.run(["docker", "exec", self.container, "cat", self.directory + "/" + name],
                                stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
        return result.stdout.strip() if result.returncode == 0 else None

    def assert_no_dump_connections(self):
        self.assertEqual(anchor.query(self.container, "SELECT COUNT(*) FROM performance_schema.session_connect_attrs WHERE ATTR_NAME='program_name' AND ATTR_VALUE='mysqldump'"), "0")

    def test_real_stream_anchor_and_gzip(self):
        with tempfile.TemporaryDirectory() as directory, backup.Processes() as processes:
            path = Path(directory) / "snapshot.gz"
            class Remote:
                def writer(self, _destination):
                    return processes.start(["sh", "-c", 'cat > "$1"', "sh", str(path)], stdin=subprocess.PIPE, stdout=subprocess.DEVNULL)
            lock = anchor.InstanceLock(self.container).acquire()
            try:
                source = backup.preflight(self.args, processes)
                result = backup.stream_dump(self.args, Remote(), "fixture", processes, self.run_id, source, lock)
                self.assertEqual(result["size"], path.stat().st_size)
                sql = gzip.decompress(path.read_bytes())
                parser = anchor.Header()
                parser.feed(sql)
                self.assertEqual(source["binlog_anchor"]["position"], parser.finish()["position"])
                self.assertLess(source["binlog_anchor"]["startup_lock_window_upper_bound_seconds"], 5)
                self.assertIn(source["binlog_anchor"]["startup_unlock_evidence"], ("completed-unlock", "active-snapshot", "post-unlock-marker"))
                self.assertEqual(source["database_scope"], ["pitr_anchor_test"])
                self.assertTrue(lock.closed)
            finally:
                lock.close()
        self.assert_no_dump_connections()

    def test_full_receipt_and_remote_io_do_not_hold_release_lock(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            binaries = root / "bin"
            binaries.mkdir(mode=0o700)
            fake = binaries / "rclone"
            fake.write_text("#!" + sys.executable + "\n" + r'''
import fcntl,os,sys
from pathlib import Path
root=Path(os.environ['ANCHOR_TEST_ROOT'])
with (root/'release.lock').open('a') as lock:
    fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
    with (root/'unlocked-io').open('a') as proof: proof.write('checked\n')
args=sys.argv[1:]
operation=next(x for x in args if x in ('rcat','cat'))
destination=args[args.index(operation)+1]
path=root/'cloud'/destination.replace(':','/')
if operation=='rcat':
    path.parent.mkdir(parents=True,exist_ok=True)
    path.write_bytes(sys.stdin.buffer.read())
else: sys.stdout.buffer.write(path.read_bytes())
''')
            fake.chmod(0o700)
            config = root / "rclone.conf"
            config.write_text("[r2]\ntype=s3\n[r2-backup]\ntype=crypt\nremote=r2:fixture\npassword=fixture\n")
            config.chmod(0o600)
            self.args.rclone_config = str(config)
            self.args.state_dir = str(root / "state")
            self.args.release_lock = str(root / "release.lock")
            self.args.job_lock = str(root / "job.lock")
            self.args.action = "run"
            with mock.patch.dict(os.environ, {"PATH": str(binaries) + os.pathsep + os.environ["PATH"], "ANCHOR_TEST_ROOT": str(root)}), mock.patch.object(backup, "require_root"):
                receipt = backup.execute(self.args)
            self.assertEqual(receipt["status"], "success")
            self.assertIn("binlog_anchor", receipt["source"])
            self.assertFalse(receipt["pitr_verified"])
            self.assertEqual(len((root / "unlocked-io").read_text().splitlines()), 6)
            self.assertEqual(len(list((root / "cloud").rglob("SUCCESS.json"))), 1)

    def test_startup_guard_kills_waiting_server_connection(self):
        self.start_blocker()
        started = time.monotonic()
        child = self.dump()
        output, _ = child.communicate(timeout=12)
        self.assertNotEqual(child.returncode, 0)
        self.assertLess(time.monotonic() - started, 10)
        self.assert_no_dump_connections()
        anchor.query(self.container, "UPDATE pitr_anchor_test.balance SET amount=amount+1 WHERE id=1")

    def test_explicit_interruption_cancels_waiting_sql(self):
        self.start_blocker()
        child = self.dump()
        time.sleep(.3)
        backup.cancel_dump(self.args, self.run_id)
        child.communicate(timeout=8)
        self.assertNotEqual(child.returncode, 0)
        self.assert_no_dump_connections()

    @unittest.skipUnless(os.environ.get('NEWAPI_PITR_TEST_DELAY_LIBRARY'), 'compile test_pitr_delay.c for the MySQL image architecture')
    def test_actual_ftwrl_wait_is_cancelled(self):
        library = '/tmp/newapi-pitr-test-delay.so'
        subprocess.run(['docker', 'cp', os.environ['NEWAPI_PITR_TEST_DELAY_LIBRARY'], self.container + ':' + library], check=True)
        def inject(script):
            return script.replace(' mysqldump --protocol=SOCKET',
                ' LD_PRELOAD=' + library + ' NEWAPI_PITR_TEST_FTWRL_DELAY=1 NEWAPI_PITR_DELAY_MARKER=' + self.directory + '/before-ftwrl mysqldump --protocol=SOCKET')
        child = self.dump(inject)
        try:
            for _ in range(50):
                if self.coordination_file('before-ftwrl') is not None:
                    break
                time.sleep(.02)
            self.assertEqual(self.coordination_file('before-ftwrl'), b'')
            self.start_blocker()
            for _ in range(60):
                count = anchor.query(self.container, "SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE INFO='FLUSH TABLES WITH READ LOCK'")
                if count == '1':
                    break
                time.sleep(.03)
            self.assertEqual(count, '1', 'observe actual FTWRL wait after the earlier FLUSH LOCAL completed')
            child.communicate(timeout=8)
            self.assertNotEqual(child.returncode, 0)
            self.assertEqual(self.coordination_file('cancel-confirmed'), b'')
            self.assert_no_dump_connections()
        finally:
            if child.poll() is None:
                backup.cancel_dump(self.args, self.run_id)
                child.communicate(timeout=8)

    def test_suspended_guard_rejects_late_ready_before_dump_completes(self):
        # Exceed Docker's output buffering so the client cannot finish while
        # the test deliberately withholds reads after its startup marker.
        for offset in (100, 200, 400, 800, 1600):
            anchor.query(self.container, "INSERT INTO pitr_anchor_test.pad SELECT id+" + str(offset) + ",v FROM pitr_anchor_test.pad")
        self.start_blocker()
        child = self.dump()
        guard = None
        try:
            for _ in range(50):
                guard = self.coordination_file('guard-pid')
                if guard:
                    break
                time.sleep(.02)
            self.assertIsNotNone(guard)
            subprocess.run(["docker", "exec", self.container, "bash", "-c", 'kill -STOP "$1"', "guard-test", guard.decode()], check=True)
            # Let a real FLUSH wait cross the deadline while its guard cannot
            # run, then release the blocker and allow a late savepoint marker.
            time.sleep(5.3)
            ids = anchor.query(self.container, "SELECT ID FROM information_schema.PROCESSLIST WHERE INFO='SELECT SLEEP(30) FROM pitr_anchor_test.pad'")
            for identity in ids.splitlines():
                anchor.query(self.container, "KILL CONNECTION " + identity)
            for _ in range(50):
                late = self.coordination_file('post-unlock-marker')
                if late:
                    break
                time.sleep(.02)
            self.assertGreater(int(late), 5000000000)
            # Model the exact previous bug: a late producer has written ready
            # before the suspended guard observes the deadline.
            subprocess.run(["docker", "exec", self.container, "cp", self.directory + '/post-unlock-marker', self.directory + '/ready'], check=True)
            subprocess.run(["docker", "exec", self.container, "bash", "-c", 'kill -CONT "$1"', "guard-test", guard.decode()], check=True)
            guard = None
            limit = time.monotonic() + 4
            while self.coordination_file('cancel-confirmed') is None and time.monotonic() < limit:
                time.sleep(.05)
            self.assertEqual(self.coordination_file('timed-out'), b'')
            self.assertEqual(self.coordination_file('cancel-confirmed'), b'')
            self.assert_no_dump_connections()
            # Docker's output bridge can wait for the consumer even after the
            # real server connection and container client are gone. Verify SQL
            # cancellation under backpressure, then drain only buffered output.
            child.communicate(timeout=4)
            self.assertNotEqual(child.returncode, 0)
        finally:
            if guard:
                subprocess.run(["docker", "exec", self.container, "bash", "-c", 'kill -CONT "$1"', "guard-test", guard.decode()], check=False)
            if child.poll() is None:
                backup.cancel_dump(self.args, self.run_id)
            child.communicate(timeout=8)

    @unittest.skipUnless(os.environ.get('NEWAPI_PITR_TEST_DELAY_LIBRARY'), 'compile test_pitr_delay.c for the MySQL image architecture')
    def test_post_unlock_show_create_delay_uses_server_evidence(self):
        library = '/tmp/newapi-pitr-test-delay.so'
        subprocess.run(['docker', 'cp', os.environ['NEWAPI_PITR_TEST_DELAY_LIBRARY'], self.container + ':' + library], check=True)
        def inject(script):
            return script.replace(' mysqldump --protocol=SOCKET',
                ' LD_PRELOAD=' + library + ' NEWAPI_PITR_DELAY_MARKER=' + self.directory + '/show-create-delayed mysqldump --protocol=SOCKET')
        child = self.dump(inject)
        try:
            for _ in range(100):
                ready = self.coordination_file('ready')
                if ready:
                    break
                time.sleep(.02)
            self.assertEqual(self.coordination_file('show-create-delayed'), b'')
            self.assertLessEqual(int(ready), 5000000000)
            self.assertIn(self.coordination_file('ready-method'), (b'completed-unlock', b'active-snapshot'))
            self.assertIsNone(self.coordination_file('post-unlock-marker'))
            output, errors = child.communicate(timeout=12)
            self.assertEqual(child.returncode, 0, errors.decode())
            self.assertGreater(int(self.coordination_file('post-unlock-marker')), 5000000000)
            self.assertIsNone(self.coordination_file('timed-out'))
            parser = anchor.Header()
            parser.feed(output)
            parser.finish()
        finally:
            if child.poll() is None:
                backup.cancel_dump(self.args, self.run_id)
                child.communicate(timeout=8)

    def test_instance_lock_allows_dml_and_blocks_ddl(self):
        lock = anchor.InstanceLock(self.container).acquire()
        try:
            anchor.query(self.container, "UPDATE pitr_anchor_test.balance SET amount=amount+1 WHERE id=1")
            with self.assertRaises(anchor.AnchorError):
                anchor.query(self.container, "SET SESSION lock_wait_timeout=1; ALTER TABLE pitr_anchor_test.balance ADD COLUMN must_not_exist INT", timeout=3)
            time.sleep(2.2)
            lock.check()
        finally:
            lock.close()

    def test_lost_ddl_lock_is_detected(self):
        lock = anchor.InstanceLock(self.container).acquire()
        try:
            anchor.query(self.container, "KILL CONNECTION " + str(lock.connection_id))
            time.sleep(2.2)
            with self.assertRaises(anchor.AnchorError):
                lock.check()
        finally:
            lock.close()

    def test_reused_connection_id_cannot_kill_another_lease(self):
        first = anchor.InstanceLock(self.container).acquire()
        second = anchor.InstanceLock(self.container).acquire()
        try:
            first.connection_id = second.connection_id
            first.close()
            self.assertEqual(anchor.query(self.container, "SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE ID=" + str(second.connection_id)), "1")
            time.sleep(2.2)
            second.check()
        finally:
            first.close()
            second.close()

    def test_orphan_instance_lock_expires_without_heartbeats(self):
        lock = anchor.InstanceLock(self.container).acquire()
        try:
            # Model abrupt coordinator loss without sending UNLOCK or KILL.
            # The mysql client remains connected, so server wait_timeout must
            # actually release the lock; a Docker socket close cannot explain it.
            lock.stop.set()
            lock.thread.join(timeout=4)
            limit = time.monotonic() + 13
            while anchor.query(self.container, "SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE ID=" + str(lock.connection_id)) != "0":
                self.assertLess(time.monotonic(), limit)
                time.sleep(.25)
            anchor.query(self.container, "CREATE TABLE pitr_anchor_test.lease_probe(id INT) ENGINE=InnoDB; DROP TABLE pitr_anchor_test.lease_probe")
        finally:
            lock.close()


if __name__ == "__main__":
    unittest.main()
