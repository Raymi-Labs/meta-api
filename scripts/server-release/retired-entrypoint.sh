#!/usr/bin/env bash
printf '%s\n' '旧自动更新入口已停用。请使用 newapi-release status 检查，使用 newapi-update 更新或 newapi-rollback 回退；不要预先停止主服务。' >&2
exit 78
