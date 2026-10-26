"""Read-only audit regressions plus opt-in real Redis RDB/AOF migration exercise."""
import copy
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import time
import unittest
import uuid

SPEC = importlib.util.spec_from_file_location("redis_migration_audit", Path(__file__).with_name("redis-migration-audit.py"))
audit = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(audit)


class AuditTests(unittest.TestCase):
    def setUp(self):
        self.expected = {"format_version": 1, "redis_version": "7.4.9", "databases": {
            "0": {"observed_at_ms": 1000, "entries": {"key": {"type": "string", "dump_sha1": "value", "expires_at_ms": 2000}}}}}

    def test_identical_expiry_and_value_pass(self):
        actual = copy.deepcopy(self.expected)
        actual["databases"]["0"]["observed_at_ms"] = 1500
        self.assertEqual(audit.compare(self.expected, actual)["retained_keys"], 1)

    def test_natural_expiry_is_allowed_but_live_key_loss_is_not(self):
        actual = copy.deepcopy(self.expected)
        actual["databases"]["0"]["entries"] = {}
        with self.assertRaises(audit.AuditError):
            audit.compare(self.expected, actual)
        actual["databases"]["0"]["observed_at_ms"] = 2000
        self.assertEqual(audit.compare(self.expected, actual)["naturally_expired_keys"], 1)

    def test_expiry_extension_value_type_changes_and_extra_keys_fail(self):
        for field, value in (("expires_at_ms", 3000), ("expires_at_ms", -1), ("dump_sha1", "other"), ("type", "hash")):
            actual = copy.deepcopy(self.expected)
            actual["databases"]["0"]["entries"]["key"][field] = value
            with self.subTest(field=field), self.assertRaises(audit.AuditError):
                audit.compare(self.expected, actual)
        actual = copy.deepcopy(self.expected)
        actual["databases"]["0"]["entries"]["extra"] = actual["databases"]["0"]["entries"]["key"]
        with self.assertRaises(audit.AuditError):
            audit.compare(self.expected, actual)

    def test_database_or_clock_drift_fails(self):
        for modify in (lambda value: value["databases"].update({"1": {}}),
                       lambda value: value["databases"]["0"].update(observed_at_ms=-1000)):
            actual = copy.deepcopy(self.expected)
            modify(actual)
            with self.assertRaises(audit.AuditError):
                audit.compare(self.expected, actual)


