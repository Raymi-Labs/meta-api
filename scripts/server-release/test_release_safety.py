"""Exercise the actual Bash update/recovery flow with isolated command doubles."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
RELEASE = HERE.parent / "newapi-release"


class ReleaseSafetyTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="newapi-release-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.state = self.root / ".release-manager"
        self.state.mkdir()
        self.compose = self.root / "docker-compose.yml"
        self.compose.write_text("services:\n  new-api:\n    image: old-image\n")
        self.initial_compose = self.compose.read_bytes()
        self.initial_state = (
            "CURRENT_IMAGE=old-image\nCURRENT_ID=old-image\nCURRENT_VERSION=test-old\n"
            "PREVIOUS_IMAGE=previous-image\nPREVIOUS_ID=previous-image\n"
            "PREVIOUS_VERSION=test-previous\nLAST_BACKUP_DIR=\n"
        ).encode()
        (self.state / "state.env").write_bytes(self.initial_state)
        (self.root / "runtime").write_text("old-image\n")
        self.bin = self.root / "bin"
        self.bin.mkdir()
        # macOS has no flock CLI. The inherited descriptor uses the same fcntl lock.
        flock = self.bin / "flock"
        flock.write_text(
            f"#!{sys.executable}\nimport fcntl,sys\n"
            "fcntl.flock(int(sys.argv[-1]),fcntl.LOCK_EX|fcntl.LOCK_NB)\n"
        )
        flock.chmod(0o755)
        self.env = {
            **os.environ,
            "PATH": str(self.bin) + os.pathsep + os.environ["PATH"],
            "NEWAPI_APP_DIR": str(self.root),
            "NEWAPI_LOCK_FILE": str(self.root / "release.lock"),
            "NEWAPI_HEALTH_TIMEOUT": "1",
            "RELEASE_SCRIPT": str(RELEASE),
        }

    def run_case(self, case, *args):
        result = subprocess.run(
            ["bash", str(HERE / "release-test-harness.bash"), *args],
            env={**self.env, "CASE": case}, text=True, capture_output=True, timeout=15,
        )
        events = self.root / "events"
        return result, events.read_text().splitlines() if events.exists() else []

    def assert_old_restored(self):
        self.assertEqual(self.compose.read_bytes(), self.initial_compose)
        self.assertEqual((self.state / "state.env").read_bytes(), self.initial_state)
        self.assertEqual((self.root / "runtime").read_text().strip(), "old-image")
        self.assertFalse((self.root / "stopped").exists())

    def assert_backup_unsealed(self):
        self.assertFalse((self.root / "backups/one-click/test/SHA256SUMS").exists())

    def test_success_keeps_source_online_until_switch_and_does_not_recover(self):
        result, events = self.run_case("success")
        self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
        self.assertEqual(events, ["resolve", "backup:old-image", "preflight:old-image", "up:new-image", "cleanup"])
        self.assertIn("CURRENT_IMAGE=new-image", (self.state / "state.env").read_text())
        self.assertIn("PREVIOUS_IMAGE=old-image", (self.state / "state.env").read_text())
        backup = self.root / "backups/one-click/test"
        manifest = (backup / "SHA256SUMS").read_text()
        for name in [".env", "mysql.sql.gz", "release-state-before.env", "release-state-after.env",
                     "recovery-metadata.env", "deploy-success.log"]:
            self.assertIn("  " + name + "\n", manifest)
        metadata = (backup / "recovery-metadata.env").read_text()
        self.assertIn("SOURCE_IMAGE=old-image", metadata)
        self.assertIn("TARGET_IMAGE=new-image", metadata)

    def test_backup_failure_never_recreates_service(self):
        result, events = self.run_case("backup-failure")
        self.assertEqual(result.returncode, 31)
        self.assertNotIn("up:new-image", events)
        self.assert_old_restored()
        self.assert_backup_unsealed()

    def test_preflight_failure_never_recreates_service(self):
        result, events = self.run_case("preflight-failure")
        self.assertEqual(result.returncode, 32)
        self.assertNotIn("up:new-image", events)
        self.assert_old_restored()
        self.assert_backup_unsealed()

    def test_deployment_failure_restores_service_compose_and_state(self):
        result, events = self.run_case("deploy-failure")
        self.assertEqual(result.returncode, 33, result.stderr + result.stdout)
        self.assertIn("up:old-image", events)
        self.assertIn("automatic recovery succeeded", result.stdout)
        self.assert_old_restored()
        self.assert_backup_unsealed()

    def test_failed_health_recovers_old_service(self):
        result, events = self.run_case("health-failure")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("up:old-image", events)
        self.assert_old_restored()
        self.assert_backup_unsealed()

    def test_signals_during_switch_restore_old_service(self):
        for signal_name, expected in [("HUP", 129), ("INT", 130), ("TERM", 143)]:
            with self.subTest(signal=signal_name):
                result, events = self.run_case("signal-" + signal_name)
                self.assertEqual(result.returncode, expected, result.stderr + result.stdout)
                self.assertIn("automatic recovery succeeded", result.stdout)
                self.assert_old_restored()
                self.assert_backup_unsealed()
                (self.root / "events").unlink()

    def test_sealing_failure_reports_error_and_keeps_healthy_new_service(self):
        result, events = self.run_case("seal-failure")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("application update is healthy", result.stderr)
        self.assertNotIn("up:old-image", events)
        self.assertEqual((self.root / "runtime").read_text().strip(), "new-image")
        self.assertIn("CURRENT_IMAGE=new-image", (self.state / "state.env").read_text())
        self.assertTrue((self.root / "backups/one-click/test/mysql.sql.gz").exists())
        self.assert_backup_unsealed()

    def test_missing_final_log_does_not_publish_complete_marker(self):
        result, events = self.run_case("final-log-failure")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("application update is healthy", result.stderr)
        self.assertNotIn("up:old-image", events)
        self.assertEqual((self.root / "runtime").read_text().strip(), "new-image")
        self.assert_backup_unsealed()

    def test_signal_after_success_keeps_new_service_and_incomplete_dump(self):
        result, events = self.run_case("seal-signal-TERM")
        self.assertEqual(result.returncode, 143)
        self.assertNotIn("up:old-image", events)
        self.assertEqual((self.root / "runtime").read_text().strip(), "new-image")
        self.assertTrue((self.root / "backups/one-click/test/mysql.sql.gz").exists())
        self.assert_backup_unsealed()

    def test_offline_source_fails_before_pull_or_backup(self):
        (self.root / "stopped").touch()
        result, events = self.run_case("success")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(events, ["cleanup"])
        self.assertIn("source service is offline", result.stderr)
        self.assertTrue((self.root / "stopped").exists())

    def test_unreachable_source_fails_before_backup(self):
        result, events = self.run_case("source-health-failure")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(events, ["cleanup"])
        self.assert_old_restored()

    def test_dry_run_does_not_backup_or_deploy(self):
        result, events = self.run_case("success", "--dry-run")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(events, ["resolve", "cleanup"])
        self.assert_old_restored()

    def test_offline_source_can_still_rollback(self):
        (self.root / "stopped").touch()
        result, events = self.run_case("rollback-offline")
        self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
        self.assertIn("up:previous-image", events)
        self.assertFalse((self.root / "stopped").exists())

    def test_pitr_snapshot_or_unknown_lock_state_fails_before_release_work(self):
        for case in ("pitr-snapshot-active", "pitr-snapshot-query-failure", "pitr-snapshot-invalid"):
            with self.subTest(case=case):
                result, events = self.run_case(case)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(events, ["cleanup"])
                self.assert_old_restored()
                (self.root / "events").unlink()

    def test_retired_entrypoints_cannot_call_deployment_commands(self):
        for command in ["docker", "mysql", "curl", "systemctl"]:
            path = self.bin / command
            path.write_text("#!/bin/sh\ntouch \"$NEWAPI_APP_DIR/unsafe-call\"\nexit 99\n")
            path.chmod(0o755)
        for interpreter, filename in [(sys.executable, "retired-entrypoint.py"), ("bash", "retired-entrypoint.sh")]:
            for args in [[], ["prepare", "test-image"], ["deploy"], ["--help"]]:
                with self.subTest(filename=filename, args=args):
                    result = subprocess.run([interpreter, str(HERE / filename), *args], env=self.env, text=True, capture_output=True)
                    self.assertEqual(result.returncode, 78)
                    self.assertIn("newapi-update", result.stderr)
                    self.assertFalse((self.root / "unsafe-call").exists())


if __name__ == "__main__":
    unittest.main()
