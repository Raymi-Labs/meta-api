#!/usr/bin/env python3
"""Exercise real subprocess pipelines with isolated Docker/rclone stand-ins."""
import contextlib
import gzip
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock


SPEC = importlib.util.spec_from_file_location("mysql_r2_backup", Path(__file__).with_name("mysql-r2-backup.py"))
backup = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(backup)


DOCKER = r'''
import os,sys,time
from pathlib import Path
args=sys.argv[1:]
command=args[-1]
root=Path(os.environ['FAKE_ROOT'])
with (root/'docker-calls').open('a') as log:log.write(repr(args)+'\n')
if ' exec mysql ' in command:
    if os.environ.get('FAIL_PREFLIGHT'):sys.exit(3)
    print('8.0.46\n4\t'+os.environ.get('NON_INNODB','0'))
elif command.startswith('run_dir='):
    (root/'cancelled').write_text('yes')
    if os.environ.get('FAIL_CANCEL'):sys.exit(4)
elif 'NEWAPI_BACKUP_RUN=' in command:
    if os.environ.get('SLOW_DUMP'):time.sleep(5)
    sys.stdout.buffer.write(b'CREATE TABLE example(id int);\n'+b'INSERT INTO example VALUES (1);\n'*10000)
    sys.stdout.buffer.flush()
    if os.environ.get('FAIL_DUMP'):sys.exit(42)
elif args[2]=='mkdir':
    pass
else:
    sys.exit(2)
'''


RCLONE = r'''
import gzip,json,os,sys
from pathlib import Path
root=Path(os.environ['FAKE_ROOT'])
args=sys.argv[1:]
operation=next(arg for arg in args if arg in ('rcat','cat'))
destination=args[args.index(operation)+1]
path=root/'cloud'/destination.replace(':','/')
with (root/'remote-calls').open('a') as log:log.write(operation+' '+path.name+'\n')
(root/'environment.json').write_text(json.dumps({key:value for key,value in os.environ.items() if key.startswith(('RCLONE_','AWS_'))}))
if operation=='rcat':
    data=sys.stdin.buffer.read()
    path.parent.mkdir(parents=True,exist_ok=True)
    path.write_bytes(data)
    if path.name==os.environ.get('FAIL_UPLOAD'):sys.exit(5)
else:
    data=path.read_bytes()
    if path.name=='mysql.sql.gz':
        if os.environ.get('BAD_GZIP'):data=b'not a gzip stream'
        if os.environ.get('BAD_HASH'):data=gzip.compress(b'different valid sql')
        if os.environ.get('EXTRA_MEMBER'):data+=gzip.compress(b'extra SQL')
    if path.name==os.environ.get('BAD_READ'):data+=b'x'
    sys.stdout.buffer.write(data)
    sys.stdout.buffer.flush()
    if path.name==os.environ.get('FAIL_READ'):sys.exit(6)
'''


