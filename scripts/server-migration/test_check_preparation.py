"""Failures must close the preparation gate without printing private config."""
import copy
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch
import io


HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("preparation", HERE / "check-preparation.py")
preparation = importlib.util.module_from_spec(spec)
spec.loader.exec_module(preparation)


def fixture():
    services = {}
    mounts_by_service = {
        "mysql": {
            "/var/lib/mysql": ("/opt/newapi/mysql_data", False),
            "/etc/mysql/conf.d/newapi.cnf": ("/opt/newapi/config/mysql.cnf", True),
            "/docker-entrypoint-initdb.d/99-preparation-readonly.sql":
                ("/opt/newapi/config/99-preparation-readonly.sql", True),
        },
        "redis": {
            "/data": ("/opt/newapi/redis_data", False),
            "/usr/local/etc/redis/redis.conf": ("/opt/newapi/config/redis.conf", True),
        },
    }
    for name, mounts in mounts_by_service.items():
        services[name] = {
            "image": name + "@sha256:" + "a" * 64,
            "container_name": "newapi-" + name,
            "restart": "unless-stopped",
            "networks": {"default": None},
            "volumes": [{"type": "bind", "source": source, "target": target,
                         "read_only": readonly, "bind": {"create_host_path": False}}
                        for target, (source, readonly) in mounts.items()],
            "logging": {"driver": "local", "options": {"max-size": "20m", "max-file": "5"}},
        }
    services["mysql"].update(environment={"MYSQL_DATABASE": "new-api", "MYSQL_ROOT_HOST": "localhost", "TZ": "Asia/Shanghai"},
                             env_file=[{"path": "/opt/newapi/.env.db", "required": True}])
    services["redis"]["command"] = ["redis-server", "/usr/local/etc/redis/redis.conf"]
    return {"name": "newapi", "services": services,
            "networks": {"default": {"name": "newapi_default", "internal": True, "ipam": {}}}}


