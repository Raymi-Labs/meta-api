#!/usr/bin/env bash
set -euo pipefail

image_ref="${1:?image reference required}"
expected_version="${2:?version required}"
target_arch="${3:?architecture required}"
case "$target_arch" in amd64|arm64) ;; *) exit 2 ;; esac

docker pull --platform "linux/$target_arch" "$image_ref"
actual_platform=$(docker image inspect --format '{{.Os}}/{{.Architecture}}' "$image_ref")
[[ "$actual_platform" == "linux/$target_arch" ]]
image_check_dir=$(mktemp -d)
image_check_container=''
cleanup() {
  if [[ -n "$image_check_container" ]]; then docker rm "$image_check_container" >/dev/null; fi
  rm -rf "$image_check_dir"
}
trap cleanup EXIT
image_check_container=$(docker create --platform "linux/$target_arch" "$image_ref")
docker cp "$image_check_container:/new-api" "$image_check_dir/new-api"
python3 - "$image_check_dir/new-api" "$target_arch" <<'PY'
import struct
import sys
from pathlib import Path

with Path(sys.argv[1]).open('rb') as executable:
    header = executable.read(20)
assert header[:6] == b'\x7fELF\x02\x01', 'expected a 64-bit little-endian ELF binary'
expected = {'amd64': 62, 'arm64': 183}[sys.argv[2]]
machine = struct.unpack('<H', header[18:20])[0]
assert machine == expected, f'wrong executable architecture: {machine}, expected {expected}'
print(f'ELF architecture verified: {sys.argv[2]}')
PY
actual_version=$(docker run --rm --network none --platform "linux/$target_arch" "$image_ref" -version)
[[ "$actual_version" == "$expected_version" ]] || { echo "Unexpected version: $actual_version"; exit 1; }
echo "Image verified: $image_ref ($actual_platform, $actual_version)"
