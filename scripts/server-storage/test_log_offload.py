import argparse
import contextlib
import gzip
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import stat
import subprocess
import tempfile
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location("log_offload", Path(__file__).with_name("log-offload.py"))
offload = importlib.util.module_from_spec(spec)
spec.loader.exec_module(offload)
NOW = 1_792_656_000 * 1_000_000_000


class FakeRemote:
    def __init__(self, owner):
        self.owner = owner
        self.files = {}
        self.uploads = 0
        self.verifications = 0
        self.fail_upload = False
        self.fail_verify = False
        self.after_upload = None
        self.after_verify = None

    def destination(self, name, digest):
        return "vault:newapi/" + digest + "/" + name

    def upload(self, source, destination):
        # Slow network operations must leave the deployment lock available.
        with offload.locked(self.owner.release_lock):
            pass
        self.uploads += 1
        if self.fail_upload:
            raise offload.OffloadError("upload failed")
        self.files.setdefault(destination, source.read_bytes())
        if self.after_upload:
            self.after_upload()

    def verify(self, destination, expected):
        with offload.locked(self.owner.release_lock):
            pass
        self.verifications += 1
        if self.fail_verify or destination not in self.files:
            raise offload.OffloadError("read-back failed")
        digest = offload.gzip_digest(io.BytesIO(self.files[destination]), expected["size"])
        if digest != expected["sha256"]:
            raise offload.OffloadError("hash mismatch")
        if self.after_verify:
            self.after_verify()


class OffloadTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.base = Path(self.temporary.name).resolve()
        self.root = self.base / "logs"
        self.root.mkdir(mode=0o700)
        self.args = argparse.Namespace(action="preview", root=str(self.root), state_dir=str(self.base / "state"),
                                       retention_days=30, release_lock=str(self.base / "release.lock"),
                                       remote="vault:newapi", rclone_config=str(self.base / "rclone.conf"))
        self.config = Path(self.args.rclone_config)
        self.config.write_text("[vault]\ntype = crypt\npassword = hidden-test-password\nremote = backing:path\n")
        self.config.chmod(0o600)
        self.job = offload.Offloader(self.args, NOW)
        self.remote = FakeRemote(self.job)
        self.closed = patch.object(offload, "ensure_closed")
        self.closed_mock = self.closed.start()
        self.addCleanup(self.closed.stop)
        self.addCleanup(self.temporary.cleanup)

    def source(self, name="oneapi-20260901.log.gz", age=31, contents=b"synthetic text log\n"):
        path = self.root / name
        path.write_bytes(gzip.compress(contents, mtime=0))
        os.utime(path, ns=(NOW - int(age * offload.DAY_NS), NOW - int(age * offload.DAY_NS)))
        return path

    def execute(self, action):
        self.args.action = action
        with patch.object(offload, "CryptRemote", return_value=self.remote), contextlib.redirect_stdout(io.StringIO()):
            self.job.execute()

    def receipts(self):
        return [json.loads(path.read_text()) for path in self.job.state.glob("*.json")]

    def write_receipt(self, receipt):
        with offload.locked(self.job.release_lock):
            self.job.save(receipt)

    def test_default_preview_has_no_writes_no_remote_and_strict_age_boundary(self):
        old = self.source()
        self.source("oneapi-boundary.log.gz", age=30)
        self.source("oneapi-recent.log.gz", age=29)
        self.source("unrelated.log.gz", age=90)
        self.source("oneapi-current.log", age=90)
        (self.root / "oneapi-directory.log.gz").mkdir()
        before = set(self.base.rglob("*"))
        with patch.object(offload, "CryptRemote") as remote, contextlib.redirect_stdout(io.StringIO()) as output:
            self.job.execute()
        self.assertIn("candidates=1", output.getvalue())
        self.assertIn(old.name, output.getvalue())
        self.assertEqual(set(self.base.rglob("*")), before)
        remote.assert_not_called()
        self.closed_mock.assert_not_called()
        self.assertFalse(Path(self.args.release_lock).exists())

    def test_rejects_expanded_retention_unsafe_roots_and_symlinks(self):
        self.args.retention_days = 14
        with self.assertRaisesRegex(offload.OffloadError, "at least 30"):
            offload.Offloader(self.args)
        self.args.retention_days = 30
        self.source()
        (self.root / "oneapi-linked.log.gz").symlink_to(self.root / "oneapi-20260901.log.gz")
        with self.assertRaisesRegex(offload.OffloadError, "symlink"):
            self.job.candidates()
        self.args.root = str(self.root / "..")
        with self.assertRaises(offload.OffloadError):
            offload.Offloader(self.args)
        self.args.root = str(self.root)
        self.args.state_dir = str(self.root / "state")
        with self.assertRaisesRegex(offload.OffloadError, "separate"):
            offload.Offloader(self.args)

    def test_hardlinked_or_other_writable_sources_are_not_candidates(self):
        path = self.source()
        os.link(path, self.root / "oneapi-second.log.gz")
        self.assertEqual(self.job.candidates(), [])
        writable = self.source("oneapi-writable.log.gz")
        writable.chmod(0o666)
        self.assertEqual(self.job.candidates(), [])

    def test_archive_retains_source_and_receipt_is_private_then_retire_rechecks_remote(self):
        path = self.source()
        original = path.read_bytes()
        self.execute("archive")
        self.assertEqual(path.read_bytes(), original)
        receipt = self.receipts()[0]
        self.assertEqual(receipt["status"], "verified")
        self.assertEqual(receipt["fingerprint"]["sha256"], hashlib.sha256(original).hexdigest())
        self.assertEqual(stat.S_IMODE(self.job.state.stat().st_mode), 0o700)
        self.assertEqual(stat.S_IMODE(self.job.receipt_path(receipt).stat().st_mode), 0o600)
        self.assertNotIn("password", json.dumps(receipt))
        self.execute("retire")
        self.assertEqual(self.remote.verifications, 2)
        self.assertFalse(path.exists())
        self.assertEqual(self.receipts()[0]["status"], "retired")
        self.execute("retire")
        self.assertEqual(self.remote.verifications, 2)

    def test_repeated_archive_is_idempotent_and_rechecks_existing_remote(self):
        self.source()
        self.execute("archive")
        self.execute("archive")
        self.assertEqual(len(self.receipts()), 1)
        self.assertEqual(self.remote.uploads, 1)
        self.assertEqual(self.remote.verifications, 2)

    def test_upload_and_verification_failure_keep_source_and_pending_receipt(self):
        path = self.source()
        for stage in ("fail_upload", "fail_verify"):
            setattr(self.remote, stage, True)
            with self.assertRaises(offload.OffloadError):
                self.execute("archive")
            self.assertTrue(path.exists())
            self.assertEqual(self.receipts()[0]["status"], "pending")
            setattr(self.remote, stage, False)
        self.execute("archive")
        self.assertEqual(self.receipts()[0]["status"], "verified")

    def test_corrupt_local_gzip_is_not_uploaded(self):
        path = self.source()
        path.write_bytes(b"not a gzip")
        os.utime(path, ns=(NOW - 40 * offload.DAY_NS,) * 2)
        with self.assertRaisesRegex(offload.OffloadError, "gzip"):
            self.execute("archive")
        self.assertEqual(self.remote.uploads, 0)
        self.assertTrue(path.exists())
        self.assertEqual(self.receipts(), [])

    def test_gzip_digest_checks_truncation_crc_and_concatenated_members(self):
        valid = gzip.compress(b"a") + gzip.compress(b"b")
        self.assertEqual(offload.gzip_digest(io.BytesIO(valid), len(valid)), hashlib.sha256(valid).hexdigest())
        for broken in (valid[:-1], valid[:-5] + b"wrong", b""):
            with self.assertRaises(offload.OffloadError):
                offload.gzip_digest(io.BytesIO(broken), len(broken))

    def test_source_changed_during_upload_is_not_verified(self):
        path = self.source()
        self.remote.after_upload = lambda: path.write_bytes(gzip.compress(b"changed"))
        with self.assertRaises(offload.OffloadError):
            self.execute("archive")
        self.assertTrue(path.exists())
        self.assertEqual(self.receipts()[0]["status"], "pending")

    def test_retire_requires_current_remote_not_only_old_receipt(self):
        path = self.source()
        self.execute("archive")
        self.remote.files.clear()
        with self.assertRaisesRegex(offload.OffloadError, "read-back"):
            self.execute("retire")
        self.assertTrue(path.exists())
        self.assertEqual(self.receipts()[0]["status"], "verified")

    def test_retire_rejects_inode_replacement_and_same_metadata_content_change(self):
        path = self.source()
        self.execute("archive")
        original, info = path.read_bytes(), path.stat()
        replacement = self.base / "replacement"
        replacement.write_bytes(original)
        os.utime(replacement, ns=(info.st_atime_ns, info.st_mtime_ns))
        replacement.replace(path)
        with self.assertRaisesRegex(offload.OffloadError, "fingerprint"):
            self.execute("retire")
        self.assertTrue(path.exists())
        # A second archive records this inode; mutate equal-length gzip while preserving mtime.
        self.execute("archive")
        changed = bytearray(original)
        changed[4] ^= 1  # gzip header mtime: still valid, same size, different SHA-256.
        path.write_bytes(changed)
        os.utime(path, ns=(info.st_atime_ns, info.st_mtime_ns))
        for receipt in self.receipts():
            if receipt["fingerprint"]["ino"] == info.st_ino:
                receipt["status"] = "retired"
                self.write_receipt(receipt)
        with self.assertRaisesRegex(offload.OffloadError, "fingerprint"):
            self.execute("retire")
        self.assertTrue(path.exists())

    def test_open_source_blocks_archive_and_retire(self):
        path = self.source()
        self.closed_mock.side_effect = offload.OffloadError("source file is open by a process")
        with self.assertRaisesRegex(offload.OffloadError, "open"):
            self.execute("archive")
        self.assertEqual(self.remote.uploads, 0)
        self.closed_mock.side_effect = None
        self.execute("archive")
        self.closed_mock.side_effect = offload.OffloadError("source file is open by a process")
        with self.assertRaisesRegex(offload.OffloadError, "open"):
            self.execute("retire")
        self.assertTrue(path.exists())

    def test_release_lock_and_task_lock_contention_fail_before_network(self):
        self.source()
        self.args.action = "archive"
        with offload.locked(self.job.release_lock), patch.object(offload, "CryptRemote") as remote:
            with self.assertRaisesRegex(offload.OffloadError, "holds the lock"):
                self.job.execute()
            remote.assert_not_called()
        self.job.prepare_state()
        with offload.locked(self.job.state / "task.lock", private=True), patch.object(offload, "CryptRemote") as remote:
            with self.assertRaisesRegex(offload.OffloadError, "holds the lock"):
                self.job.execute()
            remote.assert_not_called()

    def test_receipt_transitions_and_exact_unlink_hold_release_lock(self):
        source = self.source()
        original_save, original_unlink = self.job.save, os.unlink
        transitions, removed = [], []

        def assert_locked():
            with self.assertRaisesRegex(offload.OffloadError, "holds the lock"):
                with offload.locked(self.job.release_lock):
                    pass

        def save(receipt):
            assert_locked()
            transitions.append(receipt["status"])
            return original_save(receipt)

        def unlink(path, *args, **kwargs):
            if Path(path) == source:
                assert_locked()
                removed.append(path)
            return original_unlink(path, *args, **kwargs)

        with patch.object(self.job, "save", side_effect=save), patch.object(offload.os, "unlink", side_effect=unlink):
            self.execute("archive")
            self.execute("retire")
        self.assertEqual(transitions, ["pending", "verified", "retiring", "retired"])
        self.assertEqual(removed, [source])

    def test_state_permissions_and_untrusted_receipts_block_retire(self):
        path = self.source()
        self.job.state.mkdir(mode=0o755)
        with self.assertRaisesRegex(offload.OffloadError, "0700"):
            self.execute("archive")
        self.job.state.chmod(0o700)
        self.execute("archive")
        receipt = self.receipts()[0]
        receipt_path = self.job.receipt_path(receipt)
        receipt_path.chmod(0o644)
        with self.assertRaisesRegex(offload.OffloadError, "0600"):
            self.execute("retire")
        receipt_path.chmod(0o600)
        receipt["name"] = "../outside.log.gz"
        receipt_path.write_text(json.dumps(receipt))
        with self.assertRaisesRegex(offload.OffloadError, "invalid"):
            self.execute("retire")
        self.assertTrue(path.exists())

    def test_symlink_receipt_and_lock_are_rejected(self):
        self.source()
        self.job.prepare_state()
        (self.job.state / "task.lock").symlink_to(self.config)
        with self.assertRaisesRegex(offload.OffloadError, "symlink"):
            self.execute("archive")
        (self.job.state / "task.lock").unlink()
        self.execute("archive")
        receipt_path = next(self.job.state.glob("*.json"))
        receipt_path.unlink()
        receipt_path.symlink_to(self.config)
        with self.assertRaisesRegex(offload.OffloadError, "symlink"):
            self.execute("retire")

    def test_retiring_resume_handles_existing_source_and_post_unlink_interruption(self):
        path = self.source()
        self.execute("archive")
        receipt = self.receipts()[0]
        receipt["status"] = "retiring"
        self.write_receipt(receipt)
        self.execute("retire")
        self.assertFalse(path.exists())
        self.assertEqual(self.receipts()[0]["retirement_result"], "removed")
        receipt = self.receipts()[0]
        receipt["status"] = "retiring"
        self.write_receipt(receipt)
        self.execute("retire")
        self.assertEqual(self.receipts()[0]["retirement_result"], "absent_after_interruption")

    def test_crash_after_unlink_is_recoverable_but_failed_intent_write_never_deletes(self):
        path = self.source()
        self.execute("archive")
        actual_save = self.job.save

        def interrupted(receipt):
            if receipt["status"] == "retired":
                raise OSError("injected write failure")
            return actual_save(receipt)

        with patch.object(self.job, "save", side_effect=OSError("intent write failed")):
            with self.assertRaises(OSError):
                self.execute("retire")
        self.assertTrue(path.exists())
        with patch.object(self.job, "save", side_effect=interrupted):
            with self.assertRaises(OSError):
                self.execute("retire")
        self.assertFalse(path.exists())
        self.assertEqual(self.receipts()[0]["status"], "retiring")
        self.execute("retire")
        self.assertEqual(self.receipts()[0]["status"], "retired")

    def test_unexpected_missing_verified_source_is_not_marked_retired(self):
        path = self.source()
        self.execute("archive")
        path.unlink()
        with self.assertRaisesRegex(offload.OffloadError, "unexpectedly missing"):
            self.execute("retire")
        self.assertEqual(self.receipts()[0]["status"], "verified")

    def crypt(self, settings=None):
        settings = settings if settings is not None else {"type": "crypt", "password": "hidden", "remote": "backing:prefix"}
        result = subprocess.CompletedProcess([], 0, json.dumps({"vault": settings}).encode())
        with patch.object(offload.subprocess, "run", return_value=result):
            return offload.CryptRemote("vault:newapi", self.args.rclone_config)

    def test_crypt_type_and_content_encryption_are_required(self):
        for settings in ({"type": "s3"}, {"type": "crypt", "remote": "backing:"},
                         {"type": "crypt", "password": "secret"},
                         {"type": "crypt", "password": "secret", "remote": "backing:", "no_data_encryption": "true"}):
            with self.assertRaises(offload.OffloadError):
                self.crypt(settings)
        self.assertEqual(self.crypt().prefix, "vault:newapi")
        for target in (":crypt:/tmp", "vault,option=value:prefix", "vault:../escape", "vault:/absolute"):
            with self.assertRaises(offload.OffloadError):
                offload.CryptRemote(target, self.args.rclone_config)

    def test_crypt_config_is_private_and_environment_cannot_override_type(self):
        self.config.chmod(0o644)
        with self.assertRaisesRegex(offload.OffloadError, "0600"):
            self.crypt()
        self.config.chmod(0o600)
        with patch.dict(os.environ, {"RCLONE_CONFIG_VAULT_TYPE": "local", "RCLONE_LOG_FILE": "/tmp/secret"}):
            remote = self.crypt()
        self.assertFalse(any(key.startswith("RCLONE_") for key in remote.env))
        self.config.write_text("changed secret configuration")
        with self.assertRaisesRegex(offload.OffloadError, "changed"):
            remote.check_config()

    def test_rclone_failure_does_not_expose_output_or_secret(self):
        secret = "sensitive-backend-password"
        result = subprocess.CompletedProcess([], 1, secret.encode(), secret.encode())
        with patch.object(offload.subprocess, "run", return_value=result):
            with self.assertRaises(offload.OffloadError) as error:
                offload.CryptRemote("vault:newapi", self.args.rclone_config)
        self.assertNotIn(secret, str(error.exception))

    def test_streamed_decrypted_readback_checks_hash_size_gzip_and_exit_status(self):
        remote = self.crypt()
        content = gzip.compress(b"remote synthetic")
        expected = {"size": len(content), "sha256": hashlib.sha256(content).hexdigest()}
        for stream, exit_code in ((content, 0), (content, 1), (content[:-2], 0), (b"x" * len(content), 0), (content + b"extra", 0)):
            process = unittest.mock.Mock(stdout=io.BytesIO(stream))
            process.wait.return_value = exit_code
            process.poll.return_value = exit_code
            with patch.object(offload.subprocess, "Popen", return_value=process):
                if stream == content and exit_code == 0:
                    remote.verify("vault:newapi/file", expected)
                else:
                    with self.assertRaises(offload.OffloadError):
                        remote.verify("vault:newapi/file", expected)