class PreparationTests(unittest.TestCase):
    def reject(self, config):
        with self.assertRaises(preparation.InvalidPreparation):
            preparation.validate(config)

    def test_minimal_contract_passes_without_claiming_runtime(self):
        result = preparation.validate(fixture())
        self.assertTrue(result["configuration_valid"])
        self.assertFalse(result["runtime_verified"])

    def test_compose_2_40_3_expanded_credentials_and_omitempty_false(self):
        config = fixture()
        mysql = config["services"]["mysql"]
        del mysql["env_file"]
        mysql["environment"].update(MYSQL_USER="newapi", MYSQL_PASSWORD="PRIVATE_TEST_SENTINEL",
                                     MYSQL_ROOT_PASSWORD="PRIVATE_TEST_SENTINEL_ROOT")
        for service in config["services"].values():
            for mount in service["volumes"]:
                mount["bind"] = {}
        result = subprocess.run([sys.executable, str(HERE / "check-preparation.py"), "--stdin"],
                                input=json.dumps(config), text=True, capture_output=True, check=False)
        self.assertEqual(result.returncode, 0)
        self.assertNotIn("PRIVATE_TEST_SENTINEL", result.stdout + result.stderr)
        self.assertTrue(json.loads(result.stdout)["configuration_valid"])
        for invalid in ({"MYSQL_USER": "root"}, {"MYSQL_PASSWORD": ""}, {"MYSQL_ROOT_PASSWORD": None},
                        {"MYSQL_ALLOW_EMPTY_PASSWORD": "1"}):
            changed = copy.deepcopy(config)
            changed["services"]["mysql"]["environment"].update(invalid)
            self.reject(changed)

    def test_app_extra_and_missing_services_rejected(self):
        for name in ("new-api", "sidecar"):
            config = fixture()
            config["services"][name] = {"image": "bad"}
            self.reject(config)
        config = fixture()
        del config["services"]["mysql"]
        self.reject(config)

    def test_service_escape_hatches_rejected(self):
        unsafe = {"ports": [{"target": 3306, "published": "3306"}], "network_mode": "host",
                  "privileged": True, "pid": "host", "ipc": "host", "devices": ["/dev/sda"],
                  "cap_add": ["SYS_ADMIN"], "volumes_from": ["other"], "build": ".",
                  "provider": {"type": "custom"}, "post_start": [{"command": "bad"}],
                  "secrets": ["external"], "configs": ["external"], "use_api_socket": True}
        for key, value in unsafe.items():
            with self.subTest(key=key):
                config = fixture()
                config["services"]["mysql"][key] = value
                self.reject(config)

    def test_external_or_noninternal_network_rejected(self):
        for change in ({"internal": False}, {"external": True}, {"driver": "host"},
                       {"driver_opts": {"com.docker.network.bridge.enable_ip_masquerade": "true"}}):
            config = fixture()
            config["networks"]["default"].update(change)
            self.reject(config)
        config = fixture()
        config["services"]["mysql"]["networks"]["outside"] = None
        self.reject(config)

    def test_mutable_wrong_images_and_entrypoint_rejected(self):
        for image in ("mysql:8.0.46", "mysql:latest", "evil@sha256:" + "a" * 64, "mysql@sha256:bad"):
            config = fixture()
            config["services"]["mysql"]["image"] = image
            self.reject(config)
        for field, value in (("entrypoint", ["sh"]), ("command", ["--skip-grant-tables"])):
            config = fixture()
            config["services"]["mysql"][field] = value
            self.reject(config)

    def test_mount_changes_duplicates_and_auto_creation_rejected(self):
        changes = ({"type": "volume"}, {"source": "/"}, {"target": "/host"},
                   {"bind": {"create_host_path": True}}, {"bind": None},
                   {"bind": {"create_host_path": False, "propagation": "rshared"}}, {"read_only": True})
        for change in changes:
            config = fixture()
            config["services"]["mysql"]["volumes"][0].update(change)
            self.reject(config)
        config = fixture()
        config["services"]["mysql"]["volumes"][1] = copy.deepcopy(config["services"]["mysql"]["volumes"][0])
        self.reject(config)
        config = fixture()
        config["services"]["mysql"]["volumes"][1]["read_only"] = False
        self.reject(config)

    def test_optional_env_file_wrong_db_and_unsafe_logging_rejected(self):
        config = fixture()
        config["services"]["mysql"]["env_file"][0]["required"] = False
        self.reject(config)
        config = fixture()
        config["services"]["mysql"]["environment"]["MYSQL_DATABASE"] = "other"
        self.reject(config)
        config = fixture()
        config["services"]["mysql"]["environment"]["MYSQL_ROOT_HOST"] = "%"
        self.reject(config)
        config = fixture()
        config["services"]["mysql"]["logging"]["driver"] = "syslog"
        self.reject(config)

    def test_stdin_never_echoes_secrets_or_bad_json(self):
        config = fixture()
        config["services"]["mysql"]["environment"]["MYSQL_ROOT_PASSWORD"] = "PRIVATE_TEST_SENTINEL"
        for data in (json.dumps(config), '{"secret":"PRIVATE_TEST_SENTINEL",', '{"name":"newapi","name":"PRIVATE_TEST_SENTINEL"}'):
            result = subprocess.run([sys.executable, str(HERE / "check-preparation.py"), "--stdin"],
                                    input=data, text=True, capture_output=True, check=False)
            self.assertEqual(result.returncode, 1)
            self.assertNotIn("PRIVATE_TEST_SENTINEL", result.stdout + result.stderr)
            self.assertFalse(json.loads(result.stdout)["configuration_valid"])

    def test_compose_command_is_only_config_and_stderr_is_private(self):
        with patch.object(sys, "argv", ["check", "--compose", "/opt/newapi/docker-compose.yml"]), \
             patch.object(preparation.subprocess, "run", return_value=subprocess.CompletedProcess([], 1, b"", b"PRIVATE_TEST_SENTINEL")) as run, \
             patch("sys.stdout", new_callable=io.StringIO) as out:
            self.assertEqual(preparation.main(), 1)
        self.assertEqual(run.call_args.args[0], ["docker", "compose", "-f", "/opt/newapi/docker-compose.yml", "config", "--no-env-resolution", "--format", "json"])
        self.assertNotIn("PRIVATE_TEST_SENTINEL", out.getvalue())

    @unittest.skipUnless(os.environ.get("NEWAPI_PREPARATION_COMPOSE_TEST") == "1", "opt in to installed Compose parser")
    def test_real_template_with_dummy_private_env_and_no_containers(self):
        # v2.40.3 may read env_file despite --no-env-resolution. Use a temporary
        # copy with fixture-only values, never a repository or production .env.
        with tempfile.TemporaryDirectory(prefix="newapi-preparation-config-") as directory:
            compose = Path(directory) / "docker-compose.yml"
            shutil.copyfile(HERE / "docker-compose.prepare.yml", compose)
            env = Path(directory) / ".env.db"
            env.write_text("MYSQL_USER=newapi\nMYSQL_PASSWORD=PRIVATE_TEST_SENTINEL\n"
                           "MYSQL_ROOT_PASSWORD=PRIVATE_TEST_SENTINEL_ROOT\n")
            env.chmod(0o600)
            result = subprocess.run([sys.executable, str(HERE / "check-preparation.py"), "--compose", str(compose)],
                                    text=True, capture_output=True, check=False, timeout=40)
        self.assertEqual(result.returncode, 0, "installed Compose rejected the preparation template")
        self.assertNotIn("PRIVATE_TEST_SENTINEL", result.stdout + result.stderr)
        self.assertTrue(json.loads(result.stdout)["configuration_valid"])


if __name__ == "__main__":
    unittest.main()
