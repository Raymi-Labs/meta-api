"""Bind one static frontend archive to its exact source commit and version."""
import argparse
import hashlib
import json
from pathlib import Path, PurePosixPath
import shutil
import subprocess
import tarfile
import tempfile

ROOTS = ('web/dist', 'web/docs-dist')
MAX_BYTES = 2 * 1024 ** 3


def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
            h.update(chunk)
    return h.hexdigest()


def identity(root, version):
    if (root / 'VERSION').read_text().strip() != version:
        raise ValueError('source VERSION does not match requested frontend version')
    return subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root, text=True).strip()


def pack(root, output, version):
    commit = identity(root, version)
    output.mkdir(parents=True, exist_ok=True)
    archive = output / 'frontend.tar.gz'
    with tarfile.open(archive, 'w:gz', compresslevel=3) as tar:
        for name in ROOTS:
            if not (root / name / 'index.html').is_file():
                raise ValueError('frontend entry missing: ' + name)
            tar.add(root / name, arcname=name)
    (output / 'frontend.json').write_text(json.dumps({
        'schema': 1, 'commit': commit, 'version': version,
        'sha256': digest(archive),
    }) + '\n')


def restore(root, source, version):
    commit = identity(root, version)
    meta = json.loads((source / 'frontend.json').read_text())
    archive = source / 'frontend.tar.gz'
    if (meta.get('schema'), meta.get('commit'), meta.get('version')) != (1, commit, version):
        raise ValueError('frontend artifact belongs to a different source/version')
    if meta.get('sha256') != digest(archive):
        raise ValueError('frontend archive checksum mismatch')
    with tempfile.TemporaryDirectory(prefix='.frontend-restore-', dir=root) as temp:
        staging = Path(temp)
        with tarfile.open(archive, 'r:gz') as tar:
            members = tar.getmembers()
            if len(members) > 20000 or sum(m.size for m in members) > MAX_BYTES:
                raise ValueError('frontend archive exceeds limits')
            seen = set()
            for member in members:
                path = PurePosixPath(member.name)
                if (path.is_absolute() or '..' in path.parts or '\\' in member.name
                        or str(path) != member.name.rstrip('/') or member.size < 0
                        or member.name in seen or not (member.isdir() or member.isfile())
                        or not any(member.name == p or member.name.startswith(p + '/') for p in ROOTS)):
                    raise ValueError('unsafe frontend archive member: ' + member.name)
                seen.add(member.name)
                target = staging / member.name
                if member.isdir():
                    target.mkdir(parents=True, exist_ok=True)
                else:
                    target.parent.mkdir(parents=True, exist_ok=True)
                    with tar.extractfile(member) as src, target.open('wb') as dst:
                        shutil.copyfileobj(src, dst)
        for name in ROOTS:
            if not (staging / name / 'index.html').is_file():
                raise ValueError('frontend entry missing: ' + name)
            if (root / name).is_symlink() or (root / 'web').is_symlink():
                raise ValueError('frontend output path cannot be a symlink')
        for name in ROOTS:
            target = root / name
            if target.exists():
                shutil.rmtree(target)
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.move(str(staging / name), target)
    print('Verified shared frontend:', version, commit)


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('command', choices=['pack', 'restore'])
    parser.add_argument('--directory', required=True, type=Path)
    parser.add_argument('--version', required=True)
    args = parser.parse_args()
    {'pack': pack, 'restore': restore}[args.command](Path.cwd(), args.directory, args.version)
