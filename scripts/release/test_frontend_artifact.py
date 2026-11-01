import importlib.util
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('frontend', Path(__file__).with_name('frontend-artifact.py'))
frontend = importlib.util.module_from_spec(spec)
spec.loader.exec_module(frontend)


class FrontendArtifactTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / 'VERSION').write_text('v1.14.2\n')
        for name in frontend.ROOTS:
            (self.root / name).mkdir(parents=True)
            (self.root / name / 'index.html').write_text('original:' + name)
        self.artifact = self.root / 'artifact'
        self.mock = patch.object(frontend.subprocess, 'check_output', return_value='a' * 40 + '\n')
        self.mock.start()
        self.addCleanup(self.mock.stop)

    def pack(self):
        frontend.pack(self.root, self.artifact, 'v1.14.2')

    def test_same_archive_restores_all_consumers_and_removes_stale_output(self):
        self.pack()
        for _ in range(4):
            (self.root / 'web/dist/stale.js').write_text('old build')
            (self.root / 'web/dist/index.html').write_text('changed')
            frontend.restore(self.root, self.artifact, 'v1.14.2')
            self.assertEqual((self.root / 'web/dist/index.html').read_text(), 'original:web/dist')
            self.assertFalse((self.root / 'web/dist/stale.js').exists())

    def test_identity_checksum_and_missing_entry_fail_before_replacing_outputs(self):
        self.pack()
        meta_path = self.artifact / 'frontend.json'
        good = json.loads(meta_path.read_text())
        for field, bad in [('commit', 'b' * 40), ('version', 'v1.14.1'), ('sha256', 'bad'), ('schema', 2)]:
            meta_path.write_text(json.dumps({**good, field: bad}))
            with self.subTest(field=field), self.assertRaises(ValueError):
                frontend.restore(self.root, self.artifact, 'v1.14.2')
            self.assertEqual((self.root / 'web/dist/index.html').read_text(), 'original:web/dist')
        with self.assertRaises(ValueError):
            frontend.restore(self.root, self.artifact, 'v9.0.0')

    def test_traversal_links_duplicate_paths_and_unexpected_roots_fail_closed(self):
        self.pack()
        for name, kind in [('../escape', tarfile.REGTYPE), ('/escape', tarfile.REGTYPE),
                           ('web/dist/../../escape', tarfile.REGTYPE), ('web/dist/link', tarfile.SYMTYPE),
                           ('web/dist/link', tarfile.LNKTYPE), ('web/other', tarfile.REGTYPE),
                           ('web/dist/./duplicate', tarfile.REGTYPE), ('web/dist/pipe', tarfile.FIFOTYPE),
                           ('web\\dist\\file', tarfile.REGTYPE)]:
            archive = self.artifact / 'frontend.tar.gz'
            with tarfile.open(archive, 'w:gz') as tar:
                entry = tarfile.TarInfo(name)
                entry.type = kind
                entry.linkname = '../../escape'
                tar.addfile(entry, io.BytesIO())
            meta = json.loads((self.artifact / 'frontend.json').read_text())
            meta['sha256'] = frontend.digest(archive)
            (self.artifact / 'frontend.json').write_text(json.dumps(meta))
            with self.subTest(name=name, kind=kind), self.assertRaises(ValueError):
                frontend.restore(self.root, self.artifact, 'v1.14.2')
            self.assertEqual((self.root / 'web/dist/index.html').read_text(), 'original:web/dist')

    def test_incomplete_archive_does_not_remove_previous_frontend(self):
        self.pack()
        archive = self.artifact / 'frontend.tar.gz'
        with tarfile.open(archive, 'w:gz') as tar:
            tar.add(self.root / 'web/dist', arcname='web/dist')
        meta_path = self.artifact / 'frontend.json'
        meta = json.loads(meta_path.read_text()); meta['sha256'] = frontend.digest(archive)
        meta_path.write_text(json.dumps(meta))
        with self.assertRaises(ValueError):
            frontend.restore(self.root, self.artifact, 'v1.14.2')
        self.assertTrue((self.root / 'web/docs-dist/index.html').exists())


if __name__ == '__main__':
    unittest.main()
