#!/usr/bin/env python3
"""Preview/remove old new-api images, protecting deployments and their backups."""
import argparse
import datetime as dt
import fcntl
import json
from pathlib import Path
import re
import shlex
import subprocess

REPOSITORY = "ghcr.io/nanashiwang/new-api"
APP = Path("/opt/newapi")
STATE_KEYS = {"CURRENT_IMAGE", "PREVIOUS_IMAGE", "CURRENT_ID", "PREVIOUS_ID"}


def run(*args):
    return subprocess.check_output(args, text=True).strip()


def read_state(path, required=False):
    values = {}
    for line in path.read_text().splitlines():
        key, sep, value = line.partition("=")
        if sep and key in STATE_KEYS:
            parsed = shlex.split(value)
            if len(parsed) > 1:
                raise ValueError(f"invalid state field: {key}")
            values[key] = parsed[0] if parsed else ""
    if required and not all(values.get(k) for k in STATE_KEYS):
        raise ValueError("incomplete current/rollback state; refusing cleanup")
    return {value for value in values.values() if value}


def belongs(reference):
    return reference.startswith(REPOSITORY + ":") or reference.startswith(REPOSITORY + "@")


def candidates(images, protected, now, days=30):
    cutoff = now - dt.timedelta(days=days)
    selected = []
    for image in images:
        references = (image.get("RepoTags") or []) + (image.get("RepoDigests") or [])
        if not references or not all(belongs(ref) for ref in references):
            continue
        if image["Id"] in protected or protected.intersection(references):
            continue
        created = dt.datetime.fromisoformat(image["Created"].replace("Z", "+00:00"))
        if created < cutoff:
            selected.append(image)
    return sorted(selected, key=lambda image: image["Created"])


def inspect_images():
    ids = sorted(set(run("docker", "image", "ls", "-aq", "--no-trunc").split()))
    if not ids:
        raise ValueError("no images found; refusing cleanup")
    return json.loads(run("docker", "image", "inspect", *ids))


def protected_references(images):
    protected = read_state(APP / ".release-manager/state.env", required=True)
    # Resolve both stored IDs and immutable references without pulling any image.
    by_ref = {}
    for image in images:
        for ref in [image["Id"]] + (image.get("RepoTags") or []) + (image.get("RepoDigests") or []):
            by_ref[ref] = image["Id"]
    if not protected.issubset(by_ref):
        raise ValueError("current/rollback image is missing; refusing cleanup")
    protected.update(by_ref[ref] for ref in list(protected))
    container_ids = run("docker", "ps", "-aq").split()
    if not container_ids:
        raise ValueError("no containers found; refusing cleanup")
    containers = json.loads(run("docker", "inspect", *container_ids))
    protected.update(container["Image"] for container in containers)
    # Compose emits only image references; environment and credentials stay private.
    protected.update(run("docker", "compose", "--project-directory", str(APP),
                         "config", "--images").splitlines())
    backup_root = APP / "backups/one-click"
    for directory in backup_root.iterdir():
        if directory.is_symlink() or not directory.is_dir():
            raise ValueError("unexpected entry in backup directory; refusing cleanup")
        state = directory / "state.env"
        if state.exists():
            protected.update(read_state(state))
        compose = directory / "docker-compose.yml"
        if compose.exists():
            for line in compose.read_text().splitlines():
                match = re.match(r"\s*image:\s*(.+?)\s*$", line)
                if match:
                    refs = shlex.split(match[1], comments=True)
                    if len(refs) != 1 or "$" in refs[0]:
                        raise ValueError("unresolved backup image; refusing cleanup")
                    protected.add(refs[0])
    protected.update(by_ref[ref] for ref in list(protected) if ref in by_ref)
    return protected


def execute(selected, apply):
    for image in selected:
        references = (image.get("RepoTags") or []) + (image.get("RepoDigests") or [])
        print(f'{"remove" if apply else "would remove"} {image["Id"]} {references}', flush=True)
        if apply:
            # No --force: Docker itself refuses a newly referenced or multi-tag image.
            subprocess.run(["docker", "image", "rm", image["Id"]], check=True)
    print(f'completed candidates={len(selected)} apply={apply}', flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apply", action="store_true", help="delete; default is preview")
    args = parser.parse_args()
    with open("/run/newapi-release.lock", "a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        images = inspect_images()
        selected = candidates(images, protected_references(images), dt.datetime.now(dt.timezone.utc))
        execute(selected, args.apply)


if __name__ == "__main__":
    main()
