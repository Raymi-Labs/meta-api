#!/usr/bin/env python3
"""Opt-in real-Docker exercise using a disposable HTTP fixture, never the app."""
import argparse
import fcntl
import json
import os
from pathlib import Path
import socket
import shutil
import sys
import subprocess
import tempfile
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute", action="store_true", help="create and remove isolated test resources")
    parser.add_argument("--fixture-image", default="oven/bun:1.4.0", help="already-local fixture image; never pulled")
    args = parser.parse_args()
    if not args.execute:
        parser.error("review the exercise, then pass --execute")

    def docker(*arguments, check=True):
        return subprocess.run(["docker", *arguments], check=check, text=True, capture_output=True, timeout=30)

    image = docker("image", "inspect", "-f", "{{.Id}}", args.fixture_image).stdout.strip()
    name = "newapi-restart-e2e-" + uuid.uuid4().hex[:12]
    network = name + "-net"
    with tempfile.TemporaryDirectory(prefix=name + "-") as directory:
        root = Path(directory)
        command = root / "newapi-restart"
        command.symlink_to(Path(__file__).resolve().parents[1] / "newapi-release")
        management = root / "meta-api"
        shutil.copy2(Path(__file__).resolve().parents[1] / "meta-api", management)
        (root / "newapi-release").symlink_to(Path(__file__).resolve().parents[1] / "newapi-release")
        # The unchanged MySQL snapshot guard is covered separately by unit and
        # real-MySQL tests. Only this synthetic guard call is stubbed here; all
        # container lifecycle operations go to the actual Docker executable.
        docker_path = shutil.which("docker")
        wrapper = root / "docker"
        wrapper.write_text(f"#!{sys.executable}\nimport os,sys\n"
                           f"if sys.argv[1:3] == ['exec', '{name}-guard']:\n    print(0)\n"
                           f"else:\n    os.execv({docker_path!r}, [{docker_path!r}, *sys.argv[1:]])\n")
        wrapper.chmod(0o755)
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            port = sock.getsockname()[1]
        env = {**os.environ, "NEWAPI_APP_DIR": directory, "NEWAPI_STATE_DIR": str(root / "state"),
               "PATH": str(root) + os.pathsep + os.environ["PATH"],
               "NEWAPI_MYSQL_CONTAINER": name + "-guard", "NEWAPI_CONTAINER_NAME": name, "NEWAPI_LOCK_FILE": str(root / "release.lock"),
               "NEWAPI_HEALTH_URL": f"http://127.0.0.1:{port}/api/status",
               "NEWAPI_HEALTH_TIMEOUT": "20", "NEWAPI_RESTART_STOP_TIMEOUT": "1"}

        def invoke(*arguments, expect=0, cli=None):
            result = subprocess.run(["bash", str(cli or command), *arguments], env=env,
                                    text=True, capture_output=True, timeout=50)
            print(result.stdout, end="", flush=True)
            if result.returncode != expect:
                raise AssertionError(f"unexpected exit {result.returncode}: {result.stderr}")
            return result

        def inspect():
            # This fixture has only synthetic data; no production container is inspected.
            return json.loads(docker("inspect", name).stdout)[0]

        network_created = False
        container_created = False
        try:
            # An internal network suppresses published ports on this Docker version.
            # Use a dedicated bridge; only loopback publishes the synthetic endpoint.
            docker("network", "create", network)
            network_created = True
            docker("create", "--pull=never", "--name", name, "--network", network,
                   "--restart=no", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges",
                   "--memory=128m", "--pids-limit=32", "--user=1000:1000",
                   "-p", f"127.0.0.1:{port}:3000", "--entrypoint", "bun", image, "-e",
                   'Bun.serve({port:3000,fetch(){return Response.json({success:true})}})')
            container_created = True
            initial = inspect()
            assert not initial["Mounts"]
            assert list(initial["NetworkSettings"]["Networks"]) == [network]
            invoke("--dry-run")
            assert inspect()["State"]["Status"] == "created"
            invoke()  # Created -> start.
            started = inspect()["State"]["StartedAt"]
            invoke()  # Running -> restart.
            assert inspect()["State"]["StartedAt"] != started
            started = inspect()["State"]["StartedAt"]
            invoke("start", cli=management)
            assert inspect()["State"]["StartedAt"] == started
            invoke("stop", "--dry-run", cli=management)
            assert inspect()["State"]["Status"] == "running"
            invoke("stop", "--yes", cli=management)
            assert inspect()["State"]["Status"] == "exited"
            invoke("start", cli=management)  # Explicit stop -> start.
            docker("kill", name)
            assert inspect()["State"]["ExitCode"] == 137
            invoke()  # SIGKILL exit -> start.
            started = inspect()["State"]["StartedAt"]
            with (root / "release.lock").open("w") as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                result = invoke(expect=1)
                assert "do not delete the lock file" in result.stderr
            invoke("--dry-run")
            final = inspect()
            assert final["State"]["StartedAt"] == started
            assert final["Id"] == initial["Id"] and final["Image"] == initial["Image"]
            assert len((root / "state" / "history.tsv").read_text().splitlines()) == 6
            print("PASS: created, running restart, idempotent start, guarded stop/start, SIGKILL, lock conflict, dry-run; same ID/image", flush=True)
        except BaseException:
            if container_created:
                print("Fixture logs:", docker("logs", "--tail", "30", name, check=False).stdout, flush=True)
                print("Fixture ports:", docker("port", name, check=False).stdout, flush=True)
                probe = docker("exec", name, "bun", "-e",
                               'fetch("http://127.0.0.1:3000/api/status").then(async r=>console.log(r.status,await r.text()))',
                               check=False)
                print("Fixture internal probe:", probe.stdout, probe.stderr, flush=True)
            raise
        finally:
            if container_created:
                docker("rm", "-f", name)
            if network_created:
                docker("network", "rm", network)


if __name__ == "__main__":
    main()
