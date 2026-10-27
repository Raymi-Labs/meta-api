import importlib.util
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import unittest
from unittest import mock

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('directory_migration', HERE / 'migrate-deployment-directory.py')
migration = importlib.util.module_from_spec(spec)
spec.loader.exec_module(migration)


class DeploymentDirectoryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.old, self.new = self.root / 'newapi', self.root / 'meta-api'
        self.old.mkdir(mode=0o700)
        (self.old / 'data').write_bytes(b'same data')

    def select(self):
        return subprocess.run(['bash', '-c', 'source "$1"; select_app_dir "$2" "$3"', 'test',
                               str(HERE.parent / 'newapi-release'), str(self.new), str(self.old)],
                              text=True, capture_output=True, timeout=5)

    def test_legacy_new_and_same_directory_selection(self):
        self.assertEqual(self.select().stdout.strip(), str(self.old))
        self.old.rename(self.new)
        self.assertEqual(self.select().stdout.strip(), str(self.new))
        self.old.symlink_to(self.new)
        self.assertEqual(self.select().stdout.strip(), str(self.new))
        self.old.unlink()
        self.old.mkdir()
        result = self.select()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('independent deployment directories', result.stderr)

    def test_failed_exchange_removes_only_its_temporary_link(self):
        with mock.patch.object(migration, 'exchange', side_effect=OSError('not supported')):
            with self.assertRaises(OSError):
                migration.relocate(self.old, self.new)
        self.assertFalse(self.new.exists() or self.new.is_symlink())
        self.assertEqual((self.old / 'data').read_bytes(), b'same data')

    def test_existing_target_and_source_alias_are_refused(self):
        self.new.mkdir()
        with self.assertRaises(ValueError):
            migration.relocate(self.old, self.new)
        self.new.rmdir()
        alias = self.root / 'alias'
        alias.symlink_to(self.old)
        with self.assertRaises(ValueError):
            migration.relocate(alias, self.new)

    @unittest.skipUnless(sys.platform == 'linux', 'renameat2 requires Linux; run in CI/isolated server fixture')
    def test_atomic_exchange_preserves_inode_open_handle_and_old_path(self):
        before = (self.old / 'data').stat()
        errors, samples = [], []
        stop = threading.Event()
        def reader():
            while not stop.is_set():
                try:
                    samples.append((self.old / 'data').read_bytes())
                except Exception as error:
                    errors.append(error)
        worker = threading.Thread(target=reader)
        worker.start()
        try:
            with (self.old / 'data').open('rb') as stream:
                migration.relocate(self.old, self.new)
                self.assertEqual(stream.read(), b'same data')
            self.assertTrue(self.old.is_symlink())
            self.assertEqual(os.readlink(self.old), str(self.new))
            after = (self.new / 'data').stat()
            self.assertEqual((before.st_dev, before.st_ino), (after.st_dev, after.st_ino))
            self.assertEqual(self.select().stdout.strip(), str(self.new))
        finally:
            stop.set()
            worker.join()
        self.assertTrue(samples)
        self.assertFalse(errors)
        self.assertEqual(set(samples), {b'same data'})


if __name__ == '__main__':
    unittest.main()
