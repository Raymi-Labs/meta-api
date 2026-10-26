#!/usr/bin/env python3
"""Opt-in Linux/root migration exercise: private MySQL, internal network, local images.

The optional schema dump must contain schema ONLY, never production rows/options.
All resources have random names and are removed; no published ports or host data
mounts are used. The real release entrypoint runs twice against schema clones.
"""
import argparse
import json
import os
from pathlib import Path
import re
import secrets
import shlex
import subprocess
import tempfile
import time
import uuid

RELEASE = Path(__file__).resolve().parents[1] / "newapi-release"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--image", required=True, help="already-local app repo@sha256 digest")
    parser.add_argument("--mysql-image", required=True, help="already-local MySQL 8.0.46 digest")
    parser.add_argument("--schema-file", type=Path, help="schema-only dump without rows, database/user DDL or routines")
    args = parser.parse_args()
    if not args.execute or os.geteuid() != 0:
        parser.error("run as root on Linux with --execute after reviewing the fixture")
    for image in (args.image, args.mysql_image):
        if not re.fullmatch(r"[^\s]+@sha256:[a-f0-9]{64}", image):
            parser.error("both images must be immutable digests already present locally")
    schema = None
    if args.schema_file:
        schema = args.schema_file.read_text()
        # This is a schema fixture boundary, not a SQL security parser. Use only
        # an operator-reviewed mysqldump --no-data --skip-triggers output.
        if re.search(r"(?im)^\s*(INSERT|REPLACE|UPDATE|DELETE|CALL|USE|CREATE\s+(DATABASE|USER|PROCEDURE|FUNCTION|EVENT|TRIGGER))\b", schema):
            parser.error("schema fixture contains rows or unsupported statements")

    def run(command, *, env=None, data=None, timeout=120, expected=0):
        result = subprocess.run(command, input=data, text=True, capture_output=True, env=env, timeout=timeout)
        if result.returncode != expected:
            # Do not include raw commands, SQL, Compose configuration or logs.
            raise RuntimeError(f"fixture command failed (exit {result.returncode}, expected {expected}); phase={phase}")
        return result.stdout.strip()

    def docker(*arguments, **kwargs):
        return run(["docker", *arguments], **kwargs)

    phase = "validate local images"
    app_id = docker("image", "inspect", "-f", "{{.Id}}", args.image)
    docker("image", "inspect", args.mysql_image)
    name = "newapi-migration-e2e-" + uuid.uuid4().hex[:12]
    network, mysql, app, initializer = [name + suffix for suffix in ("-net", "-mysql", "-app", "-init")]
    created = set()
    network_created = False
    holding = None
    with tempfile.TemporaryDirectory(prefix=name + "-") as directory:
        root = Path(directory)
        root.chmod(0o700)
        password = secrets.token_hex(24)
        mysql_env = root / "mysql.env"
        mysql_env.write_text(f"MYSQL_ROOT_PASSWORD={password}\nMYSQL_DATABASE=new-api\nMYSQL_USER=fixture\nMYSQL_PASSWORD={password}\n")
        mysql_env.chmod(0o600)
        app_env = root / "app.env"
        app_env.write_text(f"SQL_DSN=fixture:{password}@tcp({mysql}:3306)/new-api?charset=utf8mb4&parseTime=true&loc=Local\nGIN_MODE=release\n")
        app_env.chmod(0o600)
        compose = {"name": name, "services": {"new-api": {
            "image": args.image, "container_name": app, "env_file": str(app_env),
            "restart": "no", "networks": ["fixture"], "mem_limit": "512m",
            "security_opt": ["no-new-privileges:true"], "cap_drop": ["ALL"]}},
            "networks": {"fixture": {"external": True, "name": network}}}
        (root / "docker-compose.yml").write_text(json.dumps(compose))
        # Internal networking intentionally publishes no host port. This curl
        # adapter performs a real HTTP request from the fixture namespace so
        # the unmodified restart health function can verify JSON success.
        bin_dir = root / "bin"
        bin_dir.mkdir()
        curl = bin_dir / "curl"
        curl.write_text("#!/bin/sh\nexec docker exec " + shlex.quote(app) + " wget -T 5 -qO- http://127.0.0.1:3000/api/status\n")
        curl.chmod(0o700)
        env = {**os.environ, "PATH": str(bin_dir) + os.pathsep + os.environ["PATH"],
               "NEWAPI_APP_DIR": str(root), "NEWAPI_STATE_DIR": str(root / "state"),
               "NEWAPI_LOCK_FILE": str(root / "release.lock"), "NEWAPI_CONTAINER_NAME": app,
               "NEWAPI_MYSQL_CONTAINER": mysql, "NEWAPI_DOCKER_NETWORK": network,
               "NEWAPI_HEALTH_TIMEOUT": "90", "NEWAPI_RESTART_STOP_TIMEOUT": "40",
               "NEWAPI_IMAGE_REPOSITORY": args.image.split("@")[0]}

        def sql(query):
            return docker("exec", "-i", mysql, "sh", "-c",
                          'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql --user=root --protocol=SOCKET --batch --skip-column-names new-api', data=query)

        def invoke(*arguments, expected=0):
            return run(["bash", str(RELEASE), *arguments], env=env, expected=expected, timeout=180)

        def fingerprint():
            return run(["bash", "-c", 'source "$1"; schema_fingerprint new-api', "fixture", str(RELEASE)], env=env)

        def health(container):
            deadline = time.monotonic() + 90
            while time.monotonic() < deadline:
                probe = subprocess.run(["docker", "exec", container, "wget", "-T", "2", "-qO-", "http://127.0.0.1:3000/api/status"],
                                       capture_output=True, text=True, timeout=10)
                if probe.returncode == 0 and json.loads(probe.stdout).get("success") is True:
                    return
                time.sleep(1)
            raise RuntimeError("isolated application did not become healthy")

        def snapshot(container):
            return docker("inspect", "-f", "{{.Id}} {{.Image}} {{.State.Status}} {{.State.StartedAt}}", container)

        try:
            phase = "create private MySQL"
            docker("network", "create", "--internal", network)
            network_created = True
            created.add(mysql)
            docker("run", "-d", "--pull=never", "--name", mysql, "--network", network,
                   "--env-file", str(mysql_env), "--memory=2g", "--pids-limit=256", "--restart=no",
                   "--tmpfs", "/var/lib/mysql:rw,size=1g", args.mysql_image,
                   "--character-set-server=utf8mb4", "--collation-server=utf8mb4_unicode_ci",
                   "--innodb-buffer-pool-size=128M", "--server-id=987654", "--log-bin=mysql-bin",
                   "--binlog-format=ROW", "--binlog-row-image=FULL", "--gtid-mode=OFF")
            deadline = time.monotonic() + 120
            while time.monotonic() < deadline:
                # The image's temporary initialization server has a socket but
                # no TCP listener; wait for the final server, not that phase.
                ready = subprocess.run(["docker", "exec", mysql, "sh", "-c", 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql --user=root --host=127.0.0.1 --protocol=TCP --batch --skip-column-names --execute="SELECT 1" new-api'], capture_output=True, timeout=10)
                if ready.returncode == 0:
                    break
                time.sleep(1)
            else:
                raise RuntimeError("isolated MySQL did not become ready")
            assert sql("SELECT VERSION();").startswith("8.0.46")
            mysql_before = snapshot(mysql)
            phase = "initialize synthetic schema"
            if schema is not None:
                sql(schema)
            else:
                created.add(initializer)
                docker("run", "-d", "--pull=never", "--name", initializer, "--network", network,
                       "--env-file", str(app_env), "--memory=512m", "--restart=no", args.image)
                health(initializer)
                docker("stop", "--time", "40", initializer, timeout=60)
                docker("rm", "-v", initializer)
                created.remove(initializer)
            source_schema = fingerprint()
            assert re.fullmatch("[a-f0-9]{64}", source_schema)
            phase = "two real MySQL schema-clone application passes"
            # preflight_target copies no options, rows, tasks or payment data.
            # Its containers inherit this fixture's internal-only network.
            run(["bash", "-c", 'source "$1"; TARGET_IMAGE="$2"; TARGET_ID="$3"; PREFLIGHT_PASSES=2; preflight_target',
                 "fixture", str(RELEASE), args.image, app_id], env=env, timeout=240)
            assert fingerprint() == source_schema
            assert sql("SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME NOT IN ('mysql','information_schema','performance_schema','sys');") == "1"
            phase = "guarded readonly-target creation"
            sql("SET GLOBAL super_read_only=ON;")
            invoke("migration-create", args.image, "0" * 64, expected=1)
            invoke("migration-create", args.image, source_schema, "--dry-run")
            created.add(app)
            invoke("migration-create", args.image, source_schema)
            app_info = json.loads(docker("inspect", app))[0]
            assert app_info["State"]["Status"] == "created" and app_info["Image"] == app_id
            assert not app_info["HostConfig"]["PortBindings"]
            assert not (root / "state/state.env").exists()
            invoke("migration-create", args.image, source_schema, expected=1)
            assert sql("SELECT @@global.read_only,@@global.super_read_only;") == "1\t1"
            phase = "same-version restart and clean drain"
            sql("SET GLOBAL super_read_only=OFF; SET GLOBAL read_only=OFF;")
            invoke("restart")
            app_identity = json.loads(docker("inspect", app))[0]
            assert app_identity["Id"] == app_info["Id"] and app_identity["Image"] == app_id
            holding = subprocess.Popen(["docker", "exec", mysql, "bash", "-c", f"exec 3<>/dev/tcp/{app}/3000; sleep 20"],
                                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            time.sleep(1)
            invoke("migration-stop", app_info["Id"], app_id, "--ingress-frozen", expected=1)
            holding.wait(timeout=25)
            holding = None
            docker("update", "--restart=always", app)
            invoke("migration-stop", app_info["Id"], app_id, "--ingress-frozen", "--dry-run")
            assert json.loads(docker("inspect", app))[0]["HostConfig"]["RestartPolicy"]["Name"] == "always"
            invoke("migration-stop", app_info["Id"], app_id, "--ingress-frozen")
            stopped = json.loads(docker("inspect", app))[0]
            assert stopped["State"]["Status"] == "exited" and stopped["State"]["ExitCode"] == 0
            assert stopped["HostConfig"]["RestartPolicy"]["Name"] == "no"
            receipts = list((root / "state").glob("migration-stop.*"))
            assert len(receipts) == 1
            assert json.loads(receipts[0].read_text())["original_restart_policy"]["Name"] == "always"
            assert snapshot(mysql) == mysql_before
            assert not (root / "state/state.env").exists()
            print(json.dumps({"result": "passed", "mysql": "8.0.46", "image": args.image,
                              "schema_source": "provided-schema-only" if schema is not None else "synthetic-initialized",
                              "schema_sha256": source_schema, "preflight_passes": 2,
                              "checks": ["wrong-schema-rejected", "readonly-create", "no-dependencies-started",
                                         "same-container-restart", "active-connection-rejected", "clean-stop",
                                         "mysql-identity-unchanged", "release-state-unset", "source-restart-disabled"]}, ensure_ascii=False))
        finally:
            if holding is not None:
                holding.terminate()
                holding.wait(timeout=5)
            for container in sorted(created):
                subprocess.run(["docker", "rm", "-f", "-v", container], capture_output=True, timeout=30)
            if network_created:
                subprocess.run(["docker", "network", "rm", network], capture_output=True, timeout=30)


if __name__ == "__main__":
    main()
