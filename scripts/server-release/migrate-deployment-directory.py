#!/usr/bin/env python3
"""Move the audited existing deployment without restarting its containers (Linux)."""
import argparse
import ctypes
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import tempfile
import urllib.request

OLD = Path('/opt/newapi')
NEW = Path('/opt/meta-api')
CONTAINERS = ['new-api-master', 'newapi-mysql', 'newapi-redis']


def exchange(left, right):
    libc = ctypes.CDLL(None, use_errno=True)
    call = libc.renameat2
    call.argtypes = [ctypes.c_int, ctypes.c_char_p, ctypes.c_int, ctypes.c_char_p, ctypes.c_uint]
    call.restype = ctypes.c_int
    if call(-100, os.fsencode(left), -100, os.fsencode(right), 2):
        error = ctypes.get_errno()
        raise OSError(error, os.strerror(error))


def relocate(old, new):
    """Both entries share one protected parent; exchange has no missing-path gap."""
    if (not old.is_absolute() or old.parent != new.parent or old.is_symlink()
            or not old.is_dir() or os.path.ismount(old) or new.exists() or new.is_symlink()):
        raise ValueError('requires a real source directory and an unused sibling target')
    original = old.stat()
    new.symlink_to(new)
    try:
        exchange(old, new)
    except BaseException:
        # Remove only the self-link created by this invocation, never data.
        if new.is_symlink() and os.readlink(new) == str(new):
            new.unlink()
        raise
    result = new.stat()
    if ((original.st_dev, original.st_ino) != (result.st_dev, result.st_ino)
            or not old.is_symlink() or os.readlink(old) != str(new) or not old.samefile(new)):
        raise RuntimeError('directory exchange result is unconfirmed; inspect both paths')
    parent = os.open(new.parent, os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(parent)
    finally:
        os.close(parent)


def safe_root_directory(path):
    for item in [path, *path.parents]:
        info = item.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
            raise ValueError('unsafe deployment or parent directory')


def acquire_lock(path):
    safe_root_directory(path.parent)
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_NOFOLLOW | os.O_NONBLOCK, 0o600)
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1 or info.st_uid != 0 or info.st_mode & 0o022:
            raise ValueError('unsafe release lock')
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BaseException:
        os.close(fd)
        raise
    return fd


def capture(*args):
    return subprocess.check_output(args, text=True, timeout=30)


def containers():
    template = ('{"id":{{json .Id}},"image":{{json .Image}},"state":{{json .State.Status}},'
                '"started":{{json .State.StartedAt}},"restarts":{{json .RestartCount}},'
                '"project":{{json (index .Config.Labels "com.docker.compose.project")}}}')
    values = [json.loads(capture('docker', 'inspect', '-f', template, name)) for name in CONTAINERS]
    if any(v['state'] != 'running' or v['project'] != 'newapi' for v in values):
        raise ValueError('requires the existing healthy newapi Compose deployment')
    return dict(zip(CONTAINERS, values))


def fingerprint(root):
    result = {}
    for name in ('', 'mysql_data', 'redis_data', 'newapi_data', 'newapi_logs', 'backups', '.release-manager'):
        p = root / name
        info = p.lstat()
        if not stat.S_ISDIR(info.st_mode):
            raise ValueError('deployment data must be real directories')
        result[name] = [info.st_dev, info.st_ino, info.st_uid, stat.S_IMODE(info.st_mode)]
    for name in ('docker-compose.yml', '.release-manager/state.env'):
        result[name] = hashlib.sha256((root / name).read_bytes()).hexdigest()
    return result


def health():
    with urllib.request.urlopen('http://127.0.0.1:3000/api/status', timeout=5) as response:
        value = json.load(response)
    if value.get('success') is not True:
        raise ValueError('application health check failed')
    return value['data']['version']


def verify_tools():
    scripts = Path(__file__).resolve().parents[1]
    for source, installed in [
        (scripts / 'newapi-release', Path('/usr/local/bin/newapi-release')),
        (scripts / 'server-storage/mysql-binlog-archive.py', Path('/usr/local/lib/newapi-storage/mysql-binlog-archive.py')),
    ]:
        info = installed.lstat()
        if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
            raise ValueError('installed compatibility tool is unsafe')
        if source.read_bytes() != installed.read_bytes():
            raise ValueError('install this revision of release and archive tools before directory migration')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--execute', action='store_true', help='perform the audited directory exchange; default is preview')
    args = parser.parse_args()
    if os.geteuid() != 0:
        parser.error('run as root')
    safe_root_directory(OLD.parent)
    if OLD.is_symlink() and os.readlink(OLD) == str(NEW) and NEW.is_dir() and not NEW.is_symlink():
        print('Already migrated: /opt/meta-api; /opt/newapi is the compatibility link')
        return
    safe_root_directory(OLD)
    if NEW.exists() or NEW.is_symlink() or os.path.ismount(OLD):
        raise ValueError('target must be absent and source cannot be a mount point')
    verify_tools()
    lock = acquire_lock(Path('/run/newapi-release.lock'))
    try:
        before = {'containers': containers(), 'data': fingerprint(OLD), 'version': health()}
        text = (OLD / 'docker-compose.yml').read_text()
        if not re.search(r'^name\s*:\s*[\x22\x27]?newapi[\x22\x27]?\s*$', text, re.M):
            raise ValueError('Compose must explicitly preserve name: newapi')
        print('Verified current containers, data identity, health, explicit Compose project and compatibility tools')
        if not args.execute:
            print('Preview: atomic /opt/newapi -> /opt/meta-api, preserving the old compatibility path')
            return
        receipt = Path(tempfile.mkdtemp(prefix='meta-api-directory-', dir='/root'))
        (receipt / 'before.json').write_text(json.dumps(before, indent=2))
        print('Receipt: ' + str(receipt), flush=True)
        try:
            relocate(OLD, NEW)
            after = {'containers': containers(), 'data': fingerprint(NEW), 'version': health()}
            (receipt / 'after.json').write_text(json.dumps(after, indent=2))
            if after != before:
                raise RuntimeError('post-migration identity differs; inspect receipt before further operations')
            (receipt / 'success').write_text('Directory moved; containers and deployment contents unchanged.\n')
            print('PASS: actual directory /opt/meta-api; old path compatible; containers, data and version unchanged')
        except BaseException as error:
            (receipt / 'failure.txt').write_text(type(error).__name__ + ': ' + str(error) + '\n')
            raise
    finally:
        os.close(lock)


if __name__ == '__main__':
    main()
