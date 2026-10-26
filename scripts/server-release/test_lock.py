"""Exercise the real release flock without Docker or privileged host paths."""
import fcntl
import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
import unittest

RELEASE = Path(__file__).resolve().parents[1] / "newapi-release"


class ReleaseLockTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="newapi-lock-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.bin = self.root / "bin"
        self.bin.mkdir()
        flock = self.bin / "flock"
        flock.write_text(f"#!{sys.executable}\nimport fcntl,sys\n"
                         "fcntl.flock(int(sys.argv[-1]),fcntl.LOCK_EX|fcntl.LOCK_NB)\n")
        flock.chmod(0o755)
        self.env = {**os.environ, "PATH": str(self.bin) + os.pathsep + os.environ["PATH"]}

    def acquire(self, path):
        return subprocess.run(["bash", "-c", 'source "$1"; acquire_lock', "lock-test", str(RELEASE)],
                              env={**self.env, "NEWAPI_LOCK_FILE": str(path)},
                              capture_output=True, text=True, timeout=5)

    def assert_success(self, result):
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def assert_rejected(self, result):
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("release lock", result.stderr)

    def test_existing_public_run_directory_keeps_mode_owner_and_inode(self):
        parent = self.root / "run"
        parent.mkdir(mode=0o755)
        parent.chmod(0o755)
        before = parent.stat()
        self.assert_success(self.acquire(parent / "newapi-release.lock"))
        after = parent.stat()
        self.assertEqual((after.st_mode, after.st_uid, after.st_gid, after.st_ino),
                         (before.st_mode, before.st_uid, before.st_gid, before.st_ino))
        self.assertEqual(stat.S_IMODE((parent / "newapi-release.lock").stat().st_mode), 0o600)

    def test_missing_private_parents_created_without_chmod_existing_ancestor(self):
        self.root.chmod(0o755)
        lock = self.root / "private" / "nested" / "release.lock"
        self.assert_success(self.acquire(lock))
        self.assertEqual(stat.S_IMODE(self.root.stat().st_mode), 0o755)
        self.assertEqual(stat.S_IMODE(lock.parent.stat().st_mode), 0o700)
        self.assertEqual(stat.S_IMODE(lock.parent.parent.stat().st_mode), 0o700)

    def test_existing_lock_contents_mode_and_inode_are_preserved(self):
        lock = self.root / "release.lock"
        lock.write_bytes(b"existing lock metadata\n")
        lock.chmod(0o644)
        before = lock.stat()
        self.assert_success(self.acquire(lock))
        self.assertEqual(lock.read_bytes(), b"existing lock metadata\n")
        self.assertEqual((lock.stat().st_ino, lock.stat().st_mode), (before.st_ino, before.st_mode))

    def test_contended_lock_preserves_contents_and_refuses(self):
        lock = self.root / "release.lock"
        lock.write_bytes(b"existing lock metadata\n")
        with lock.open("a") as held:
            fcntl.flock(held.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
            result = self.acquire(lock)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("another new-api deployment", result.stderr)
        self.assertEqual(lock.read_bytes(), b"existing lock metadata\n")

    def test_parent_symlink_is_rejected_without_touching_target(self):
        target = self.root / "target"
        target.mkdir(mode=0o755)
        parent = self.root / "alias"
        parent.symlink_to(target, target_is_directory=True)
        self.assert_rejected(self.acquire(parent / "release.lock"))
        self.assertEqual(stat.S_IMODE(target.stat().st_mode), 0o755)
        self.assertFalse((target / "release.lock").exists())

    def test_lock_symlink_and_dangling_symlink_are_rejected(self):
        target = self.root / "target"
        target.write_bytes(b"must survive\n")
        lock = self.root / "release.lock"
        lock.symlink_to(target)
        self.assert_rejected(self.acquire(lock))
        self.assertEqual(target.read_bytes(), b"must survive\n")
        target.unlink()
        self.assert_rejected(self.acquire(lock))
        self.assertFalse(target.exists())

    def test_hardlink_is_rejected_without_touching_target(self):
        target = self.root / "target"
        target.write_bytes(b"must survive\n")
        lock = self.root / "release.lock"
        os.link(target, lock)
        self.assert_rejected(self.acquire(lock))
        self.assertEqual(target.read_bytes(), b"must survive\n")

    def test_special_file_is_rejected_without_blocking(self):
        lock = self.root / "release.lock"
        os.mkfifo(lock)
        self.assert_rejected(self.acquire(lock))
        self.assertTrue(stat.S_ISFIFO(lock.lstat().st_mode))

    def test_writable_parent_and_lock_are_rejected_without_chmod(self):
        parent = self.root / "shared"
        parent.mkdir(mode=0o777)
        parent.chmod(0o777)
        self.assert_rejected(self.acquire(parent / "release.lock"))
        self.assertEqual(stat.S_IMODE(parent.stat().st_mode), 0o777)
        self.assertFalse((parent / "release.lock").exists())
        lock = self.root / "release.lock"
        lock.touch(mode=0o666)
        lock.chmod(0o666)
        self.assert_rejected(self.acquire(lock))
        self.assertEqual(stat.S_IMODE(lock.stat().st_mode), 0o666)

    def test_unsafe_ancestor_is_rejected_before_creating_children(self):
        parent = self.root / "shared"
        parent.mkdir(mode=0o777)
        parent.chmod(0o777)
        self.assert_rejected(self.acquire(parent / "missing" / "release.lock"))
        self.assertFalse((parent / "missing").exists())

    def test_relative_and_parent_traversal_paths_are_rejected(self):
        self.assert_rejected(self.acquire("relative-release.lock"))
        self.assert_rejected(self.acquire(self.root / ".." / "release.lock"))


if __name__ == "__main__":
    unittest.main()
