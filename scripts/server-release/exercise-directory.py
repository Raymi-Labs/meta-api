#!/usr/bin/env python3
"""Opt-in directory migration test with an isolated real MySQL bind mount (Linux)."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import shutil
import sys
import tempfile
import time
from types import SimpleNamespace
import uuid


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--execute', action='store_true')
    parser.add_argument('--fixture-image', default='mysql:8.0.46', help='already cached image; never pulled')
    args = parser.parse_args()
    if not args.execute or os.geteuid() != 0 or sys.platform != 'linux':
        parser.error('requires Linux root and explicit --execute')
    scripts = Path(__file__).resolve().parents[1]
    migration = module('directory_migration_fixture', Path(__file__).with_name('migrate-deployment-directory.py'))
    def docker(*args, check=True):
        return subprocess.run(['docker', *args], capture_output=True, text=True, check=check, timeout=90)
    image = docker('image', 'inspect', '-f', '{{.Id}}', args.fixture_image).stdout.strip()
    container = None
    with tempfile.TemporaryDirectory(prefix='meta-api-directory-e2e-') as temp:
        root = Path(temp)
        # The helper loader requires administrator-owned siblings. Copy the
        # reviewed fixture code into this root-owned directory, as CI checkouts
        # belong to the unprivileged runner; do not weaken the production loader.
        code = root / 'code'
        code.mkdir(mode=0o700)
        for name in ('mysql-binlog-archive.py', 'mysql-r2-backup.py', 'pitr_binlog.py', 'pitr_anchor.py'):
            shutil.copyfile(scripts / 'server-storage' / name, code / name)
            (code / name).chmod(0o600)
        sys.path.insert(0, str(code))
        archive = module('directory_archive_fixture', code / 'mysql-binlog-archive.py')
        old, new = root / 'newapi', root / 'meta-api'
        old.mkdir(mode=0o700)
        (old / 'mysql_data').mkdir(mode=0o700)
        try:
            container = docker('run', '-d', '--pull=never', '--name', 'meta-api-directory-e2e-' + uuid.uuid4().hex[:12],
                               '--label', 'meta-api.directory-fixture=true', '--network=none', '--restart=no',
                               '--memory=768m', '--pids-limit=256', '-e', 'MYSQL_ROOT_PASSWORD=fixture-only',
                               '-v', str(old / 'mysql_data') + ':/var/lib/mysql', image,
                               '--server-id=199', '--log-bin=binlog', '--binlog-format=ROW',
                               '--binlog-row-image=FULL', '--sync-binlog=1').stdout.strip()
            def sql(query, check=True):
                return docker('exec', '-e', 'MYSQL_PWD=fixture-only', container, 'mysql', '--user=root',
                              '--protocol=SOCKET', '--batch', '--skip-column-names', '-e', query, check=check)
            def ready():
                for _ in range(90):
                    if sql('SELECT 1', check=False).returncode == 0:
                        return
                    time.sleep(1)
                raise RuntimeError('isolated MySQL did not become ready')
            ready()
            sql('CREATE DATABASE pitr_fixture; CREATE TABLE pitr_fixture.probe (id BIGINT PRIMARY KEY); INSERT INTO pitr_fixture.probe VALUES (1)')
            before = json.loads(docker('inspect', '-f', '{"id":{{json .Id}},"started":{{json .State.StartedAt}},"image":{{json .Image}}}', container).stdout)
            inode = (old / 'mysql_data').stat().st_ino
            source_uuid = sql('SELECT @@server_uuid').stdout.strip()
            migration.relocate(old, new)
            assert (new / 'mysql_data').stat().st_ino == inode
            sql('INSERT INTO pitr_fixture.probe VALUES (2)')
            assert sql('SELECT COUNT(*) FROM pitr_fixture.probe').stdout.strip() == '2'
            after = json.loads(docker('inspect', '-f', '{"id":{{json .Id}},"started":{{json .State.StartedAt}},"image":{{json .Image}}}', container).stdout)
            assert after == before
            archive.LEGACY_APP_DIR, archive.META_APP_DIR = old, new
            class Processes:
                def capture(self, command):
                    return subprocess.check_output(command, timeout=10)
            for configured in (old / 'mysql_data', new / 'mysql_data'):
                source = archive.Source(SimpleNamespace(datadir=str(configured), container=container, database='pitr_fixture'), Processes())
                sample = source.sample()
                assert sample['uuid'] == source_uuid
                assert source.read(sample['file'], 0, 4) == b'\xfebin'
            docker('restart', '--timeout', '10', container)
            ready()
            assert sql('SELECT COUNT(*) FROM pitr_fixture.probe').stdout.strip() == '2'
            assert sql('SELECT @@server_uuid').stdout.strip() == source_uuid
            assert (new / 'mysql_data').stat().st_ino == inode
            print('PASS: real MySQL writes stay online through exchange; old/new binlog Source verified; same data survives isolated restart')
        finally:
            if container is not None:
                docker('rm', '-f', container)


if __name__ == '__main__':
    main()
