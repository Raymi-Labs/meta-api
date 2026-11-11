#!/usr/bin/env python3
"""Package the installed combined Skill from an explicit source, excluding credentials."""
import argparse
import io
from pathlib import Path
import zipfile

ROOT = Path(__file__).resolve().parents[1]
DESTINATION = ROOT / "web/public/downloads/meta-api-imagegen-skill.zip"
FILES = [
    "SKILL.md", "agents/openai.yaml", "references/gemini-image.md", "references/gif.md",
    "references/video.md", "scripts/meta_api_imagegen.py", "scripts/meta_api_video.py",
    "scripts/windows_dpapi.py", "scripts/test_gemini_images.py", "scripts/test_gif.py",
    "scripts/test_image_boundaries.py", "scripts/test_meta_api_imagegen.py",
    "scripts/test_recovery.py", "scripts/test_returned_formats.py",
    "scripts/test_windows_dpapi.py", "tests/test_video.py", "使用说明.md", "群友快速开始.md",
]


def archive_bytes(source):
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_DEFLATED) as archive:
        for relative in sorted(FILES):
            path = source / relative
            if path.is_symlink() or not path.is_file():
                raise ValueError("Missing regular allowlisted file: " + relative)
            info = zipfile.ZipInfo("meta-api-imagegen/" + relative, (1980, 1, 1, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = 0o100644 << 16
            archive.writestr(info, path.read_bytes())
    return output.getvalue()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, required=True, help="Combined Skill directory; never credential storage")
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    data = archive_bytes(args.source)
    if args.check:
        if not DESTINATION.exists() or DESTINATION.read_bytes() != data:
            raise SystemExit("Combined Skill archive is stale; rebuild from the intended source")
        print("Combined Skill archive verified (18 allowlisted files).")
    else:
        DESTINATION.parent.mkdir(parents=True, exist_ok=True)
        DESTINATION.write_bytes(data)
        print("Built web/public/downloads/meta-api-imagegen-skill.zip (18 allowlisted files).")