@unittest.skipUnless(os.environ.get("NEWAPI_REDIS_MIGRATION_IMAGE"), "opt-in local Docker exercise")
class RealRedisMigrationTests(unittest.TestCase):
    def test_rdb_bootstrap_then_aof_survives_restart_with_original_absolute_ttls(self):
        image = os.environ["NEWAPI_REDIS_MIGRATION_IMAGE"]
        self.assertRegex(image, r"^redis@sha256:[a-f0-9]{64}$")
        prefix = "newapi-redis-migration-" + uuid.uuid4().hex[:12]
        source, wrong, bootstrap, normal = [prefix + suffix for suffix in ("-source", "-wrong", "-bootstrap", "-normal")]
        volumes = [prefix + suffix for suffix in ("-source-data", "-old-empty-data", "-fresh-data")]
        containers = []
        password, target_password = secrets.token_hex(24), secrets.token_hex(24)

        def docker(*arguments, data=None, env=None, timeout=60):
            result = subprocess.run(["docker", *arguments], input=data, capture_output=True, env=env, timeout=timeout)
            # Never put command arguments or private stderr into a test failure.
            if result.returncode:
                self.fail("private Docker fixture command failed")
            return result.stdout.decode()

        def client(container, source_mode=False):
            identity = json.loads(docker("inspect", container))[0]
            return audit.DockerRedis(container, identity["Id"], identity["Image"],
                                     auth_command=source_mode, auth_config=None if source_mode else "/etc/fixture-redis.conf")

        def ready(instance):
            deadline = time.monotonic() + 20
            while time.monotonic() < deadline:
                try:
                    if instance.cli("PING") == "PONG":
                        return
                except audit.AuditError:
                    pass
                time.sleep(0.1)
            self.fail("private Redis fixture did not become ready")

        def persistence(instance):
            text = instance.cli("INFO", "persistence")
            return dict(line.split(":", 1) for line in text.splitlines() if ":" in line)

        def wait_aof(instance):
            deadline = time.monotonic() + 30
            while time.monotonic() < deadline:
                state = persistence(instance)
                if (state.get("aof_enabled") == "1" and state.get("aof_rewrite_in_progress") == "0"
                        and state.get("aof_rewrite_scheduled") == "0" and state.get("aof_last_bgrewrite_status") == "ok"
                        and state.get("aof_last_write_status") == "ok" and int(state.get("aof_current_size", 0)) > 0):
                    return
                time.sleep(0.1)
            self.fail("AOF rewrite did not finish successfully")

        docker("image", "inspect", image)  # Never pull implicitly.
        with tempfile.TemporaryDirectory(prefix=prefix) as directory:
            root = Path(directory)
            config = root / "redis.conf"
            config.write_text(f"dir /data\nappendonly yes\nappendfsync everysec\nsave \"\"\nrequirepass {target_password}\n")
            config.chmod(0o644)  # Synthetic credential; enclosing directory is private.

            def create_target(name, volume, override=False):
                containers.append(name)
                docker("create", "--pull=never", "--name", name, "--network", "none", "--restart=no",
                       "--memory=256m", "--mount", f"type=volume,src={volume},dst=/data", image,
                       "redis-server", "/etc/fixture-redis.conf", *(["--appendonly", "no"] if override else []))
                docker("cp", str(config), name + ":/etc/fixture-redis.conf")

            try:
                for volume in volumes:
                    docker("volume", "create", volume)
                containers.append(source)
                docker("run", "-d", "--pull=never", "--name", source, "--network", "none", "--restart=no",
                       "--memory=256m", "--mount", f"type=volume,src={volumes[0]},dst=/data", image,
                       "redis-server", "--dir", "/data", "--appendonly", "yes", "--appendfsync", "everysec",
                       "--save", "", "--requirepass", password)
                src = client(source, True)
                ready(src)
                # Synthetic keys include binary strings and all common Redis
                # value types, multiple DBs, security nonces and affinity state.
                commands = [["SET", f"fixture:{index}", "x" * 1024] for index in range(2153)]
                commands += [["SET", "newapi:pulse:nonce:fixture", "1", "PX", "120000"],
                             ["SET", "affinity:fixture", "channel-123", "PX", "180000"],
                             ["SET", "expires-during-migration", "short", "PX", "1000"],
                             ["HSET", "hash", "field", "value"], ["RPUSH", "list", "a", "b"],
                             ["SADD", "set", "a", "b"], ["ZADD", "zset", "1.5", "a"],
                             ["XADD", "stream", "*", "field", "value"],
                             ["SELECT", "1"], ["SET", b"binary\x00key", b"\x00\xffbinary\nvalue"], ["SELECT", "0"]]
                protocol = bytearray()
                for command in commands:
                    values = [value if isinstance(value, bytes) else str(value).encode() for value in command]
                    protocol.extend(f"*{len(values)}\r\n".encode())
                    for value in values:
                        protocol.extend(f"${len(value)}\r\n".encode() + value + b"\r\n")
                docker("exec", "-i", "-e", "REDISCLI_AUTH", source, "redis-cli", "--pipe", data=bytes(protocol),
                       env={**os.environ, "REDISCLI_AUTH": password})
                before = src.snapshot()
                self.assertEqual(sum(len(db["entries"]) for db in before["databases"].values()), 2162)
                # This is the final stopped-writer export; no keys are written
                # from this point onward, only natural TTL expiration occurs.
                docker("exec", "-e", "REDISCLI_AUTH", source, "redis-cli", "--rdb", "/data/final.rdb",
                       env={**os.environ, "REDISCLI_AUTH": password})
                rdb = root / "final.rdb"
                docker("cp", source + ":/data/final.rdb", str(rdb))
                rdb.chmod(0o600)
                self.assertGreater(rdb.stat().st_size, 1000)
                source_sha256 = docker("exec", source, "sha256sum", "/data/final.rdb").split()[0]
                self.assertEqual(hashlib.sha256(rdb.read_bytes()).hexdigest(), source_sha256)
                # Existing empty AOF takes precedence: placing dump.rdb beside
                # it must NOT be accepted as a restored target.
                create_target(wrong, volumes[1])
                docker("start", wrong)
                bad = client(wrong)
                ready(bad)
                bad.cli("SET", "old-empty-directory-marker", "1")
                bad.cli("FLUSHALL")
                time.sleep(1.1)
                docker("stop", "--time", "10", wrong)
                docker("cp", str(rdb), wrong + ":/data/dump.rdb")
                docker("start", wrong)
                ready(bad)
                self.assertEqual(bad.cli("DBSIZE"), 0)
                with self.assertRaises(audit.AuditError):
                    audit.compare(before, bad.snapshot())
                # Correct path: a fresh target directory, permanent AOF=yes,
                # temporary process-only override to read RDB first.
                create_target(bootstrap, volumes[2], override=True)
                docker("cp", str(rdb), bootstrap + ":/data/dump.rdb")
                docker("run", "--rm", "--pull=never", "--network", "none", "--entrypoint", "redis-check-rdb",
                       "--mount", f"type=volume,src={volumes[2]},dst=/data,readonly", image, "/data/dump.rdb")
                docker("start", bootstrap)
                boot = client(bootstrap)
                ready(boot)
                first = audit.compare(before, boot.snapshot())
                self.assertGreaterEqual(first["naturally_expired_keys"], 1)
                self.assertEqual(persistence(boot)["aof_enabled"], "0")
                boot.cli("CONFIG", "SET", "appendonly", "yes")
                wait_aof(boot)
                # Abrupt termination AFTER the rewrite gate must still load the
                # verified data from AOF. This does not imply arbitrary crash
                # safety before the rewrite has completed.
                docker("kill", bootstrap)
                create_target(normal, volumes[2])
                docker("start", normal)
                dst = client(normal)
                ready(dst)
                final = audit.compare(before, dst.snapshot())
                self.assertEqual(persistence(dst)["aof_enabled"], "1")
                self.assertEqual(dst.cli("CONFIG", "GET", "appendfsync"), {"appendfsync": "everysec"})
                # Verify another normal stop/start retains the same absolute
                # TTLs, nonce, affinity and binary/multitype contents.
                docker("stop", "--time", "10", normal)
                docker("start", normal)
                ready(dst)
                audit.compare(before, dst.snapshot())
                print(json.dumps({"redis": "7.4.9", "initial_keys": 2162, **final,
                                  "existing_empty_aof_false_restore_rejected": True,
                                  "export_sha256_and_rdb_checksum_verified": True,
                                  "rdb_bootstrap_to_aof_and_crash_restart_verified": True,
                                  "normal_restart_verified": True}))
            finally:
                for container in reversed(containers):
                    subprocess.run(["docker", "rm", "-f", "-v", container], capture_output=True, timeout=30)
                for volume in reversed(volumes):
                    subprocess.run(["docker", "volume", "rm", volume], capture_output=True, timeout=30)


if __name__ == "__main__":
    unittest.main()
