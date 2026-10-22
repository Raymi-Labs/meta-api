"""Opt-in Linux integration: local-only crypt backend, no network or real secrets."""
import gzip
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import sys
import tempfile
import time
import unittest


ENABLED = os.environ.get("NEWAPI_RCLONE_INTEGRATION") == "1" and sys.platform == "linux" and shutil.which("rclone")
SCRIPT = Path(__file__).with_name("log-offload.py")


@unittest.skipUnless(ENABLED, "requires Linux, rclone, and NEWAPI_RCLONE_INTEGRATION=1")
class RcloneIntegrationTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="newapi-log-offload-test-")
        self.base = Path(self.temporary.name).resolve()
        self.addCleanup(self.temporary.cleanup)
        self.logs = self.base / "logs"
        self.logs.mkdir(mode=0o700)
        self.objects = self.base / "objects"
        self.objects.mkdir(mode=0o700)
        self.config = self.base / "rclone.conf"
        # Only randomly generated throwaway test credentials are used.
        password = secrets.token_urlsafe(32)
        obscured = subprocess.run(["rclone", "obscure", password], check=True, stdout=subprocess.PIPE,
                                  stderr=subprocess.DEVNULL, text=True, timeout=15).stdout.strip()
        self.config.write_text("[backing]\ntype = local\n\n[vault]\ntype = crypt\n"
                               f"remote = backing:{self.objects}\npassword = {obscured}\n")
        self.config.chmod(0o600)
        self.source = self.logs / "oneapi-synthetic-old.log.gz"
        self.plaintext = b"synthetic-recovery-marker: no real customer data\n" * 64
        self.compressed = gzip.compress(self.plaintext, mtime=0)
        self.source.write_bytes(self.compressed)
        old = time.time_ns() - 31 * 86400 * 1_000_000_000
        os.utime(self.source, ns=(old, old))

    def command(self, action):
        return [sys.executable, "-B", str(SCRIPT), action, "--root", str(self.logs),
                "--state-dir", str(self.base / "state"), "--release-lock", str(self.base / "release.lock"),
                "--remote", "vault:synthetic", "--rclone-config", str(self.config)]

    def execute(self, action, success=True):
        result = subprocess.run(self.command(action), stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                text=True, timeout=90)
        if success:
            self.assertEqual(result.returncode, 0, result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0)
        return result

    def receipt(self):
        return json.loads(next((self.base / "state").glob("*.json")).read_text())

    def test_actual_crypt_archive_and_decrypt_retirement(self):
        self.execute("preview")
        self.assertFalse((self.base / "state").exists())
        self.execute("archive")
        self.assertEqual(self.source.read_bytes(), self.compressed)
        receipt = self.receipt()
        self.assertEqual(receipt["status"], "verified")
        encrypted_files = [path for path in self.objects.rglob("*") if path.is_file()]
        self.assertEqual(len(encrypted_files), 1)
        encrypted = encrypted_files[0].read_bytes()
        self.assertTrue(encrypted.startswith(b"RCLONE\x00\x00"))
        self.assertNotIn(self.compressed, encrypted)
        self.assertNotIn(b"synthetic-recovery-marker", encrypted)
        self.assertNotIn(self.source.name, str(encrypted_files[0]))
        self.execute("archive")  # Idempotent: verifies the same encrypted object.
        self.execute("retire")
        self.assertFalse(self.source.exists())
        self.assertEqual(self.receipt()["status"], "retired")
        restored = subprocess.run(["rclone", "--config", str(self.config), "cat", receipt["remote"]],
                                  check=True, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=30).stdout
        self.assertEqual(restored, self.compressed)
        self.assertEqual(gzip.decompress(restored), self.plaintext)
        self.execute("retire")

    def test_real_open_descriptor_blocks_archive(self):
        holder = subprocess.Popen([sys.executable, "-c",
                                   "import sys; f=open(sys.argv[1],'rb'); print('ready',flush=True); sys.stdin.read()",
                                   str(self.source)], stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True)
        try:
            self.assertEqual(holder.stdout.readline().strip(), "ready")
            result = self.execute("archive", success=False)
            self.assertIn("open by a process", result.stderr)
            self.assertTrue(self.source.exists())
            self.assertFalse(any(path.is_file() for path in self.objects.rglob("*")))
        finally:
            holder.stdin.close()
            holder.stdout.close()
            holder.wait(timeout=10)

    def test_corrupt_encrypted_object_blocks_retirement(self):
        self.execute("archive")
        encrypted = next(path for path in self.objects.rglob("*") if path.is_file())
        content = bytearray(encrypted.read_bytes())
        content[-1] ^= 1
        encrypted.write_bytes(content)
        self.execute("retire", success=False)
        self.assertTrue(self.source.exists())
        self.assertEqual(self.receipt()["status"], "verified")


if __name__ == "__main__":
    unittest.main()