class PipelineTests(unittest.TestCase):
    def setUp(self):
        # resolve() handles macOS /var -> /private/var before safety validation.
        self.temporary = tempfile.TemporaryDirectory()
        self.root = Path(self.temporary.name).resolve()
        self.bin = self.root / "bin"
        self.bin.mkdir(mode=0o700)
        for name, source in (("docker", DOCKER), ("rclone", RCLONE)):
            path = self.bin / name
            path.write_text("#!" + sys.executable + "\n" + source)
            path.chmod(0o700)
        self.config = self.root / "rclone.conf"
        self.config.write_text("[r2]\ntype = s3\nprovider = Cloudflare\n"
                               "[r2-backup]\ntype = crypt\nremote = r2:bucket/encrypted\npassword = obscured\n")
        self.config.chmod(0o600)
        self.args = backup.parse_args(["run", "--rclone-config", str(self.config),
                                      "--state-dir", str(self.root / "state"),
                                      "--job-lock", str(self.root / "job.lock"),
                                      "--release-lock", str(self.root / "release.lock"),
                                      "--disk-path", str(self.root), "--min-free-bytes", "1024",
                                      "--stop-free-bytes", "512"])
        self.env = mock.patch.dict(os.environ, {"PATH": str(self.bin) + os.pathsep + os.environ['PATH'],
                                               "FAKE_ROOT": str(self.root)})
        self.env.start()
        self.root_patch = mock.patch.object(backup, "require_root")
        self.root_patch.start()

    def tearDown(self):
        self.root_patch.stop()
        self.env.stop()
        self.temporary.cleanup()

    def receipt(self):
        files = list((self.root / "state").glob("*/receipt.json"))
        self.assertEqual(len(files), 1)
        return json.loads(files[0].read_text())

    def objects(self, name):
        return list((self.root / "cloud").rglob(name))

    def failure(self, environment, marker_absent=True):
        with mock.patch.dict(os.environ, environment):
            with self.assertRaises(backup.BackupError):
                backup.execute(self.args)
        self.assertEqual(self.receipt()["status"], "failed")
        self.assertFalse((self.root / "state/latest-success.json").exists())
        if marker_absent:
            self.assertFalse(self.objects("SUCCESS.json"))

    def test_success_is_read_back_and_receipts_are_private(self):
        receipt = backup.execute(self.args)
        self.assertEqual(receipt["status"], "success")
        self.assertFalse(receipt["pitr_verified"])
        self.assertEqual(receipt["scope"], "database-only")
        self.assertEqual(json.loads((self.root / "state/latest-success.json").read_text()), receipt)
        marker = json.loads(self.objects("SUCCESS.json")[0].read_text())
        manifest = self.objects("manifest.json")[0].read_bytes()
        self.assertEqual(marker["manifest_sha256"], hashlib.sha256(manifest).hexdigest())
        for path in (self.root / "state").rglob("*"):
            self.assertEqual(path.stat().st_mode & 0o777, 0o700 if path.is_dir() else 0o600)
        calls = (self.root / "remote-calls").read_text().splitlines()
        self.assertEqual(calls[-2:], ["rcat SUCCESS.json", "cat SUCCESS.json"])
        self.assertFalse((self.root / "cancelled").exists())
        docker_calls = (self.root / "docker-calls").read_text()
        self.assertNotIn("--source-data", docker_calls)
        self.assertNotIn("--flush-logs", docker_calls)
        self.assertIn("--single-transaction", docker_calls)
        self.assertIn("--no-tablespaces", docker_calls)

    def test_dump_failure_even_if_gzip_and_upload_succeed(self):
        self.failure({"FAIL_DUMP": "1"})
        self.assertTrue(self.objects("mysql.sql.gz"))
        self.assertTrue((self.root / "cancelled").exists())

    def test_upload_failure_does_not_commit(self):
        self.failure({"FAIL_UPLOAD": "mysql.sql.gz"})

    def test_valid_but_different_remote_gzip_is_not_success(self):
        self.failure({"BAD_HASH": "1"})

    def test_corrupt_remote_gzip_is_not_success(self):
        self.failure({"BAD_GZIP": "1"})

    def test_extra_member_is_not_success(self):
        self.failure({"EXTRA_MEMBER": "1"})

    def test_remote_exit_failure_after_correct_bytes_is_not_success(self):
        self.failure({"FAIL_READ": "mysql.sql.gz"})

    def test_manifest_upload_failure_does_not_commit(self):
        self.failure({"FAIL_UPLOAD": "manifest.json"})

    def test_manifest_readback_failure_does_not_commit(self):
        self.failure({"FAIL_READ": "manifest.json"})

    def test_manifest_readback_extra_bytes_does_not_commit(self):
        self.failure({"BAD_READ": "manifest.json"})

    def test_marker_upload_failure_never_updates_local_latest(self):
        self.failure({"FAIL_UPLOAD": "SUCCESS.json"}, marker_absent=False)

    def test_marker_readback_failure_never_updates_local_latest(self):
        self.failure({"FAIL_READ": "SUCCESS.json"}, marker_absent=False)

    def test_failed_run_preserves_previous_latest_success(self):
        receipt = backup.execute(self.args)
        previous = (self.root / "state/latest-success.json").read_bytes()
        with mock.patch.dict(os.environ, {"FAIL_DUMP": "1"}):
            with self.assertRaises(backup.BackupError):
                backup.execute(self.args)
        self.assertEqual((self.root / "state/latest-success.json").read_bytes(), previous)
        self.assertEqual(len(self.objects("SUCCESS.json")), 1)
        self.assertEqual(json.loads(previous)["run_id"], receipt["run_id"])

    def test_non_innodb_table_refuses_dump(self):
        self.failure({"NON_INNODB": "1"})
        self.assertFalse((self.root / "remote-calls").exists())

    def test_preflight_failure_refuses_dump(self):
        self.failure({"FAIL_PREFLIGHT": "1"})

    def test_rclone_environment_overrides_removed_aws_credentials_preserved(self):
        with mock.patch.dict(os.environ, {"RCLONE_CONFIG_R2_BACKUP_TYPE": "local",
                                          "RCLONE_CRYPT_NO_DATA_ENCRYPTION": "true",
                                          "AWS_ACCESS_KEY_ID": "fixture-access",
                                          "AWS_SECRET_ACCESS_KEY": "fixture-secret"}):
            backup.execute(self.args)
        actual = json.loads((self.root / "environment.json").read_text())
        self.assertEqual(actual, {"AWS_ACCESS_KEY_ID": "fixture-access", "AWS_SECRET_ACCESS_KEY": "fixture-secret"})
        for path in (self.root / "state").rglob("*.json"):
            self.assertNotIn("fixture-secret", path.read_text())

    def test_job_lock_competition_creates_no_receipt(self):
        with backup.locked(self.args.job_lock):
            with self.assertRaisesRegex(backup.BackupError, "holds the lock"):
                backup.execute(self.args)
        self.assertFalse((self.root / "state").exists())

    def test_release_lock_competition_records_failure_without_dump(self):
        with backup.locked(self.args.release_lock):
            self.failure({})
        self.assertFalse((self.root / "docker-calls").exists())

    def test_preview_is_read_only_and_default(self):
        self.args.action = "preview"
        result = backup.execute(self.args)
        self.assertFalse(result["writes"])
        self.assertFalse((self.root / "state").exists())
        self.assertFalse((self.root / "job.lock").exists())
        self.assertFalse((self.root / "docker-calls").exists())
        self.assertEqual(backup.parse_args([]).action, "preview")

    def test_timeout_cancels_container_and_records_failure(self):
        self.args.timeout_seconds = 1
        self.failure({"SLOW_DUMP": "1"})
        self.assertTrue((self.root / "cancelled").exists())

    def test_start_disk_floor_prevents_mysql_call(self):
        disk = shutil._ntuple_diskusage(10000, 9999, 1)
        with mock.patch.object(backup.shutil, "disk_usage", return_value=disk):
            self.failure({})
        self.assertFalse((self.root / "docker-calls").exists())

    def test_low_water_mark_cancels_dump(self):
        high = shutil._ntuple_diskusage(10000, 100, 9900)
        low = shutil._ntuple_diskusage(10000, 9999, 1)
        with mock.patch.object(backup.shutil, "disk_usage", side_effect=[high, low]):
            self.failure({})
        self.assertTrue((self.root / "cancelled").exists())

    def test_plaintext_remote_rejected(self):
        self.args.remote = "r2:bucket/full"
        with self.assertRaisesRegex(backup.BackupError, "crypt remote"):
            backup.execute(self.args)

    def test_crypt_no_data_encryption_rejected(self):
        with self.config.open("a") as stream:
            stream.write("no_data_encryption = true\n")
        with self.assertRaisesRegex(backup.BackupError, "encryption must be enabled"):
            backup.execute(self.args)

    def test_public_config_rejected(self):
        self.config.chmod(0o644)
        with self.assertRaisesRegex(backup.BackupError, "0600"):
            backup.execute(self.args)

    def test_symlink_config_rejected(self):
        link = self.root / "linked.conf"
        link.symlink_to(self.config)
        self.args.rclone_config = str(link)
        with self.assertRaisesRegex(backup.BackupError, "symlink"):
            backup.execute(self.args)

    def test_hardlinked_config_rejected(self):
        os.link(self.config, self.root / "hardlink.conf")
        with self.assertRaisesRegex(backup.BackupError, "regular files"):
            backup.execute(self.args)

    def test_unsafe_state_directory_rejected(self):
        path = self.root / "state"
        path.mkdir(mode=0o755)
        with self.assertRaisesRegex(backup.BackupError, "0700"):
            backup.execute(self.args)

    def test_config_change_during_job_fails_closed(self):
        with backup.Processes() as processes:
            remote = backup.CryptRemote(self.args.remote, self.config, processes)
            with self.config.open("a") as stream:
                stream.write("\n# changed\n")
            with self.assertRaisesRegex(backup.BackupError, "configuration changed"):
                remote.command("cat", "anything")

    def test_executable_config_option_rejected(self):
        with self.config.open("a") as stream:
            stream.write("password_command = touch forbidden\n")
        with self.assertRaisesRegex(backup.BackupError, "executable"):
            backup.execute(self.args)


