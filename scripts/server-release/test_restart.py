"""Run the installed-name CLI with isolated Docker/HTTP executables, no daemon."""
import fcntl
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

RELEASE = Path(__file__).resolve().parents[1] / "newapi-release"

DOCKER = r'''
import json,os,sys
from pathlib import Path
root=Path(os.environ["NEWAPI_APP_DIR"])
path=root/"runtime.json"
data=json.loads(path.read_text())
args=sys.argv[1:]
operation=args[0]
with (root/"events").open("a") as out: out.write("docker:"+" ".join(args)+"\n")
if operation=="info":
    sys.exit(1 if os.environ.get("CASE")=="daemon-down" else 0)
if operation=="exec":
    print(1 if os.environ.get("CASE")=="pitr-snapshot-active" else 0)
    sys.exit(0)
if operation=="inspect":
    if data["status"]=="missing": sys.exit(1)
    if os.environ.get("CASE")=="crash-loop" and data.get("acted"):
        data["started"]+="x"
        path.write_text(json.dumps(data))
    fields={".Id":data["id"], ".Image":data["image"], ".State.Status":data["status"],
            ".State.StartedAt":data["started"], ".RestartCount":str(data["restarts"])}
    print(" ".join(fields[v.strip("{}")] for v in args[2].split()))
elif operation in ("start","restart"):
    assert args[-1]==data["id"], "operate on inspected immutable container ID"
    if os.environ.get("CASE")=="operation-error": sys.exit(1)
    data.update(status="running",started="new-start",acted=True)
    path.write_text(json.dumps(data))
else:
    raise RuntimeError("unexpected Docker action: "+str(args))
'''

TIMEOUT = r'''
import os,sys
from pathlib import Path
args=sys.argv[1:]
assert args.pop(0).startswith("--kill-after=")
assert int(args.pop(0))>0
if os.environ.get("CASE")=="operation-timeout" and args[:2] in (["docker","start"],["docker","restart"]):
    with (Path(os.environ["NEWAPI_APP_DIR"])/"events").open("a") as out: out.write("operation-timeout\n")
    sys.exit(124)
os.execvp(args[0], args)
'''


class RestartTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="newapi-restart-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.data = {"id": "same-container", "image": "same-image", "status": "exited",
                     "started": "old-start", "restarts": 7, "historical_logs": "[FATAL] old crash"}
        self.write_runtime()
        self.state = self.root / ".release-manager" / "state.env"
        self.state.parent.mkdir()
        self.state.write_text("CURRENT_ID=same-image\nPREVIOUS_ID=older-image\n")
        self.initial_state = self.state.read_bytes()
        self.script("docker", DOCKER)
        self.script("timeout", TIMEOUT)
        self.script("id", 'print("0")')
        self.script("flock", "import fcntl,sys\nfcntl.flock(int(sys.argv[-1]),fcntl.LOCK_EX|fcntl.LOCK_NB)")
        self.script("curl", 'import os,sys\nprint(os.environ.get("STATUS_BODY", \'{"success":true}\'))\nsys.exit(int(os.environ.get("CURL_EXIT","0")))')
        self.command = self.bin / "newapi-restart"
        self.command.symlink_to(RELEASE)
        self.env = {**os.environ, "PATH": str(self.bin) + os.pathsep + os.environ["PATH"],
                    "NEWAPI_APP_DIR": str(self.root), "NEWAPI_CONTAINER_NAME": "test-app",
                    "NEWAPI_LOCK_FILE": str(self.root / "release.lock"),
                    "NEWAPI_HEALTH_TIMEOUT": "3", "NEWAPI_RESTART_STOP_TIMEOUT": "2"}

    def script(self, name, body):
        path = self.bin / name
        path.write_text(f"#!{sys.executable}\n" + body + "\n")
        path.chmod(0o755)

    def write_runtime(self):
        (self.root / "runtime.json").write_text(json.dumps(self.data))

    def run_cli(self, *args, **env):
        events = self.root / "events"
        if events.exists():
            events.unlink()
        result = subprocess.run(["bash", str(self.command), *args], env={**self.env, **env},
                                text=True, capture_output=True, timeout=12)
        self.assertEqual(self.state.read_bytes(), self.initial_state)
        self.events = events.read_text() if events.exists() else ""
        return result

    def assert_success(self, result, operation):
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("恢复成功", result.stdout)
        self.assertIn("docker:" + operation + " ", self.events)
        data = json.loads((self.root / "runtime.json").read_text())
        self.assertEqual(data["id"], self.data["id"])
        self.assertEqual(data["image"], self.data["image"])
        self.assertIn("\tsuccess\t", (self.state.parent / "history.tsv").read_text())

    def test_stopped_container_starts_with_old_restart_count_and_fatal_logs(self):
        self.assert_success(self.run_cli(), "start")
        self.assertNotIn("docker:logs", self.events)

    def test_created_container_starts(self):
        self.data["status"] = "created"
        self.write_runtime()
        self.assert_success(self.run_cli(), "start")

    def test_pitr_snapshot_refuses_before_stopping_running_container(self):
        self.data['status'] = 'running'
        self.write_runtime()
        before = (self.root / 'runtime.json').read_bytes()
        result = self.run_cli(CASE='pitr-snapshot-active')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('PITR snapshot', result.stderr)
        self.assertNotIn('docker:restart', self.events)
        self.assertEqual((self.root / 'runtime.json').read_bytes(), before)

    def test_running_or_restarting_container_restarts(self):
        for state in ("running", "restarting"):
            with self.subTest(state=state):
                self.data["status"] = state
                self.write_runtime()
                self.assert_success(self.run_cli(), "restart")

    def test_dry_run_does_not_change_runtime_or_write_history(self):
        before = (self.root / "runtime.json").read_bytes()
        result = self.run_cli("--dry-run")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("dry-run", result.stdout)
        self.assertEqual((self.root / "runtime.json").read_bytes(), before)
        self.assertFalse((self.state.parent / "history.tsv").exists())

    def test_missing_or_paused_container_fails_without_creation(self):
        for state in ("missing", "paused", "dead", "removing"):
            with self.subTest(state=state):
                self.data["status"] = state
                self.write_runtime()
                result = self.run_cli()
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("docker:start", self.events)
                self.assertNotIn("docker:restart", self.events)

    def test_unavailable_daemon_explains_failure(self):
        result = self.run_cli(CASE="daemon-down")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Docker daemon is unavailable", result.stderr)
        self.assertNotIn("docker:inspect", self.events)

    def test_operation_errors_and_timeouts_are_reported(self):
        for case in ("operation-error", "operation-timeout"):
            with self.subTest(case=case):
                result = self.run_cli(CASE=case)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("恢复未通过", result.stderr)
                self.assertIn("\tfailed\t", (self.state.parent / "history.tsv").read_text())

    def test_http_failure_invalid_json_and_false_success_fail_health(self):
        for body, code in [('{}', '22'), ('not json', '0'), ('{"success":false}', '0')]:
            with self.subTest(body=body):
                result = self.run_cli(STATUS_BODY=body, CURL_EXIT=code, NEWAPI_HEALTH_TIMEOUT="2")
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("恢复成功", result.stdout)

    def test_crash_loop_does_not_pass_two_stable_observations(self):
        result = self.run_cli(CASE="crash-loop", NEWAPI_HEALTH_TIMEOUT="2")
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("恢复成功", result.stdout)

    def test_deployment_lock_conflict_is_immediate_and_preserves_lock(self):
        lock = self.root / "release.lock"
        with lock.open("w") as held:
            fcntl.flock(held, fcntl.LOCK_EX | fcntl.LOCK_NB)
            result = self.run_cli()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("do not delete the lock file", result.stderr)
        self.assertTrue(lock.exists())
        self.assertNotIn("docker:start", self.events)

    def test_invalid_arguments_do_not_mutate_runtime(self):
        result = self.run_cli("latest")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("restart accepts only --dry-run", result.stderr)
        self.assertNotIn("docker:start", self.events)

    def test_invalid_timeout_fails_before_docker(self):
        result = self.run_cli(NEWAPI_RESTART_STOP_TIMEOUT="0")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.events, "")

    def test_help_works_without_a_daemon(self):
        result = self.run_cli("--help", CASE="daemon-down")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("newapi-restart", result.stdout)
        self.assertEqual(self.events, "")

    def test_release_restart_subcommand_uses_same_flow(self):
        self.command = RELEASE
        self.assert_success(self.run_cli("restart"), "start")


if __name__ == "__main__":
    unittest.main()
