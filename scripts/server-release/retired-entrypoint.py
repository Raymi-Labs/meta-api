#!/usr/bin/env python3
"""Fail closed for retired, unsafe production rollout entrypoints."""
import sys

sys.stderr.write(
    "此临时/旧发布入口已停用：原流程可能在失败后留下停止的服务。\n"
    "请使用 newapi-release status 检查；更新使用 newapi-update，"
    "回退使用 newapi-rollback。不要在更新前手动停止主服务。\n"
    "候选缓存功能尚未上线；不要直接运行已封存的脚本。\n"
)
raise SystemExit(78)
