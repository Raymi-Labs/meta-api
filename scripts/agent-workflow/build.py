#!/usr/bin/env python3
"""Build a self-contained, allowlisted distribution without project data."""
import hashlib
from pathlib import Path
import sys
import zipfile

HERE = Path(__file__).resolve().parent


def build(output):
    output.mkdir(parents=True, exist_ok=True)
    version = (HERE / 'VERSION').read_text().strip()
    archive = output / ('agent-workflow-v' + version + '.zip')
    files = {name: HERE / name for name in ['SKILL.md', 'README.md', 'VERSION', 'CHANGELOG.md', 'workflow.py']}
    for p in sorted((HERE / 'templates').rglob('*')):
        if p.is_file():
            files[p.relative_to(HERE).as_posix()] = p
    source = HERE.parents[1] / '.agents/skills/write-notes-like-deepseek'
    for p in sorted(source.rglob('*')):
        if p.is_file() and p.suffix in {'.md', '.ts', '.html'}:
            files['payload/write-notes-like-deepseek/' + p.relative_to(source).as_posix()] = p
    with zipfile.ZipFile(archive, 'w', zipfile.ZIP_DEFLATED) as z:
        for name, p in sorted(files.items()):
            info = zipfile.ZipInfo('project-workflow/' + name, (2026, 10, 15, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = 0o100644 << 16
            z.writestr(info, p.read_bytes())
    checksum = hashlib.sha256(archive.read_bytes()).hexdigest()
    (output / 'SHA256SUMS.txt').write_text(checksum + '  ' + archive.name + '\n')
    print(archive)
    return archive


if __name__ == '__main__':
    build(Path(sys.argv[1]).resolve())
