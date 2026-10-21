#!/usr/bin/env python3
"""Resolve an existing release to immutable sources before allocating a desktop runner."""

import argparse
import json
import re
import subprocess
from pathlib import Path


def plan(tag, platform, repository, read=None):
    if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?", tag):
        raise ValueError("必须指定完整的应用版本标签")
    if platform not in ("macos", "windows"):
        raise ValueError("仅支持 macos 或 windows")
    if read is None:
        read = lambda args: subprocess.check_output(args, text=True)
    read(["git", "fetch", "--no-tags", "--depth=1", "origin", f"refs/tags/{tag}"])
    revision = read(["git", "rev-parse", "FETCH_HEAD^{commit}"]).strip()
    if not re.fullmatch(r"[0-9a-f]{40}", revision):
        raise ValueError("标签没有解析到有效提交")
    if read(["git", "show", f"{revision}:VERSION"]).strip() != tag:
        raise ValueError("标签源码的 VERSION 不匹配")
    release = json.loads(read([
        "gh", "release", "view", tag, "--repo", repository,
        "--json", "isDraft,assets",
    ]))
    if release["isDraft"]:
        raise ValueError("请先完成 Linux 正式发布，再补充可选附件")
    complete = {
        a["name"] for a in release["assets"]
        if a["state"] == "uploaded" and a["size"] > 0
    }
    required = {f"new-api-{tag}", f"new-api-arm64-{tag}"}
    if not required <= complete:
        raise ValueError("Linux 发布附件尚未齐全，请完成后再补充可选附件")
    asset = f"new-api-macos-{tag}" if platform == "macos" else f"new-api-{tag}.exe"
    existing = [a for a in release["assets"] if a["name"] == asset]
    if existing and (len(existing) != 1 or existing[0]["state"] != "uploaded" or existing[0]["size"] <= 0):
        raise ValueError("已有附件状态不完整，请检查后再操作；不会自动覆盖")
    return {
        "revision": revision,
        "asset": asset,
        "runner": "macos-latest" if platform == "macos" else "windows-latest",
        "build_needed": "false" if existing else "true",
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--platform", required=True)
    parser.add_argument("--repository", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    result = plan(args.tag, args.platform, args.repository)
    with Path(args.output).open("a") as output:
        for key, value in result.items():
            output.write(f"{key}={value}\n")
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
