"""Offline recovery-package checks against the installed, single-file entrypoint."""
import gzip
import hashlib
import os
from pathlib import Path
import shutil
import stat
import subprocess
import sys
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
RELEASE = HERE.parent / "newapi-release"
# Fault injection executes the exact embedded verifier without adding production flags.
MANIFEST_PYTHON = RELEASE.read_text().split('  python3 - "$@" <<\'PY\'\n', 1)[1].split('\nPY\n', 1)[0]


class BackupManifestTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="newapi-backup-manifest-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.backup = self.root / "backup"
        self.backup.mkdir()
        files = {
            "docker-compose.yml": b"services: {}\n",
            ".env": b"EXAMPLE_SECRET=fixture-only\n",
            "release-state-before.env": b"CURRENT_IMAGE=old-image\n",
            "release-state-after.env": b"CURRENT_IMAGE=new-image\n",
            "recovery-metadata.env": b"FORMAT_VERSION=1\nSOURCE_IMAGE=old-image\nTARGET_IMAGE=new-image\n",
            "deploy-success.log": b"late deployment output\n",
            ".metadata/.hidden file": b"nested hidden fixture\n",
        }
        for relative, content in files.items():
            path = self.backup / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(content)
        dump = gzip.compress(b"CREATE TABLE example (id BIGINT);\n")
        (self.backup / "mysql.sql.gz").write_bytes(dump)
        (self.backup / "DATABASE_SHA256SUMS").write_text(hashlib.sha256(dump).hexdigest() + "  mysql.sql.gz\n")

    def run_manifest(self, operation="verify", directory=None):
        directory = directory or self.backup
        if operation == "verify":
            command = ["bash", str(RELEASE), "verify-backup", str(directory)]
        else:
            command = ["bash", "-c", 'source "$1"; backup_manifest seal "$2"', "test", str(RELEASE), str(directory)]
        return subprocess.run(command, capture_output=True, text=True, timeout=15)

    def seal(self):
        result = self.run_manifest("seal")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def assert_rejected(self):
        result = self.run_manifest()
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("ERROR: recovery package", result.stderr)

    def inject_fault(self, code, operation="seal"):
        program = code + "\nexec(" + repr(MANIFEST_PYTHON) + ")\n"
        return subprocess.run([sys.executable, "-c", program, operation, str(self.backup)],
                              text=True, capture_output=True, timeout=15)

    def test_seal_covers_hidden_recursive_and_late_files(self):
        self.seal()
        manifest = self.backup / "SHA256SUMS"
        text = manifest.read_text()
        for name in [".env", ".metadata/.hidden file", "release-state-before.env", "deploy-success.log"]:
            self.assertIn("  " + name + "\n", text)
        self.assertNotIn(str(self.backup), text)
        self.assertNotIn("  SHA256SUMS\n", text)
        self.assertEqual(stat.S_IMODE(manifest.stat().st_mode), 0o600)
        result = self.run_manifest()
        self.assertEqual(result.returncode, 0, result.stderr)
        if shutil.which("sha256sum"):
            result = subprocess.run(["sha256sum", "-c", "SHA256SUMS"], cwd=self.backup,
                                    capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)

    def test_moved_package_verifies_without_root_docker_or_app_directory(self):
        self.seal()
        destination = self.root / "restored package"
        shutil.move(str(self.backup), destination)
        env = {**os.environ, "NEWAPI_APP_DIR": "/nonexistent/newapi-test"}
        binaries = self.root / "bin"
        binaries.mkdir()
        for name in ["docker", "id"]:
            executable = binaries / name
            executable.write_text("#!/bin/sh\nexit 99\n")
            executable.chmod(0o755)
        env["PATH"] = str(binaries) + os.pathsep + os.environ["PATH"]
        result = subprocess.run(["bash", str(RELEASE), "verify-backup", str(destination)],
                                env=env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_missing_completion_marker_is_rejected(self):
        self.assert_rejected()

    def test_added_file_is_rejected(self):
        self.seal()
        (self.backup / "late-file.log").write_text("not covered")
        self.assert_rejected()

    def test_deleted_hidden_file_is_rejected(self):
        self.seal()
        (self.backup / ".env").unlink()
        self.assert_rejected()

    def test_modified_dump_is_rejected(self):
        self.seal()
        (self.backup / "mysql.sql.gz").write_bytes(gzip.compress(b"changed SQL"))
        self.assert_rejected()

    def test_incomplete_required_files_cannot_be_sealed(self):
        (self.backup / "deploy-success.log").unlink()
        result = self.run_manifest("seal")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.backup / "SHA256SUMS").exists())

    def test_unsealed_package_with_links_or_special_files_is_rejected(self):
        unsafe = self.backup / "unsafe"
        for kind in ["symlink", "directory-symlink", "hardlink", "fifo"]:
            with self.subTest(kind=kind):
                if kind == "symlink":
                    unsafe.symlink_to(self.backup / ".env")
                elif kind == "directory-symlink":
                    unsafe.symlink_to(self.backup / ".metadata", target_is_directory=True)
                elif kind == "hardlink":
                    os.link(self.backup / ".env", unsafe)
                else:
                    os.mkfifo(unsafe)
                result = self.run_manifest("seal")
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertFalse((self.backup / "SHA256SUMS").exists())
                unsafe.unlink()

    def test_replaced_file_and_root_symlink_are_rejected(self):
        self.seal()
        original = (self.backup / ".env").read_bytes()
        outside = self.root / "external"
        outside.write_bytes(original)
        (self.backup / ".env").unlink()
        (self.backup / ".env").symlink_to(outside)
        self.assert_rejected()
        link = self.root / "linked-backup"
        link.symlink_to(self.backup, target_is_directory=True)
        self.assertNotEqual(self.run_manifest(directory=link).returncode, 0)
        self.assertNotEqual(self.run_manifest(directory=str(link) + "/").returncode, 0)

    def test_manifest_paths_duplicates_and_self_reference_are_rejected(self):
        self.seal()
        manifest = self.backup / "SHA256SUMS"
        original = manifest.read_text()
        digest = "0" * 64
        for extra in ["../external", "/absolute", "a/../b", "./mysql.sql.gz", "SHA256SUMS", "mysql.sql.gz"]:
            with self.subTest(path=extra):
                manifest.write_text(original + digest + "  " + extra + "\n")
                self.assert_rejected()
        manifest.write_text(original)

    def test_unsafe_filename_cannot_create_ambiguous_checksum_record(self):
        for name in ["newline\nname", "back\\slash", "separator\u2028name"]:
            with self.subTest(name=name):
                path = self.backup / name
                path.write_text("unsafe")
                result = self.run_manifest("seal")
                self.assertNotEqual(result.returncode, 0)
                path.unlink()

    def test_cannot_silently_reseal_an_altered_package(self):
        self.seal()
        before = (self.backup / "SHA256SUMS").read_bytes()
        (self.backup / ".env").write_text("altered")
        result = self.run_manifest("seal")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((self.backup / "SHA256SUMS").read_bytes(), before)

    def test_atomic_publish_failure_leaves_no_complete_manifest(self):
        result = self.inject_fault("import os\ndef fail(*args, **kwargs):\n    raise OSError('simulated rename failure')\nos.rename = fail")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("simulated rename failure", result.stderr)
        self.assertFalse((self.backup / "SHA256SUMS").exists())
        self.assertEqual(list(self.backup.glob(".SHA256SUMS-*.tmp")), [])
        self.assertTrue((self.backup / "mysql.sql.gz").exists())

    def test_process_interruption_before_publish_leaves_no_complete_marker(self):
        result = self.inject_fault("import os,signal\ndef stop(*args, **kwargs):\n    os.kill(os.getpid(), signal.SIGTERM)\nos.rename = stop")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.backup / "SHA256SUMS").exists())
        self.assertTrue((self.backup / "mysql.sql.gz").exists())

    def test_file_changed_during_read_is_rejected(self):
        result = self.inject_fault(
            "import hashlib,sys\nfrom pathlib import Path\n"
            "real_sha256 = hashlib.sha256\n"
            "class MutatingHash:\n"
            "    def __init__(self): self.value = real_sha256()\n"
            "    def update(self, block):\n"
            "        self.value.update(block)\n"
            "        if block.startswith(b'EXAMPLE_SECRET='):\n"
            "            with (Path(sys.argv[2]) / '.env').open('ab') as output: output.write(b'changed')\n"
            "    def hexdigest(self): return self.value.hexdigest()\n"
            "hashlib.sha256 = MutatingHash"
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("changed while reading", result.stderr)
        self.assertFalse((self.backup / "SHA256SUMS").exists())

    def test_database_dump_stage_never_advertises_complete_package(self):
        app = self.root / "app"
        app.mkdir()
        (app / "docker-compose.yml").write_text("services: {}\n")
        (app / ".env").write_text("EXAMPLE_SECRET=fixture-only\n")
        result = subprocess.run(
            ["bash", "-c", 'source "$1"; available_kb() { echo 99999999; }; '
             'docker() { printf "CREATE TABLE example (id BIGINT);\\n"; }; '
             'create_database_backup old new', "test", str(RELEASE)],
            env={**os.environ, "NEWAPI_APP_DIR": str(app)}, capture_output=True, text=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        backup, = (app / "backups/one-click").iterdir()
        self.assertTrue((backup / "DATABASE_SHA256SUMS").exists())
        self.assertTrue((backup / ".env").exists())
        self.assertFalse((backup / "SHA256SUMS").exists())
        self.assertIn("  mysql.sql.gz\n", (backup / "DATABASE_SHA256SUMS").read_text())

    def prepared_app(self):
        app = Path(tempfile.mkdtemp(prefix="app-", dir=self.root))
        (app / "docker-compose.yml").write_text("services: {}\n")
        (app / "config").mkdir()
        values = {
            ".env": b"LEGACY_SECRET=fixture-only\n",
            ".env.db": b"MYSQL_ROOT_PASSWORD=DB_PRIVATE_FIXTURE_SENTINEL\n",
            ".env.app": b"CRYPTO_SECRET=APP_PRIVATE_FIXTURE_SENTINEL\n",
            "config/mysql.cnf": b"[mysqld]\nserver-id=14713539\n",
            "config/redis.conf": b"requirepass REDIS_PRIVATE_FIXTURE_SENTINEL\n",
            "config/99-preparation-readonly.sql": b"SET PERSIST_ONLY super_read_only=ON;\n",
        }
        for name, value in values.items():
            (app / name).write_bytes(value)
        return app, values

    def run_database_backup(self, app, *, compose=None):
        env = {**os.environ, "NEWAPI_APP_DIR": str(app)}
        if compose is not None:
            env["NEWAPI_COMPOSE_FILE"] = str(compose)
        result = subprocess.run(
            ["bash", "-c", 'source "$1"; available_kb() { echo 99999999; }; '
             'docker() { touch "$NEWAPI_APP_DIR/dump-started"; printf "CREATE TABLE example (id BIGINT);\\n"; }; '
             'create_database_backup old new', "test", str(RELEASE)],
            env=env, capture_output=True, text=True, timeout=15,
        )
        for secret in ("DB_PRIVATE_FIXTURE_SENTINEL", "APP_PRIVATE_FIXTURE_SENTINEL", "REDIS_PRIVATE_FIXTURE_SENTINEL"):
            self.assertNotIn(secret, result.stdout + result.stderr)
        return result

    def test_target_recovery_captures_fixed_env_and_config_files_and_seals_them(self):
        app, values = self.prepared_app()
        (app / "config/unrelated-private-file").write_text("must not recursively copy config")
        (app / "mysql_data").mkdir()
        (app / "mysql_data/do-not-copy").write_text("live database files")
        result = self.run_database_backup(app)
        self.assertEqual(result.returncode, 0, result.stderr)
        backup, = (app / "backups/one-click").iterdir()
        for name, expected in values.items():
            self.assertEqual((backup / name).read_bytes(), expected, name)
            self.assertEqual(stat.S_IMODE((backup / name).stat().st_mode), 0o600, name)
        self.assertEqual(stat.S_IMODE((backup / "config").stat().st_mode), 0o700)
        self.assertFalse((backup / "config/unrelated-private-file").exists())
        self.assertFalse((backup / "mysql_data").exists())
        self.assertFalse((backup / "SHA256SUMS").exists())
        for name in ("release-state-after.env", "recovery-metadata.env", "deploy-success.log"):
            (backup / name).write_text("fixture completion metadata\n")
        result = self.run_manifest("seal", backup)
        self.assertEqual(result.returncode, 0, result.stderr)
        manifest = (backup / "SHA256SUMS").read_text()
        for name in values:
            self.assertIn("  " + name + "\n", manifest)
        self.assertEqual(self.run_manifest(directory=backup).returncode, 0)
        (backup / ".env.app").write_text("changed crypto material")
        self.assertNotEqual(self.run_manifest(directory=backup).returncode, 0)

    def test_missing_optional_target_config_keeps_legacy_backup_supported(self):
        app, _ = self.prepared_app()
        for name in (".env.db", ".env.app"):
            (app / name).unlink()
        shutil.rmtree(app / "config")
        result = self.run_database_backup(app)
        self.assertEqual(result.returncode, 0, result.stderr)
        backup, = (app / "backups/one-click").iterdir()
        self.assertTrue((backup / ".env").is_file())
        self.assertFalse((backup / ".env.db").exists())
        self.assertFalse((backup / "config").exists())

    def test_source_config_symlinks_hardlinks_and_special_files_fail_before_dump(self):
        for name, kind in ((".env.db", "symlink"), (".env.app", "dangling"),
                           ("config/mysql.cnf", "symlink"), ("config/redis.conf", "hardlink"),
                           ("config/99-preparation-readonly.sql", "fifo"),
                           ("docker-compose.yml", "symlink")):
            with self.subTest(name=name, kind=kind):
                app, _ = self.prepared_app()
                unsafe = app / name
                original = unsafe.read_bytes()
                unsafe.unlink()
                external = app / "outside-fixture"
                if kind == "dangling":
                    unsafe.symlink_to(external)
                elif kind == "fifo":
                    os.mkfifo(unsafe)
                else:
                    external.write_bytes(original)
                    if kind == "hardlink":
                        os.link(external, unsafe)
                    else:
                        unsafe.symlink_to(external)
                result = self.run_database_backup(app)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertFalse((app / "dump-started").exists())

    def test_source_config_directory_symlink_is_not_followed(self):
        app, _ = self.prepared_app()
        external = app / "outside-config"
        (app / "config").rename(external)
        (app / "config").symlink_to(external, target_is_directory=True)
        result = self.run_database_backup(app)
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertFalse((app / "dump-started").exists())

    def test_explicit_compose_file_is_copied_under_canonical_package_name(self):
        app, _ = self.prepared_app()
        alternate = app / "docker-compose.target.yml"
        alternate.write_text("services: {target: {image: fixture}}\n")
        result = self.run_database_backup(app, compose=alternate)
        self.assertEqual(result.returncode, 0, result.stderr)
        backup, = (app / "backups/one-click").iterdir()
        self.assertEqual((backup / "docker-compose.yml").read_bytes(), alternate.read_bytes())
        self.assertFalse((backup / alternate.name).exists())


if __name__ == "__main__":
    unittest.main()
