import importlib.util
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
import zipfile

BASE = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('builder', BASE / 'build.py')
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)


class WorkflowTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='workflow test ')
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        archive = builder.build(self.root / 'dist')
        with zipfile.ZipFile(archive) as z:
            self.assertFalse(any('/notes/' in n or '__pycache__' in n for n in z.namelist()))
            z.extractall(self.root / 'unpacked')
        self.package = self.root / 'unpacked/project-decision-kit'
        self.target = self.root / 'my project'
        self.target.mkdir()

    def run_cli(self, *args, ok=True, installed=False):
        script = self.target / '.agents/workflow.py' if installed else self.package / 'workflow.py'
        result = subprocess.run([sys.executable, str(script), *map(str, args)], capture_output=True, text=True, encoding="utf-8", cwd=self.root)
        self.assertEqual(result.returncode == 0, ok, result.stdout + result.stderr)
        return result

    def snapshot(self):
        return {p.relative_to(self.target).as_posix(): p.read_bytes() for p in self.target.rglob('*') if p.is_file()}

    def test_preview_idempotence_and_preservation(self):
        (self.target / 'AGENTS.md').write_bytes(b'# Existing rules\r\nKeep me.\r\n')
        (self.target / 'docs').mkdir()
        (self.target / 'docs/FEATURE_MAP.md').write_text('existing map')
        before = self.snapshot()
        self.run_cli('install', self.target)
        self.assertEqual(before, self.snapshot())
        self.run_cli('install', self.target, '--apply', '--ci')
        self.assertTrue((self.target / 'AGENTS.md').read_bytes().startswith(before['AGENTS.md']))
        self.assertEqual((self.target / 'docs/FEATURE_MAP.md').read_text(), 'existing map')
        first = self.snapshot()
        self.run_cli('install', self.target, '--apply', '--ci')
        self.assertEqual(first, self.snapshot())
        self.run_cli('install', self.target, '--apply')
        self.assertEqual(first, self.snapshot())

    def test_conflict_is_preflight_and_preserves_all_files(self):
        conflict = self.target / '.agents/skills/write-notes-like-deepseek/SKILL.md'
        conflict.parent.mkdir(parents=True)
        conflict.write_text('custom')
        before = self.snapshot()
        self.run_cli('install', self.target, '--apply', ok=False)
        self.assertEqual(before, self.snapshot())

    def test_upgrade_and_customization_protection(self):
        self.run_cli('install', self.target, '--apply')
        source = self.package / 'payload/write-notes-like-deepseek/SKILL.md'
        source.write_text(source.read_text(encoding='utf-8') + '\nNew version.\n', encoding='utf-8')
        (self.package / 'VERSION').write_text('1.0.1\n')
        self.run_cli('install', self.target, '--apply')
        dest = self.target / '.agents/skills/write-notes-like-deepseek/SKILL.md'
        self.assertEqual(dest.read_bytes(), source.read_bytes())
        dest.write_text('local changes')
        before = self.snapshot()
        self.run_cli('install', self.target, '--apply', ok=False)
        self.assertEqual(before, self.snapshot())

    def test_modified_rule_block_rejected(self):
        self.run_cli('install', self.target, '--apply')
        p = self.target / 'AGENTS.md'
        p.write_text(p.read_text(encoding='utf-8').replace('## 能力核对与决策笔记', '## Local customization'), encoding='utf-8')
        before = self.snapshot()
        self.run_cli('install', self.target, '--apply', ok=False)
        self.assertEqual(before, self.snapshot())

    def test_symlink_refused(self):
        outside = self.root / 'outside'
        outside.mkdir()
        try:
            (self.target / '.agents').symlink_to(outside, target_is_directory=True)
        except OSError:
            self.skipTest('OS does not permit symlink creation')
        self.run_cli('install', self.target, '--apply', ok=False)
        self.assertEqual(list(outside.iterdir()), [])

    @unittest.skipUnless(shutil.which('bun'), 'Bun required for runtime validation')
    def test_standalone_verifier_and_failure_propagation(self):
        self.run_cli('install', self.target, '--apply')
        shutil.rmtree(self.package)
        self.run_cli('doctor', self.target, installed=True)
        self.run_cli('verify', self.target, installed=True)
        bad = self.target / '.agents/notes/implemented/feature/2026-10-15-bad.md'
        bad.parent.mkdir(parents=True)
        bad.write_text('# Invalid note\n')
        self.run_cli('verify', self.target, installed=True, ok=False)

    def test_build_reproducible(self):
        first = builder.build(self.root / 'one').read_bytes()
        second = builder.build(self.root / 'two').read_bytes()
        self.assertEqual(first, second)


if __name__ == '__main__':
    unittest.main()