class OpenFileInspectionTests(unittest.TestCase):
    def test_proc_inspection_detects_open_files_and_ignores_only_own_verifier_fd(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory).resolve()
            source = base / "source"
            source.write_text("test")
            proc = base / "proc"
            (proc / "self/fd").mkdir(parents=True)
            own = proc / str(os.getpid()) / "fd"
            own.mkdir(parents=True)
            (own / "123").symlink_to(source)
            with patch.object(offload, "PROC", proc):
                offload.ensure_closed(source.stat(), 123)
                with self.assertRaisesRegex(offload.OffloadError, "open"):
                    offload.ensure_closed(source.stat())
                (own / "124").symlink_to(source)
                with self.assertRaisesRegex(offload.OffloadError, "open"):
                    offload.ensure_closed(source.stat(), 123)

    def test_proc_absence_or_permission_failure_is_not_treated_as_closed(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory).resolve()
            with patch.object(offload, "PROC", base):
                with self.assertRaisesRegex(offload.OffloadError, "/proc"):
                    offload.ensure_closed(base.stat())
            (base / "self/fd").mkdir(parents=True)
            with patch.object(offload, "PROC", base), patch.object(Path, "iterdir", side_effect=PermissionError):
                with self.assertRaisesRegex(offload.OffloadError, "every process"):
                    offload.ensure_closed(base.stat())


if __name__ == "__main__":
    unittest.main()
