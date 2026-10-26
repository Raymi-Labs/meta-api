"""Exercise protected migration CLI without a daemon or production data."""
import fcntl
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest

RELEASE = Path(__file__).resolve().parents[1] / "newapi-release"
CONTAINER = "a" * 64
IMAGE_ID = "sha256:" + "b" * 64
IMAGE = "ghcr.io/nanashiwang/new-api@sha256:" + "c" * 64
SCHEMA = "d" * 64

DOCKER = r'''
import json,os,sys,time
from pathlib import Path
root=Path(os.environ["NEWAPI_APP_DIR"])
path=root/"runtime.json"
data=json.loads(path.read_text())
args=sys.argv[1:]
case=os.environ.get("CASE","")
with (root/"events").open("a") as out: out.write("docker:"+" ".join(args)+"\n")
if args[0]=="info": sys.exit(0)
if args[0]=="image":
    if case=="missing-image": sys.exit(1)
    print("sha256:"+"b"*64)
elif args[0]=="inspect":
    target=args[-1]
    if target=="test-mysql":
        print("mysql-id mysql-image running mysql-start" if case!="mysql-stopped" else "mysql-id mysql-image exited mysql-start")
        sys.exit(0)
    if data["status"]=="missing": sys.exit(1)
    if "-f" not in args: sys.exit(0)
    fields={".Id":data["id"],".Image":data["image"],".State.Status":data["status"],
            ".State.ExitCode":str(data.get("exit",0)),".State.OOMKilled":"false",
            ".HostConfig.RestartPolicy.Name":data.get("policy","always"),
            ".HostConfig.RestartPolicy.MaximumRetryCount":"0"}
    print(" ".join(fields[v.strip("{}")] for v in args[2].split()))
elif args[0]=="exec":
    command=args[-1]
    if "metadata_locks" in command: print(1 if case=="snapshot" else 0)
    elif "/proc/net/tcp" in command:
        assert args[1]==data["id"]
        if case=="unknown-connections": sys.exit(1)
        print(1 if case=="active-request" else 0)
    elif "MYSQL_DATABASE" in command: print("new-api")
    elif "SCHEMATA" in command:
        print({"empty-db":"1\n0\n0\n1\t1","multiple-db":"2\n76\n0\n1\t1",
               "non-innodb":"1\n76\n1\n1\t1","bad-readonly":"1\n76\n0\n0\t1"}.get(case,"1\n76\n0\n1\t1"))
    elif "information_schema.COLUMNS" in command: print("schema-data")
    else: raise AssertionError("unexpected mysql command")
elif args[0]=="compose":
    if "config" in args:
        print(json.dumps({"services":{"new-api":{"container_name":"test-app","image":os.environ["TEST_IMAGE"] if case!="compose-drift" else "latest", "environment":{"PASSWORD":"never-output-this-secret"}}}}))
    elif "up" in args:
        assert all(flag in args for flag in ("--no-start","--no-deps","--no-build","--no-recreate"))
        assert args[args.index("--pull")+1]=="never"
        if case=="create-error": sys.exit(1)
        data["status"]="running" if case=="unexpected-start" else "created"
        path.write_text(json.dumps(data))
        if case=="interrupt": time.sleep(20)
    else: raise AssertionError("unexpected compose action")
elif args[0]=="update":
    assert args==["update","--restart=no",data["id"]]
    if case=="policy-update-error": sys.exit(1)
    if case!="policy-not-disabled": data["policy"]="no"
    path.write_text(json.dumps(data))
    if case=="interrupt-policy": time.sleep(20)
elif args[0]=="stop":
    assert args[-1]==data["id"]
    if case=="stop-error": sys.exit(1)
    data.update(status="exited",exit=137 if case=="forced-stop" else 0)
    path.write_text(json.dumps(data))
    if case=="interrupt": time.sleep(20)
else: raise AssertionError("unexpected Docker mutation: "+str(args))
'''


class MigrationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="newapi-migration-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.runtime = {"id": CONTAINER, "image": IMAGE_ID, "status": "running", "policy": "always"}
        self.save()
        (self.root / "docker-compose.yml").write_text("# synthetic compose\n")
        self.script("docker", DOCKER)
        self.script("id", 'print("0")')
        self.script("curl", 'raise AssertionError("migration must not send HTTP requests")')
        self.script("timeout", 'import os,sys\na=sys.argv[3:];os.execvp(a[0],a)')
        self.script("flock", 'import fcntl,sys\nfcntl.flock(int(sys.argv[-1]),fcntl.LOCK_EX|fcntl.LOCK_NB)')
        self.script("sha256sum", 'import os,sys\nsys.stdin.read();print(("e" if os.environ.get("CASE")=="schema-drift" else "d")*64+"  -")')
        self.env = {**os.environ, "PATH": str(self.bin) + os.pathsep + os.environ["PATH"],
                    "NEWAPI_APP_DIR": str(self.root), "NEWAPI_CONTAINER_NAME": "test-app",
                    "NEWAPI_MYSQL_CONTAINER": "test-mysql", "TEST_IMAGE": IMAGE,
                    "NEWAPI_LOCK_FILE": str(self.root / "release.lock"), "NEWAPI_RESTART_STOP_TIMEOUT": "2"}

    def script(self, name, body):
        path = self.bin / name
        path.write_text(f"#!{sys.executable}\n{body}\n")
        path.chmod(0o755)

    def save(self):
        (self.root / "runtime.json").write_text(json.dumps(self.runtime))

    def create_args(self):
        self.runtime["status"] = "missing"
        self.save()
        return ["migration-create", IMAGE, SCHEMA]

    def stop_args(self):
        return ["migration-stop", CONTAINER, IMAGE_ID, "--ingress-frozen"]

    def invoke(self, args, **env):
        result = subprocess.run(["bash", str(RELEASE), *args], env={**self.env, **env},
                                text=True, capture_output=True, timeout=12)
        self.assertNotIn("never-output-this-secret", result.stdout + result.stderr)
        self.assertFalse((self.root / ".release-manager/state.env").exists())
        return result

    def history(self):
        path = self.root / ".release-manager/history.tsv"
        return path.read_text() if path.exists() else ""

    def events(self):
        path = self.root / "events"
        return path.read_text() if path.exists() else ""

    def test_stop_only_expected_application_after_drain(self):
        result = self.invoke(self.stop_args())
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads((self.root / "runtime.json").read_text())["status"], "exited")
        self.assertEqual(json.loads((self.root / "runtime.json").read_text())["policy"], "no")
        receipts = list((self.root / ".release-manager").glob("migration-stop.*"))
        self.assertEqual(len(receipts), 1)
        self.assertEqual(json.loads(receipts[0].read_text())["original_restart_policy"], {"Name": "always", "MaximumRetryCount": 0})
        self.assertEqual(receipts[0].stat().st_mode & 0o777, 0o600)
        self.assertIn("\tstarted\t", self.history())
        self.assertIn("\tsuccess\t", self.history())
        self.assertNotIn("docker:start", self.events())

    def test_stop_requires_explicit_frozen_ingress_and_exact_identity(self):
        for args in (self.stop_args()[:-1], ["migration-stop", "f" * 64, IMAGE_ID, "--ingress-frozen"],
                     ["migration-stop", CONTAINER, "sha256:" + "f" * 64, "--ingress-frozen"]):
            with self.subTest(args=args):
                self.assertNotEqual(self.invoke(args).returncode, 0)
                self.assertNotIn("docker:stop", self.events())

    def test_stop_refuses_connections_unknown_state_and_backup_guard(self):
        for case in ("active-request", "unknown-connections", "snapshot"):
            with self.subTest(case=case):
                self.assertNotEqual(self.invoke(self.stop_args(), CASE=case).returncode, 0)
                self.assertNotIn("docker:stop", self.events())

    def test_stop_failure_does_not_restart_or_report_success(self):
        for case in ("stop-error", "forced-stop"):
            with self.subTest(case=case):
                self.save()
                self.assertNotEqual(self.invoke(self.stop_args(), CASE=case).returncode, 0)
                self.assertNotIn("migration-stop\t" + CONTAINER + "\t" + IMAGE_ID + "\tsuccess\t", self.history())
                self.assertIn("\tfailed\t", self.history())
                self.assertNotIn("docker:start", self.events())

    def test_dry_runs_never_mutate(self):
        for args in (self.stop_args(),):
            self.assertEqual(self.invoke([*args, "--dry-run"]).returncode, 0)
        self.assertEqual(self.invoke([*self.create_args(), "--dry-run"]).returncode, 0)
        self.assertEqual(self.history(), "")
        self.assertNotIn("docker:stop", self.events())
        self.assertNotIn("docker:update", self.events())
        self.assertNotIn(" up ", self.events())

    def test_create_is_local_and_remains_created(self):
        result = self.invoke(self.create_args())
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads((self.root / "runtime.json").read_text())["status"], "created")
        self.assertIn("\tsuccess\t", self.history())
        self.assertNotIn("docker:start", self.events())
        self.assertNotIn("docker:pull", self.events())

    def test_create_refuses_unsafe_image_schema_database_and_config(self):
        for case in ("missing-image", "compose-drift", "mysql-stopped", "snapshot", "empty-db", "multiple-db", "non-innodb", "bad-readonly", "schema-drift"):
            with self.subTest(case=case):
                self.assertNotEqual(self.invoke(self.create_args(), CASE=case).returncode, 0)
                self.assertNotIn(" up ", self.events())

    def test_create_rejects_existing_application_or_release_state(self):
        self.assertNotEqual(self.invoke(["migration-create", IMAGE, SCHEMA]).returncode, 0)
        args = self.create_args()
        state = self.root / ".release-manager/state.env"
        state.parent.mkdir()
        state.write_text("preserve=1\n")
        result = subprocess.run(["bash", str(RELEASE), *args], env=self.env, text=True, capture_output=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(state.read_text(), "preserve=1\n")
        self.assertNotIn(" up ", self.events())

    def test_create_failures_do_not_start_or_initialize(self):
        for case in ("create-error", "unexpected-start"):
            with self.subTest(case=case):
                self.assertNotEqual(self.invoke(self.create_args(), CASE=case).returncode, 0)
                self.assertIn("\tfailed\t", self.history())
                self.assertNotIn("\tsuccess\t", self.history())
                self.assertNotIn("docker:start", self.events())

    def test_mutable_or_foreign_image_rejected(self):
        for image in ("latest", IMAGE.replace("new-api", "other"), IMAGE + "extra"):
            args = self.create_args()
            args[1] = image
            self.assertNotEqual(self.invoke(args).returncode, 0)
            self.assertNotIn(" up ", self.events())

    def test_both_commands_share_deployment_lock(self):
        with (self.root / "release.lock").open("w") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            for args in (self.stop_args(), self.create_args()):
                result = self.invoke(args)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("do not delete the lock file", result.stderr)
        self.assertNotIn(" up ", self.events())
        self.assertNotIn("docker:stop", self.events())

    def test_interrupt_preserves_started_evidence_and_never_auto_restarts(self):
        for command in ("stop", "create"):
            with self.subTest(command=command):
                self.runtime["status"] = "running"
                self.save()
                args = self.stop_args() if command == "stop" else self.create_args()
                process = subprocess.Popen(["bash", str(RELEASE), *args], env={**self.env, "CASE": "interrupt"},
                                           stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, start_new_session=True)
                try:
                    deadline = time.monotonic() + 8
                    while time.monotonic() < deadline:
                        status = json.loads((self.root / "runtime.json").read_text())["status"]
                        if status in ("exited", "created"):
                            break
                        time.sleep(0.05)
                    else:
                        self.fail("interruption fixture never reached mutation")
                    os.killpg(process.pid, signal.SIGTERM)
                    process.communicate(timeout=5)
                    self.assertNotEqual(process.returncode, 0)
                    self.assertIn("\tstarted\t", self.history())
                    self.assertNotIn("migration-stop\t" + CONTAINER + "\t" + IMAGE_ID + "\tsuccess\t", self.history())
                    self.assertNotIn("docker:start", self.events())
                finally:
                    if process.poll() is None:
                        os.killpg(process.pid, signal.SIGKILL)
                    process.communicate()

    def test_policy_update_failure_or_unverified_policy_prevents_stop(self):
        for case in ("policy-update-error", "policy-not-disabled"):
            with self.subTest(case=case):
                self.save()
                self.assertNotEqual(self.invoke(self.stop_args(), CASE=case).returncode, 0)
                self.assertNotIn("docker:stop", self.events())
                self.assertIn("migration-policy\t" + CONTAINER + "\t" + IMAGE_ID + "\tfailed\t", self.history())
                self.assertNotIn("docker:start", self.events())

    def test_interrupt_after_policy_update_keeps_writer_restart_disabled(self):
        process = subprocess.Popen(["bash", str(RELEASE), *self.stop_args()], env={**self.env, "CASE": "interrupt-policy"},
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, start_new_session=True)
        try:
            deadline = time.monotonic() + 8
            while time.monotonic() < deadline:
                if json.loads((self.root / "runtime.json").read_text())["policy"] == "no":
                    break
                time.sleep(0.05)
            else:
                self.fail("policy fixture never reached update")
            os.killpg(process.pid, signal.SIGTERM)
            process.communicate(timeout=5)
            self.assertNotEqual(process.returncode, 0)
            self.assertEqual(json.loads((self.root / "runtime.json").read_text())["policy"], "no")
            self.assertNotIn("docker:stop", self.events())
            self.assertNotIn("docker:start", self.events())
            self.assertTrue(list((self.root / ".release-manager").glob("migration-stop.*")))
        finally:
            if process.poll() is None:
                os.killpg(process.pid, signal.SIGKILL)
            process.communicate()


if __name__ == "__main__":
    unittest.main()
