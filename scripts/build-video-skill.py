#!/usr/bin/env python3
"""Build a reproducible, allowlisted public Skill archive without credentials."""
import argparse
import io
from pathlib import Path
import zipfile

ROOT = Path(__file__).resolve().parents[1]
FILES = ["SKILL.md", "scripts/meta_api_video.py"]
DESTINATION = ROOT / "web/public/downloads/meta-api-video-skill.zip"


def archive_bytes():
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_DEFLATED) as archive:
        for relative in FILES:
            info = zipfile.ZipInfo("meta-api-video/" + relative, (1980, 1, 1, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = 0o100644 << 16
            archive.writestr(info, (ROOT / "scripts/meta-api-video" / relative).read_bytes())
    return output.getvalue()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    data = archive_bytes()
    if args.check:
        if not DESTINATION.exists() or DESTINATION.read_bytes() != data:
            raise SystemExit("Video Skill archive is stale; run scripts/build-video-skill.py")
        print("Video Skill archive verified.")
    else:
        DESTINATION.parent.mkdir(parents=True, exist_ok=True)
        DESTINATION.write_bytes(data)
        print("Built web/public/downloads/meta-api-video-skill.zip")
