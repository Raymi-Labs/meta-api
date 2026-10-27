"""Execute the release tool's container shell with isolated MySQL clients."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

RELEASE = Path(__file__).resolve().parents[1] / 'newapi-release'


class MySQLCredentialsTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='meta-api-mysql-credentials-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.app = self.root / 'app'
        self.app.mkdir()
        (self.app / 'docker-compose.yml').write_text('services: {}\n')
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        self.events = self.root / 'events.jsonl'
        self.secret = 'fixture-$password-"with spaces"'
        self.env = {**os.environ, 'PATH': str(self.bin) + os.pathsep + os.environ['PATH'],
                    'NEWAPI_APP_DIR': str(self.app), 'NEWAPI_MYSQL_CONTAINER': 'fixture-mysql',
                    'EVENTS': str(self.events), 'FIXTURE_PASSWORD': self.secret}
        self.script('docker', '''
import json, os, subprocess, sys
args = sys.argv[1:]
assert args.pop(0) == 'exec'
env = dict(os.environ, MYSQL_ROOT_PASSWORD=os.environ['FIXTURE_PASSWORD'], MYSQL_DATABASE='fixture_db')
while args and args[0].startswith('-'):
    flag = args.pop(0)
    if flag == '-i': continue
    assert flag == '-e'
    key, value = args.pop(0).split('=', 1)
    env[key] = value
assert args.pop(0) == 'fixture-mysql'
assert os.environ['FIXTURE_PASSWORD'] not in str(sys.argv)
sys.exit(subprocess.run(args, env=env).returncode)
''')
        client = '''
import json, os, sys
from pathlib import Path
args = sys.argv[1:]
assert os.environ.get('MYSQL_PWD') == os.environ['FIXTURE_PASSWORD']
assert os.environ['FIXTURE_PASSWORD'] not in str(args)
assert not any(a.startswith('-p') or a.startswith('--password') for a in args)
assert '--protocol=SOCKET' in args
kind = Path(sys.argv[0]).name
with open(os.environ['EVENTS'], 'a') as out:
    out.write(json.dumps({'kind': kind, 'args': args}) + '\\n')
if kind == 'mysqldump':
    if os.environ.get('FAIL_DUMP') == '1':
        print('fixture dump permission denied', file=sys.stderr)
        sys.exit(27)
    print('CREATE TABLE fixture (id BIGINT);')
elif '-Nse' in args:
    print('stable schema fingerprint input')
elif '-e' not in args:
    assert 'CREATE TABLE fixture' in sys.stdin.read()
'''
        self.script('mysql', client)
        self.script('mysqldump', client)

    def script(self, name, body):
        path = self.bin / name
        path.write_text(f'#!{sys.executable}\n' + body)
        path.chmod(0o755)

    def run_shell(self, body, **env):
        result = subprocess.run(['bash', '-c', 'source "$1"; ' + body, 'test', str(RELEASE)],
                                env={**self.env, **env}, capture_output=True, text=True, timeout=15)
        self.assertNotIn(self.secret, result.stdout + result.stderr)
        return result

    def test_backup_uses_container_environment_without_password_arguments(self):
        result = self.run_shell('available_kb() { echo 99999999; }; create_database_backup old new')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        events = [json.loads(line) for line in self.events.read_text().splitlines()]
        self.assertEqual([e['kind'] for e in events], ['mysqldump'])
        self.assertIn('--single-transaction', events[0]['args'])
        self.assertIn('--routines', events[0]['args'])

    def test_dump_error_is_not_suppressed_or_marked_complete(self):
        result = self.run_shell('available_kb() { echo 99999999; }; create_database_backup old new', FAIL_DUMP='1')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('fixture dump permission denied', result.stderr)
        backup, = (self.app / 'backups/one-click').iterdir()
        self.assertFalse((backup / 'DATABASE_SHA256SUMS').exists())
        self.assertFalse((backup / 'mysql.sql.gz').exists())

    def test_schema_clone_fingerprint_and_cleanup_use_same_credentials(self):
        result = self.run_shell('run_preflight_pass() { PREFLIGHT_CONTAINER=""; }; preflight_target')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        events = [json.loads(line) for line in self.events.read_text().splitlines()]
        self.assertEqual(sum(e['kind'] == 'mysqldump' for e in events), 1)
        self.assertTrue(any('--no-data' in e['args'] for e in events))
        self.assertEqual(sum('-Nse' in e['args'] for e in events), 2)
        self.assertTrue(any('CREATE DATABASE' in str(e['args']) for e in events))
        self.assertTrue(any('DROP DATABASE' in str(e['args']) for e in events))


if __name__ == '__main__':
    unittest.main()
