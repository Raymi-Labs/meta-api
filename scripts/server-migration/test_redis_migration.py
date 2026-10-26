"""Read-only audit regressions plus opt-in real Redis RDB/AOF migration exercise."""
import copy
import contextlib
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import time
import unittest
from unittest import mock
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

    def zset_pair(self):
        legacy = copy.deepcopy(self.expected)
        legacy["databases"]["0"]["entries"]["key"] = {
            "type": "zset", "dump_sha1": "a" * 40, "expires_at_ms": 2000}
        modern = copy.deepcopy(legacy)
        modern["format_version"] = 2
        modern["databases"]["0"]["entries"]["key"].update(
            fingerprint="zset-members-scores-v1", value_sha1="b" * 40)
        return legacy, modern

    def test_legacy_upgrade_requires_original_dump_continuity(self):
        legacy, modern = self.zset_pair()
        upgraded = audit.upgrade(legacy, modern, "c" * 64)
        self.assertTrue(upgraded["continuity"]["legacy_dump_verified"])
        self.assertEqual(upgraded["continuity"]["prior_manifest_sha256"], "c" * 64)
        modern["databases"]["0"]["entries"]["key"]["dump_sha1"] = "d" * 40
        with self.assertRaises(audit.AuditError):
            audit.compare(legacy, modern)
        with self.assertRaises(audit.AuditError):
            audit.upgrade(legacy, modern, "c" * 64)

    def test_v2_allows_only_zset_encoding_change_with_equal_semantics(self):
        _, modern = self.zset_pair()
        actual = copy.deepcopy(modern)
        actual["databases"]["0"]["entries"]["key"]["dump_sha1"] = "d" * 40
        self.assertEqual(audit.compare(modern, actual)["zset_encoding_changes"], 1)
        for field, value in (("value_sha1", "e" * 40), ("fingerprint", "redis-dump-v1"),
                             ("expires_at_ms", 2001), ("type", "string")):
            changed = copy.deepcopy(actual)
            changed["databases"]["0"]["entries"]["key"][field] = value
            with self.subTest(field=field), self.assertRaises(audit.AuditError):
                audit.compare(modern, changed)
        for change in (lambda x: x["databases"].update({"1": {}}),
                       lambda x: x["databases"]["0"]["entries"].clear(),
                       lambda x: x["databases"]["0"]["entries"].update(extra=copy.deepcopy(x["databases"]["0"]["entries"]["key"]))):
            changed = copy.deepcopy(actual)
            change(changed)
            with self.assertRaises(audit.AuditError):
                audit.compare(modern, changed)

    def test_upgrade_cli_checks_private_prior_sha_and_never_prints_values(self):
        legacy, modern = self.zset_pair()
        marker = "private-fixture-value-must-not-be-printed"
        legacy["private_marker"] = marker
        with tempfile.TemporaryDirectory() as directory:
            prior, target = Path(directory) / "v1.json", Path(directory) / "v2.json"
            raw = json.dumps(legacy).encode()
            prior.write_bytes(raw)
            prior.chmod(0o600)
            base = ["audit", "upgrade", "--container", "fixture", "--container-id", "a" * 64,
                    "--image-id", "sha256:" + "b" * 64, "--auth-env", "FIXTURE_AUTH",
                    "--prior-manifest", str(prior), "--manifest", str(target), "--prior-sha256"]
            for sha, changed_dump in (("0" * 64, False), (hashlib.sha256(raw).hexdigest(), True)):
                current = copy.deepcopy(modern)
                if changed_dump:
                    current["databases"]["0"]["entries"]["key"]["dump_sha1"] = "d" * 40
                stdout, stderr = io.StringIO(), io.StringIO()
                with mock.patch.object(audit.sys, "argv", [*base, sha]), mock.patch.object(audit, "DockerRedis") as client:
                    client.return_value.snapshot.return_value = current
                    with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
                        self.assertEqual(audit.main(), 1)
                    if not changed_dump:
                        client.assert_not_called()
                self.assertFalse(target.exists())
                self.assertNotIn(marker, stdout.getvalue() + stderr.getvalue())
                self.assertEqual(stdout.getvalue(), "")

    def test_private_manifest_fifo_is_rejected_without_blocking(self):
        with tempfile.TemporaryDirectory() as directory:
            fifo = Path(directory) / "manifest.json"
            os.mkfifo(fifo, 0o600)
            child = """
import importlib.util, sys
spec = importlib.util.spec_from_file_location('audit', sys.argv[1])
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)
try:
    audit.read_private(sys.argv[2])
except audit.AuditError:
    sys.exit(0)
sys.exit(1)
"""
            result = subprocess.run([audit.sys.executable, "-B", "-c", child, str(Path(audit.__file__)), str(fifo)],
                                    capture_output=True, timeout=3)
            self.assertEqual(result.returncode, 0)
            self.assertEqual(result.stdout + result.stderr, b"")

    def test_malformed_manifest_only_emits_fixed_private_diagnostic(self):
        with tempfile.TemporaryDirectory() as directory:
            manifest = Path(directory) / "bad.json"
            marker = "private-value-must-not-appear-in-traceback"
            manifest.write_text(json.dumps([marker]))
            manifest.chmod(0o600)
            argv = ["audit", "verify", "--container", "fixture", "--container-id", "a" * 64,
                    "--image-id", "sha256:" + "b" * 64, "--auth-env", "FIXTURE_AUTH", "--manifest", str(manifest)]
            stdout, stderr = io.StringIO(), io.StringIO()
            with mock.patch.object(audit.sys, "argv", argv), mock.patch.object(audit, "DockerRedis") as client:
                client.return_value.snapshot.return_value = copy.deepcopy(self.expected)
                with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
                    self.assertEqual(audit.main(), 1)
            self.assertEqual(stdout.getvalue(), "")
            self.assertNotIn(marker, stderr.getvalue())
            self.assertNotIn("Traceback", stderr.getvalue())
            self.assertIn("Redis migration audit failed", stderr.getvalue())


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
                             ["SET", "expires-during-migration", "short", "PX", "5000"],
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
                # Redis keeps a reduced sorted set in skiplist form, but RDB
                # loading may choose listpack. Its DUMP bytes then differ even
                # though every binary member, score and absolute TTL is equal.
                members = [f"extra-{index:03d}" for index in range(128)]
                additions = [part for index, member in enumerate(members) for part in (str(index + 10), member)]
                src.cli("ZADD", "zset", *additions)
                src.cli("ZREM", "zset", *members)
                src.cli("PEXPIRE", "zset", "180000")
                self.assertEqual(src.cli("OBJECT", "ENCODING", "zset"), "skiplist")
                sampled = src.snapshot()
                legacy = copy.deepcopy(sampled)
                legacy["format_version"] = 1
                for db in legacy["databases"].values():
                    for entry in db["entries"].values():
                        entry.pop("value_sha1")
                        entry.pop("fingerprint")
                legacy_sha = hashlib.sha256(json.dumps(legacy, sort_keys=True).encode()).hexdigest()
                # A v2 proof must be sampled from the frozen source and match
                # the old v1 bytes; it cannot be invented from the target.
                before = audit.upgrade(legacy, src.snapshot(), legacy_sha)
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
                self.assertEqual(boot.cli("OBJECT", "ENCODING", "zset"), "listpack")
                restored = boot.snapshot()
                with self.assertRaises(audit.AuditError):
                    audit.compare(legacy, restored)
                with self.assertRaises(audit.AuditError):
                    audit.upgrade(legacy, restored, legacy_sha)
                expiry = before["databases"]["0"]["entries"][hashlib.sha1(b"expires-during-migration").hexdigest()]["expires_at_ms"]
                clock = boot.cli("TIME")
                remaining = expiry - (int(clock[0]) * 1000 + int(clock[1]) // 1000)
                if remaining > 0:
                    time.sleep(remaining / 1000 + 0.1)
                first = audit.compare(before, boot.snapshot())
                self.assertEqual(first["zset_encoding_changes"], 1)
                self.assertGreaterEqual(first["naturally_expired_keys"], 1)
                # Alter score/member independently while preserving type and
                # absolute TTL, so failures specifically exercise semantics.
                boot.cli("ZADD", "zset", "2.5", "a")
                with self.assertRaises(audit.AuditError):
                    audit.compare(before, boot.snapshot())
                boot.cli("ZADD", "zset", "1.5", "a")
                boot.cli("ZADD", "zset", "1.5", "different-member")
                boot.cli("ZREM", "zset", "a")
                with self.assertRaises(audit.AuditError):
                    audit.compare(before, boot.snapshot())
                boot.cli("ZADD", "zset", "1.5", "a")
                boot.cli("ZREM", "zset", "different-member")
                audit.compare(before, boot.snapshot())
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
                                  "zset_skiplist_to_listpack_verified": True,
                                  "zset_member_and_score_tampering_rejected": True,
                                  "legacy_manifest_continuity_verified": True,
                                  "normal_restart_verified": True}))
            finally:
                for container in reversed(containers):
                    subprocess.run(["docker", "rm", "-f", "-v", container], capture_output=True, timeout=30)
                for volume in reversed(volumes):
                    subprocess.run(["docker", "volume", "rm", volume], capture_output=True, timeout=30)


if __name__ == "__main__":
    unittest.main()
