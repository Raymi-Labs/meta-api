"""Source configuration and public commands, using a PTY and isolated executables."""
import os
from pathlib import Path
import pty
import select
import shutil
import subprocess
import tempfile
import time
import unittest

SCRIPTS = Path(__file__).resolve().parents[1]
CONFIG = 'NEWAPI_IMAGE_REPOSITORY=ghcr.io/raymi-labs/meta-api\nNEWAPI_GITHUB_REPOSITORY=Raymi-Labs/meta-api\n'


class SourceConfigTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.config = self.root / 'release.conf'
        self.config.write_text(CONFIG)
        self.config.chmod(0o600)

    def invoke(self, **overrides):
        env = {k: v for k, v in os.environ.items() if not k.startswith('NEWAPI_')}
        env.update(NEWAPI_RELEASE_CONFIG=str(self.config), **overrides)
        return subprocess.run(['bash', '-c', 'source "$1"; load_source_config; printf "%s\\n" "$IMAGE_REPOSITORY" "$GITHUB_REPOSITORY"', 'test', str(SCRIPTS / 'newapi-release')], env=env, capture_output=True, text=True, timeout=5)

    def test_company_source_and_explicit_override(self):
        result = self.invoke()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.splitlines(), ['ghcr.io/raymi-labs/meta-api', 'Raymi-Labs/meta-api'])
        result = self.invoke(NEWAPI_IMAGE_REPOSITORY='ghcr.io/example/test', NEWAPI_GITHUB_REPOSITORY='example/test')
        self.assertEqual(result.stdout.splitlines(), ['ghcr.io/example/test', 'example/test'])

    def test_config_never_executes_shell_and_requires_paired_keys(self):
        marker = self.root / 'must-not-exist'
        for content in (CONFIG + 'EXTRA=x\n', CONFIG + CONFIG, CONFIG.splitlines()[0],
                        CONFIG.replace('Raymi-Labs/meta-api', f'$(touch {marker})')):
            with self.subTest(content=content):
                self.config.write_text(content)
                self.assertNotEqual(self.invoke().returncode, 0)
                self.assertFalse(marker.exists())

    def test_missing_writable_or_symlink_config_rejected(self):
        self.config.chmod(0o666)
        self.assertNotEqual(self.invoke().returncode, 0)
        self.config.unlink()
        self.assertNotEqual(self.invoke().returncode, 0)
        target = self.root / 'target'
        target.write_text(CONFIG)
        self.config.symlink_to(target)
        self.assertNotEqual(self.invoke().returncode, 0)
        self.config.unlink()
        self.config.write_text(CONFIG)
        self.root.chmod(0o777)
        self.assertNotEqual(self.invoke().returncode, 0)
        self.root.chmod(0o700)


class MenuTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.command = self.root / 'meta-api'
        shutil.copy2(SCRIPTS / 'meta-api', self.command)
        self.events = self.root / 'events'
        for name, body in {
            'id': '#!/bin/sh\necho 0\n',
            'newapi-release': '#!/bin/sh\nprintf "%s\\n" "$*" >> "$EVENTS"\nexit "${ENGINE_EXIT:-0}"\n',
        }.items():
            p = self.root / name
            p.write_text(body)
            p.chmod(0o755)
        self.env = {**os.environ, 'PATH': str(self.root) + ':' + os.environ['PATH'], 'EVENTS': str(self.events)}

    def invoke(self, *args, name='meta-api', **env):
        return subprocess.run([str(self.root / name), *args], env={**self.env, **env}, input='', capture_output=True, text=True, timeout=5)

    def test_shortcut_arguments_and_failures_preserved(self):
        for name in ('update', 'start', 'restart', 'stop', 'rollback'):
            alias = 'meta-api-' + name
            (self.root / alias).symlink_to(self.command)
            result = self.invoke('--dry-run', name=alias)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(self.events.read_text().splitlines()[-1], name + ' --dry-run')
        self.assertEqual(self.invoke('update', ENGINE_EXIT='23').returncode, 23)

    def test_help_invalid_input_and_unconfirmed_stop_do_not_call_engine(self):
        self.assertEqual(self.invoke('--help').returncode, 0)
        for args in ((), ('invalid',), ('stop',), ('stop', '--typo')):
            self.assertNotEqual(self.invoke(*args).returncode, 0)
        self.assertFalse(self.events.exists())
        self.assertEqual(self.invoke('stop', '--yes').returncode, 0)
        self.assertEqual(self.events.read_text(), 'stop --yes\n')

    def test_real_terminal_menu_cancel_confirm_and_exit(self):
        master, slave = pty.openpty()
        process = subprocess.Popen([str(self.command)], stdin=slave, stdout=slave, stderr=slave, env=self.env)
        os.close(slave)
        output = bytearray()
        try:
            # PTY input is delivered in order: invalid choice, status, cancel stop,
            # confirm stop, then exit. All actions use the same installed engine.
            os.write(master, b'9\n1\n5\nn\n5\ny\n0\n')
            deadline = time.monotonic() + 8
            while time.monotonic() < deadline:
                if select.select([master], [], [], 0.2)[0]:
                    try:
                        chunk = os.read(master, 65536)
                    except OSError:
                        break
                    if not chunk:
                        break
                    output.extend(chunk)
                if process.poll() is not None:
                    break
            self.assertEqual(process.wait(timeout=2), 0)
        finally:
            if process.poll() is None:
                process.kill()
                process.wait()
            os.close(master)
        self.assertEqual(self.events.read_text().splitlines(), ['status', 'stop --yes'])
        self.assertIn('已取消', output.decode())
        self.assertIn('请输入 0 到 6', output.decode())


if __name__ == '__main__':
    unittest.main()
