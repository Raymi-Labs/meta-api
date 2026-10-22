#!/usr/bin/env bash
# Preserve the existing manual entrypoint while sharing the scheduled job.
set -Eeuo pipefail
if [[ "${EUID}" -ne 0 ]]; then
  echo '请以 root 运行数据库备份。' >&2
  exit 1
fi
exec systemctl start --wait newapi-r2-backup.service