class GzipIntegrityTests(unittest.TestCase):
    def test_single_member_is_verified(self):
        data = gzip.compress(b"SQL" * 1000)
        self.assertEqual(backup.gzip_digest(io.BytesIO(data), len(data)),
                         {"size": len(data), "sha256": hashlib.sha256(data).hexdigest()})

    def test_truncation_trailing_and_second_member_rejected(self):
        data = gzip.compress(b"SQL")
        for invalid in (data[:-1], data + b"junk", data + gzip.compress(b"SQL"), b""):
            with self.subTest(invalid=invalid), self.assertRaises(backup.BackupError):
                backup.gzip_digest(io.BytesIO(invalid), len(invalid))

    def test_large_compression_ratio_uses_bounded_decompression(self):
        data = gzip.compress(b"0" * (backup.BLOCK * 10))
        self.assertEqual(backup.gzip_digest(io.BytesIO(data), len(data))["size"], len(data))


@unittest.skipUnless(sys.platform.startswith("linux") and shutil.which("timeout"),
                     "requires Linux /proc and coreutils timeout")
class ContainerCancellationTests(unittest.TestCase):
    """Run the actual container shell wrapper without using any Docker daemon."""
    def setUp(self):
        PipelineTests.setUp(self)
        (self.bin / "docker").write_text("#!" + sys.executable + "\n" + r'''
import subprocess,sys
# Mimic a Docker client: its container exec survives termination of the client.
child=subprocess.Popen(sys.argv[3:], start_new_session=True)
sys.exit(child.wait())
''')
        (self.bin / "mysql").write_text("#!" + sys.executable + "\nprint('8.0.46\\n4\\t0')\n")
        (self.bin / "mysql").chmod(0o700)
        (self.bin / "mysqldump").write_text("#!" + sys.executable + "\n" + r'''
import os,signal,sys,time
from pathlib import Path
Path(os.environ['FAKE_ROOT'],'dump-pid').write_text(str(os.getpid()))
signal.signal(signal.SIGTERM, signal.SIG_IGN)
while True:
    sys.stdout.buffer.write(b'INSERT INTO example VALUES (1);\n'*100)
    sys.stdout.buffer.flush()
    time.sleep(0.05)
''')
        (self.bin / "mysqldump").chmod(0o700)
        self.password = mock.patch.dict(os.environ, {"MYSQL_ROOT_PASSWORD": "isolated-fixture-password"})
        self.password.start()

    def tearDown(self):
        # Clean up a failed test without targeting any unrelated process group.
        path = self.root / "dump-pid"
        if path.exists():
            pid = int(path.read_text())
            try:
                command = Path("/proc", str(pid), "cmdline").read_bytes()
                if str(self.bin / "mysqldump").encode() in command:
                    group = os.getpgid(pid)
                    if group != os.getpgrp():
                        os.killpg(group, signal.SIGKILL)
            except (ProcessLookupError, FileNotFoundError):
                pass
        self.password.stop()
        PipelineTests.tearDown(self)

    def assert_dump_exited(self):
        pid = int((self.root / "dump-pid").read_text())
        for _ in range(50):
            try:
                state = Path("/proc", str(pid), "stat").read_text().split()[2]
                if state == "Z":  # cannot run or retain a DB snapshot
                    return
            except FileNotFoundError:
                return
            time.sleep(0.1)
        self.fail("container mysqldump survived cancellation")

    def test_job_timeout_terminates_term_ignoring_container_dump(self):
        self.args.timeout_seconds = 1
        with self.assertRaisesRegex(backup.BackupError, "time limit"):
            backup.execute(self.args)
        self.assert_dump_exited()
        receipt = PipelineTests.receipt(self)
        self.assertEqual(receipt["status"], "failed")
        self.assertFalse((self.root / "state/latest-success.json").exists())

    def test_container_timeout_limits_dump_even_without_job_deadline(self):
        self.args.timeout_seconds = 30
        self.args.dump_timeout_seconds = 1
        with self.assertRaisesRegex(backup.BackupError, "mysqldump failed"):
            backup.execute(self.args)
        self.assert_dump_exited()
        self.assertFalse((self.root / "state/latest-success.json").exists())


if __name__ == "__main__":
    unittest.main()
