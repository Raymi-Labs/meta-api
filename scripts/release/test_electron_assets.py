import contextlib
import hashlib
import io
import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


@contextlib.contextmanager
def working_directory(path):
    previous = Path.cwd()
    try:
        os.chdir(path)
        yield
    finally:
        os.chdir(previous)


class ElectronAssetTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        root = Path(__file__).resolve().parents[2]
        command = "const w = Bun.YAML.parse(await Bun.file('.github/workflows/electron-build.yml').text()); console.log(JSON.stringify(w.jobs.release.steps.find(s => s.name === 'Add desktop assets to existing Linux release').run));"
        shell = json.loads(subprocess.check_output(['bun', '-e', command], cwd=root, text=True))
        cls.code = shell.split("python3 - <<'PYUPLOAD'\n", 1)[1].rsplit('\nPYUPLOAD', 1)[0]
        cls.tag = 'v1.12.8'
        cls.names = ['New-API-App Setup 1.12.8.exe', 'New-API-App 1.12.8.exe', 'New-API-App-1.12.8-windows.sha256']

    def release(self, assets=(), draft=False, linux=True):
        required = [f'new-api-{self.tag}', f'new-api-arm64-{self.tag}'] if linux else []
        return json.dumps({'isDraft': draft, 'assets': list(assets) + [
            {'name': name, 'state': 'uploaded', 'size': 100} for name in required
        ]})

    def execute(self, responses=None, *, existing=(), missing=False, corrupt=False,
                binary=False, invalid_manifest=None):
        self.commands = []
        with tempfile.TemporaryDirectory() as tmp, working_directory(tmp), \
                patch.dict(os.environ, {'RELEASE_TAG': self.tag, 'GITHUB_REPOSITORY': 'owner/repo'}), \
                patch('subprocess.check_output', side_effect=responses, return_value=self.release(existing)), \
                patch('subprocess.run', side_effect=lambda args, **_: self.commands.append(args)), \
                patch('time.sleep'), contextlib.redirect_stdout(io.StringIO()):
            directory = Path('windows-build')
            directory.mkdir()
            paths = [directory / name for name in self.names]
            for path in paths[:2]:
                path.write_bytes(b'synthetic Windows executable fixture')
            marker, newline = ('*', '\r\n') if binary else (' ', '\n')
            paths[2].write_text(''.join(
                f'{hashlib.sha256(p.read_bytes()).hexdigest()} {marker}./{p.name}{newline}'
                for p in (paths[:2] if binary else sorted(paths[:2]))
            ))
            if invalid_manifest is not None:
                lines = paths[2].read_text().splitlines()
                paths[2].write_text(invalid_manifest(lines))
            if missing:
                paths[0].unlink()
            if corrupt:
                paths[0].write_bytes(b'corrupt')
            exec(self.code, {})
        return self.commands

    def test_uploads_only_desktop_assets_without_release_mutation(self):
        self.assertEqual(self.execute(), [
            ['gh', 'release', 'upload', self.tag, f'windows-build/{name}', '--repo', 'owner/repo'] for name in self.names
        ])

    def test_waits_for_published_complete_linux_release(self):
        self.assertEqual(len(self.execute([
            subprocess.CalledProcessError(1, ['gh']), self.release(draft=True),
            self.release(linux=False), self.release(),
        ])), 3)

    def test_windows_binary_checksums_accept_crlf_and_either_file_order(self):
        self.assertEqual(len(self.execute(binary=True)), 3)

    def test_invalid_checksum_records_stop_before_upload(self):
        for manifest in [lambda lines: lines[0],
                         lambda lines: '\n'.join(lines + [lines[0]]),
                         lambda lines: '\n'.join(lines).replace('./New-API-App', './unexpected'),
                         lambda lines: '\n'.join(lines).replace('  ./', ' ?./')]:
            with self.subTest(manifest=manifest), self.assertRaises(RuntimeError):
                self.execute(invalid_manifest=manifest)
            self.assertEqual(self.commands, [])

    def test_missing_or_corrupt_local_asset_stops_before_upload(self):
        for options in [{'missing': True}, {'corrupt': True}]:
            with self.subTest(options=options), self.assertRaises(RuntimeError):
                self.execute(**options)
            self.assertEqual(self.commands, [])

    def test_complete_existing_assets_preserved_on_retry(self):
        digest = 'sha256:' + hashlib.sha256(b'synthetic Windows executable fixture').hexdigest()
        existing = [{'name': name, 'state': 'uploaded', 'size': 100, 'digest': digest} for name in self.names]
        self.assertEqual(self.execute(existing=existing), [])
        self.assertEqual(len(self.execute(existing=existing[:1])), 2)

    def test_partial_upload_from_different_build_cannot_mix_checksums(self):
        for digest in [None, 'sha256:' + '0' * 64]:
            with self.subTest(digest=digest), self.assertRaises(RuntimeError):
                self.execute(existing=[{'name': self.names[0], 'state': 'uploaded',
                                        'size': 100, 'digest': digest}])
            self.assertEqual(self.commands, [])

    def test_every_collision_validated_before_any_upload(self):
        for state, size in [('starter', 100), ('uploaded', 0)]:
            existing = [{'name': self.names[-1], 'state': state, 'size': size}]
            with self.subTest(state=state, size=size), self.assertRaises(RuntimeError):
                self.execute(existing=existing)
            self.assertEqual(self.commands, [])
        asset = {'name': self.names[-1], 'state': 'uploaded', 'size': 100}
        with self.assertRaises(RuntimeError):
            self.execute(existing=[asset, asset])
        self.assertEqual(self.commands, [])

    def test_unavailable_linux_release_times_out_without_writes(self):
        for response in [self.release(linux=False), subprocess.TimeoutExpired(['gh'], 30)]:
            with self.subTest(response=response), self.assertRaises(RuntimeError):
                self.execute([response] * 60)
            self.assertEqual(self.commands, [])


if __name__ == '__main__':
    unittest.main()
