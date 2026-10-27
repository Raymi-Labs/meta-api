import contextlib
import io
import json
import subprocess
import unittest
from pathlib import Path
from unittest.mock import patch


class LinuxAssetTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        root = Path(__file__).resolve().parents[2]
        command = "const w = Bun.YAML.parse(await Bun.file('.github/workflows/release.yml').text()); console.log(JSON.stringify(w.jobs.linux.steps.find(s => s.name === 'Fill missing Linux assets without updating release metadata').run));"
        shell = json.loads(subprocess.check_output(['bun', '-e', command], cwd=root, text=True))
        cls.code = shell.split("python3 - <<'PY'\n", 1)[1].rsplit('\nPY', 1)[0]
        cls.version = 'v1.0.0-compat.101'
        cls.names = [f'new-api-{cls.version}', f'new-api-arm64-{cls.version}',
                     f'new-api-docs-{cls.version}.tar.gz', f'new-api-docs-{cls.version}.sha256',
                     f'meta-api-management-{cls.version}.tar.gz', f'meta-api-management-{cls.version}.sha256']

    def execute(self, assets, *, read_error=None):
        with patch.dict('os.environ', {'VERSION': self.version, 'GITHUB_REPOSITORY': 'owner/repo'}), \
             patch('subprocess.check_output', return_value=json.dumps({'assets': assets}), side_effect=read_error), \
             patch('subprocess.run') as write, contextlib.redirect_stdout(io.StringIO()):
            exec(self.code, {})
            return [call.args[0] for call in write.call_args_list]

    def test_uploads_required_assets_without_metadata_changes(self):
        self.assertEqual(self.execute([]), [
            ['gh', 'release', 'upload', self.version, name, '--repo', 'owner/repo']
            for name in self.names
        ])

    def test_partial_and_complete_retries_keep_uploaded_assets(self):
        complete = [{'name': name, 'state': 'uploaded', 'size': 100} for name in self.names]
        self.assertEqual(self.execute(complete), [])
        self.assertEqual(self.execute(complete[:1]), [
            ['gh', 'release', 'upload', self.version, name, '--repo', 'owner/repo']
            for name in self.names[1:]
        ])

    def test_incomplete_asset_is_not_overwritten(self):
        for name in self.names:
            for state, size in [('starter', 100), ('uploaded', 0)]:
                with self.subTest(name=name, state=state, size=size), self.assertRaises(RuntimeError):
                    self.execute([{'name': name, 'state': state, 'size': size}])

    def test_read_failure_does_not_create_or_edit_release(self):
        with self.assertRaises(subprocess.CalledProcessError):
            self.execute([], read_error=subprocess.CalledProcessError(1, ['gh']))


if __name__ == '__main__':
    unittest.main()
