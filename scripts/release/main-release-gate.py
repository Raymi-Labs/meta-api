"""Defer a release commit's SHA image only when its exact version tag exists."""
import os
from pathlib import Path
import re
import subprocess
import time


def exact_tag(output, version, commit):
    refs = dict(line.split('\t', 1)[::-1] for line in output.splitlines() if '\t' in line)
    ref = 'refs/tags/' + version
    return refs.get(ref + '^{}', refs.get(ref)) == commit


def should_defer(event, version, previous, commit, lookup, wait=time.sleep, clock=time.monotonic, budget=45):
    if event != 'push' or version == previous or not re.fullmatch(r'v\d+\.\d+\.\d+(?:-[A-Za-z0-9.-]+)?', version):
        return False
    deadline = clock() + budget
    while True:
        try:
            if exact_tag(lookup(version), version, commit):
                return True
        except (OSError, subprocess.SubprocessError):
            pass  # An unconfirmed release never suppresses the ordinary SHA build.
        remaining = deadline - clock()
        if remaining <= 0:
            return False
        wait(min(10, remaining))


if __name__ == '__main__':
    version = Path('VERSION').read_text().strip()
    commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
    previous = subprocess.run(['git', 'show', 'HEAD^:VERSION'], capture_output=True, text=True).stdout.strip()
    def lookup(tag):
        return subprocess.check_output(['git', 'ls-remote', '--tags', 'origin', 'refs/tags/' + tag, 'refs/tags/' + tag + '^{}'], text=True, timeout=10)
    skip = should_defer(os.environ['GITHUB_EVENT_NAME'], version, previous, commit, lookup)
    with open(os.environ['GITHUB_OUTPUT'], 'a') as output:
        output.write('release_build=' + str(skip).lower() + '\n')
    print('Exact release tag confirmed; release workflow publishes the same SHA image.' if skip else 'Build the ordinary main SHA image.')
