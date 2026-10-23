#!/usr/bin/env python3
"""Unit tests plus opt-in real Linux MySQL tests (Docker, no network/real data).

NEWAPI_PITR_TEST_MYSQL_IMAGE=mysql:8.0 python3 -m unittest discover \
  -s scripts/server-storage -p test_pitr_anchor.py -v
"""
import gzip
import faulthandler
import fcntl
import importlib.util
import io
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import tempfile
import threading
import time
import traceback
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
        cls.addClassCleanup(lambda: subprocess.run(["docker", "rm", "-fv", cls.container],
                                                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=20))
        subprocess.run(["docker", "run", "-d", "--name", cls.container, "--network", "none",
                        "-e", "MYSQL_ROOT_PASSWORD=fixture-anchor-password", image,
                        "--server-id=40123", "--log-bin=mysql-bin", "--binlog-format=ROW",
                        "--gtid-mode=OFF", "--log-error-verbosity=1"], check=True, stdout=subprocess.DEVNULL, timeout=120)
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
        faulthandler.dump_traceback_later(45, repeat=True)
        self.addCleanup(faulthandler.cancel_dump_traceback_later)
        self.run_id = uuid.uuid4().hex
        self.directory = "/tmp/newapi-r2-backup-" + self.run_id
        self.args = backup.parse_args(["--pitr-anchor", "--container", self.container, "--database", "pitr_anchor_test",
                                       "--disk-path", "/tmp", "--min-free-bytes", "2", "--stop-free-bytes", "1",
                                       "--timeout-seconds", "60", "--dump-timeout-seconds", "30"])

    def tearDown(self):
        # Cancel clients while their coordination files still exist. unittest
        # otherwise runs addCleanup after tearDown has removed those files.
        self.doCleanups()
        subprocess.run(["docker", "exec", self.container, "rm", "-rf", "--", self.directory],
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=10)

    def start_blocker(self):
        sql = "SELECT SLEEP(30) FROM pitr_anchor_test.pad"
        command = anchor.mysql_shell() + " --execute=" + __import__("shlex").quote(sql)
        blocker = subprocess.Popen(["docker", "exec", self.container, "sh", "-c", command], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        def reap():
            try:
                blocker.wait(timeout=5)
            except subprocess.TimeoutExpired:
                blocker.kill()
                blocker.wait(timeout=3)
        self.addCleanup(reap)
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
        subprocess.run(["docker", "exec", self.container, "mkdir", "-m", "700", self.directory], check=True, timeout=10)
        options = ["--single-transaction", "--source-data=2", "--set-gtid-purged=OFF", "--no-tablespaces", "--databases", "pitr_anchor_test"]
        command = backup.dump_command(self.args, self.run_id, options)
        if transform:
            command[-1] = transform(command[-1])
        child = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        def reap():
            try:
                if child.poll() is None:
                    try:
                        backup.cancel_dump(self.args, self.run_id)
                    finally:
                        try:
                            child.communicate(timeout=5)
                        except subprocess.TimeoutExpired:
                            child.kill()
                            child.communicate(timeout=3)
            finally:
                for stream in (child.stdout, child.stderr):
                    if stream is not None:
                        stream.close()
        self.addCleanup(reap)
        return child

    def coordination_file(self, name):
        result = subprocess.run(["docker", "exec", self.container, "cat", self.directory + "/" + name],
                                stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=5)
        return result.stdout.strip() if result.returncode == 0 else None

    def assert_no_dump_connections(self):
        self.assertEqual(anchor.query(self.container, "SELECT COUNT(*) FROM performance_schema.session_connect_attrs WHERE ATTR_NAME='program_name' AND ATTR_VALUE='mysqldump'"), "0")

    def dump_diagnostics(self, phase, child):
        print("\nPITR failure phase=" + phase + " docker_returncode=" + str(child.poll()), file=sys.stderr, flush=True)
        script = r'''
for name in pids guard-pid held-consumer guard-first-poll transaction-starting post-unlock-marker ready ready-method done timed-out cancellation-started cancel-confirmed cancel-failed cancelled; do
  if [ -f "$1/$name" ]; then printf '%s=' "$name"; head -c 256 "$1/$name"; printf '\n'; fi
done
for pid in $(cat "$1/pids" "$1/guard-pid" "$1/held-consumer" 2>/dev/null); do
  case "$pid" in *[!0-9]*|'') continue;; esac
  if [ -r "/proc/$pid/stat" ]; then
    printf 'proc_stat='; cat "/proc/$pid/stat"
    printf 'proc_wchan='; cat "/proc/$pid/wchan"; printf '\n'
  fi
done
'''
        try:
            result = subprocess.run(['docker', 'exec', self.container, 'bash', '-c', script, 'diagnostic', self.directory],
                                    stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=5)
            print(result.stdout.decode('utf-8', 'replace')[:8192], file=sys.stderr, flush=True)
        except subprocess.SubprocessError as error:
            print('PITR process diagnostics unavailable: ' + type(error).__name__, file=sys.stderr, flush=True)
        try:
            rows = anchor.query(self.container, "SELECT t.PROCESSLIST_ID,t.PROCESSLIST_COMMAND,t.PROCESSLIST_TIME,"
                "COALESCE(t.PROCESSLIST_STATE,''),a.ATTR_VALUE FROM performance_schema.threads t JOIN "
                "performance_schema.session_connect_attrs a ON a.PROCESSLIST_ID=t.PROCESSLIST_ID "
                "WHERE a.ATTR_NAME='program_name' AND a.ATTR_VALUE IN ('mysql','mysqldump') ORDER BY t.PROCESSLIST_ID")
            print('PITR connection metadata:\n' + rows[:4096], file=sys.stderr, flush=True)
        except anchor.AnchorError:
            print('PITR connection metadata unavailable within five seconds', file=sys.stderr, flush=True)

    def test_real_stream_anchor_and_gzip(self):
        with backup.deadline(30), tempfile.TemporaryDirectory() as directory, backup.Processes() as processes:
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

    def exercise_upload_backpressure(self, interrupt):
        for offset in (100, 200, 400, 800, 1600):
            anchor.query(self.container, "INSERT INTO pitr_anchor_test.pad SELECT id+" + str(offset) + ",v FROM pitr_anchor_test.pad")
        self.addCleanup(lambda: anchor.query(self.container, "DELETE FROM pitr_anchor_test.pad WHERE id<>1"))
        with backup.Processes() as processes:
            class Remote:
                child = None
                def writer(self, _destination):
                    self.child = processes.start([sys.executable, '-c', 'import time; time.sleep(60)'],
                                                 stdin=subprocess.PIPE, stdout=subprocess.DEVNULL)
                    # Four KiB fills well before this 32 MiB dump completes.
                    # The uploader never consumes stdin, leaving the backup
                    # host's Docker output bridge genuinely backpressured.
                    fcntl.fcntl(self.child.stdin, fcntl.F_SETPIPE_SZ, 4096)
                    return self.child
            remote = Remote()
            lock = anchor.InstanceLock(self.container).acquire()
            timer = None
            try:
                source = backup.preflight(self.args, processes)
                expected = 'backup was interrupted' if interrupt else 'backup exceeded its time limit'
                with self.assertRaisesRegex(backup.BackupError, expected):
                    with backup.deadline(20 if interrupt else 4):
                        if interrupt:
                            timer = threading.Timer(4, lambda: os.kill(os.getpid(), signal.SIGTERM))
                            timer.start()
                        backup.stream_dump(self.args, remote, 'fixture', processes, self.run_id, source, lock)
                self.assertIsNotNone(remote.child)
                self.assertIsNone(remote.child.poll(), 'uploader must still be withholding reads at cancellation')
                self.assert_no_dump_connections()
            finally:
                if timer:
                    timer.cancel()
                    timer.join(timeout=1)
                lock.close()
            self.assertEqual(anchor.query(self.container, "SELECT COUNT(*) FROM performance_schema.metadata_locks WHERE OBJECT_TYPE='BACKUP LOCK'"), '0')

    @unittest.skipUnless(hasattr(fcntl, 'F_SETPIPE_SZ'), 'Linux pipe backpressure test')
    def test_deadline_cancels_sql_under_host_upload_backpressure(self):
        self.exercise_upload_backpressure(False)

    @unittest.skipUnless(hasattr(fcntl, 'F_SETPIPE_SZ'), 'Linux pipe backpressure test')
    def test_sigterm_cancels_sql_under_host_upload_backpressure(self):
        self.exercise_upload_backpressure(True)

    def test_initial_cancel_inventory_failure_still_terminates_and_confirms(self):
        self.start_blocker()
        child = self.dump()
        time.sleep(.3)
        script = anchor.cancel_script(self.run_id).replace('connections() {', 'original_connections() {', 1)
        injected = r'''
connections() {
  if [ ! -f "$run_dir/injected-inventory-failure" ]; then
    : > "$run_dir/injected-inventory-failure"
    return 1
  fi
  original_connections
}
cancel_sql || exit 74
'''
        script = script.replace('cancel_sql || exit 74', injected)
        result = subprocess.run(['docker', 'exec', self.container, 'bash', '-c', script],
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=12)
        self.assertEqual(result.returncode, 0, 'initial inventory failure must not bypass client cancellation')
        self.assertEqual(self.coordination_file('injected-inventory-failure'), b'')
        child.communicate(timeout=8)
        self.assertNotEqual(child.returncode, 0)
        self.assert_no_dump_connections()

    def test_unconfirmed_cancellation_has_one_container_deadline(self):
        self.start_blocker()
        child = self.dump()
        time.sleep(.3)
        script = anchor.cancel_script(self.run_id)
        # An unavailable observer keeps reporting a connection; each attempted
        # cancellation also takes server time. Client death must not become a
        # false success, and a host CLI timeout must not be the primary bound.
        injected = r'''
connections() { printf '999999999\n'; }
kill_connection() { sql 'SELECT SLEEP(2)' >/dev/null; return 1; }
cancel_sql || exit 74
'''
        script = script.replace('cancel_sql || exit 74', injected)
        started = time.monotonic()
        result = subprocess.run(['docker', 'exec', self.container, 'bash', '-c', script],
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=12)
        self.assertEqual(result.returncode, 74, 'unconfirmed server cancellation must remain a failure')
        self.assertGreater(time.monotonic() - started, 7)
        self.assertLess(time.monotonic() - started, 12)
        child.communicate(timeout=8)
        self.assertNotEqual(child.returncode, 0)
        self.assert_no_dump_connections()

    @unittest.skipUnless(os.environ.get('NEWAPI_PITR_TEST_DELAY_LIBRARY'), 'compile test_pitr_delay.c for the MySQL image architecture')
    def test_actual_ftwrl_wait_is_cancelled(self):
        library = '/tmp/newapi-pitr-test-delay.so'
        subprocess.run(['docker', 'cp', os.environ['NEWAPI_PITR_TEST_DELAY_LIBRARY'], self.container + ':' + library], check=True, timeout=10)
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
        # Apply backpressure inside the container. Holding Docker's transport
        # stdout unread can block unrelated docker exec observations on Linux,
        # hiding a successful SQL cancellation behind the test's own bridge.
        for offset in (100, 200, 400, 800, 1600):
            anchor.query(self.container, "INSERT INTO pitr_anchor_test.pad SELECT id+" + str(offset) + ",v FROM pitr_anchor_test.pad")
        self.start_blocker()
        def hold_output(script):
            script = script.replace('cleanup() {\n', 'cleanup() {\n  [ ! -f "$run_dir/held-consumer" ] || kill "$(cat "$run_dir/held-consumer")" 2>/dev/null || :\n', 1)
            script = script.replace('monitor_pid=$!\n', 'monitor_pid=$!\nmkfifo -m 600 "$run_dir/held-output"\nsleep 30 < "$run_dir/held-output" >/dev/null 2>&1 &\nprintf "%s\\n" "$!" > "$run_dir/held-consumer"\n', 1)
            return script.replace(' 2> "$run_dir/events" &', ' > "$run_dir/held-output" 2> "$run_dir/events" &', 1)
        child = self.dump(hold_output)
        guard = None
        phase = 'waiting-for-guard'
        try:
            for _ in range(50):
                guard = self.coordination_file('guard-pid')
                if guard:
                    break
                time.sleep(.02)
            self.assertIsNotNone(guard)
            phase = 'suspending-guard'
            subprocess.run(["docker", "exec", self.container, "bash", "-c", 'kill -STOP "$1"', "guard-test", guard.decode()], check=True, timeout=5)
            # Let a real FLUSH wait cross the deadline while its guard cannot
            # run, then release the blocker and allow a late savepoint marker.
            time.sleep(5.3)
            phase = 'releasing-table-blocker'
            ids = anchor.query(self.container, "SELECT ID FROM information_schema.PROCESSLIST WHERE INFO='SELECT SLEEP(30) FROM pitr_anchor_test.pad'")
            for identity in ids.splitlines():
                anchor.query(self.container, "KILL CONNECTION " + identity)
            for _ in range(50):
                late = self.coordination_file('post-unlock-marker')
                if late:
                    break
                time.sleep(.02)
            self.assertGreater(int(late), 5000000000)
            phase = 'publishing-late-ready'
            self.assertEqual(anchor.query(self.container, "SELECT COUNT(*) FROM performance_schema.session_connect_attrs WHERE ATTR_NAME='program_name' AND ATTR_VALUE='mysqldump'"), '1')
            # Model the exact previous bug: a late producer has written ready
            # before the suspended guard observes the deadline.
            subprocess.run(["docker", "exec", self.container, "cp", self.directory + '/post-unlock-marker', self.directory + '/ready'], check=True, timeout=5)
            subprocess.run(["docker", "exec", self.container, "bash", "-c", 'kill -CONT "$1"', "guard-test", guard.decode()], check=True, timeout=5)
            guard = None
            phase = 'waiting-for-cancellation-proof'
            limit = time.monotonic() + 4
            while self.coordination_file('cancel-confirmed') is None and time.monotonic() < limit:
                time.sleep(.05)
            self.assertEqual(self.coordination_file('timed-out'), b'')
            self.assertEqual(self.coordination_file('cancel-confirmed'), b'')
            self.assert_no_dump_connections()
            phase = 'draining-buffered-output'
            # The real client was blocked on the private FIFO until cancelled;
            # Docker's observation channel remains free throughout this check.
            child.communicate(timeout=4)
            self.assertNotEqual(child.returncode, 0)
        except BaseException:
            # Print the primary error before cleanup. A secondary cancellation
            # timeout must never replace the failure that triggered cleanup.
            traceback.print_exc(file=sys.stderr)
            self.dump_diagnostics(phase, child)
            raise
        finally:
            original_error = sys.exc_info()[1]
            cleanup_errors = []
            if guard:
                try:
                    subprocess.run(["docker", "exec", self.container, "bash", "-c", 'kill -CONT "$1"', "guard-test", guard.decode()], check=False, timeout=5)
                except subprocess.SubprocessError as error:
                    cleanup_errors.append(error)
            if child.poll() is None:
                try:
                    backup.cancel_dump(self.args, self.run_id)
                except backup.BackupError as error:
                    cleanup_errors.append(error)
            try:
                child.communicate(timeout=8)
            except subprocess.SubprocessError as error:
                cleanup_errors.append(error)
            if cleanup_errors:
                self.dump_diagnostics('cleanup-after-' + phase, child)
                for error in cleanup_errors:
                    detail = str(error) if isinstance(error, backup.BackupError) else ' timeout=' + str(getattr(error, 'timeout', None))
                    print('PITR additional cleanup error: ' + type(error).__name__ + ': ' + detail, file=sys.stderr, flush=True)
                if original_error is None:
                    raise cleanup_errors[0]

    @unittest.skipUnless(os.environ.get('NEWAPI_PITR_TEST_DELAY_LIBRARY'), 'compile test_pitr_delay.c for the MySQL image architecture')
    def test_post_unlock_show_create_delay_uses_server_evidence(self):
        library = '/tmp/newapi-pitr-test-delay.so'
        subprocess.run(['docker', 'cp', os.environ['NEWAPI_PITR_TEST_DELAY_LIBRARY'], self.container + ':' + library], check=True, timeout=10)
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
