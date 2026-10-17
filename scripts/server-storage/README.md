# New API 源站磁盘维护

适用于 `/opt/newapi` 的既有部署。独立 root 运维工具，不改变应用版本，不清理数据库记录、发票附件、Docker volume、MySQL binlog 或历史文本日志。

## 现有能力与保留规则

源站已有 `/usr/local/sbin/newapi-backup-retain.py` 和 `newapi-log-archive.py`，由 `newapi-storage-maintenance.service` 顺序执行：

- 备份保留最近三份完整部署备份；删除旧备份前校验保留备份的 SHA-256，并保护发布状态中的 `LAST_BACKUP_DIR`。缺少完整性标记的目录不作为可删除备份。
- 文本日志仅对关闭且至少一天未更新的文件做无损 gzip，校验压缩结果、再次确认原文件未改变且未被打开后替换；历史不丢弃。
- `hourly-maintenance.conf` 将现有每日检查改成每小时的第20分钟加0—5分钟随机延迟，尽快回收连续更新产生的额外备份。任务与发布共用锁，已有 CPU/IO 限制保持。

这两个已有脚本仍以服务器已安装版本为准，本目录不覆盖它们，也不是新服务器的一键安装包。

`image-retain.py` 默认只预览；传入 `--apply` 才删除。只处理全部已知标签/摘要都属于 `ghcr.io/nanashiwang/new-api` 且创建超过30天的镜像，保护：

- 全部运行及停止容器使用的镜像。
- `.release-manager/state.env` 中的当前及回滚镜像，状态缺失、不完整或对应镜像缺失时整体停止。
- 当前有效 Compose 的镜像，以及 `backups/one-click/*` 内保存的 Compose 和发布状态引用。
- 其他仓库、混合仓库标签、无法确认归属的无标签镜像。

脚本读取并持有 `/run/newapi-release.lock`，不执行 shell 配置文件，不拉取镜像，不使用 `docker image rm --force`。未知备份引用、符号链接、锁竞争或 Docker 删除失败均停止并报告；失败前已删除的旧镜像不会恢复。多标签镜像可能被 Docker 拒绝，需单独确认，不能改为全局强制 prune。

## 安装、预览与检查

先核对服务器路径、现有脚本、Compose、发布状态和备份结构；备份已有同名配置。将本目录文件复制到服务器后执行：

```sh
install -m 0755 image-retain.py /usr/local/sbin/newapi-image-retain.py
install -m 0644 newapi-image-retention.service /etc/systemd/system/
install -m 0644 newapi-image-retention.timer /etc/systemd/system/
install -d -m 0755 /etc/systemd/system/newapi-storage-maintenance.timer.d
install -m 0644 hourly-maintenance.conf /etc/systemd/system/newapi-storage-maintenance.timer.d/hourly.conf
systemctl daemon-reload
systemd-analyze verify /etc/systemd/system/newapi-image-retention.service /etc/systemd/system/newapi-image-retention.timer /etc/systemd/system/newapi-storage-maintenance.timer
/usr/local/sbin/newapi-image-retain.py
```

确认候选清单后执行 `systemctl start newapi-image-retention.service`，核对 `Result=success` 和日志，再启用 `systemctl enable --now newapi-image-retention.timer`。镜像维护每天北京时间05:45开始，加0—15分钟随机延迟，避开每小时的备份维护。重启已有 `newapi-storage-maintenance.timer` 使覆盖生效；无需重启 Docker、MySQL、Redis 或应用。

```sh
systemctl show newapi-image-retention.service -p Result -p ExecMainStatus
systemctl list-timers 'newapi-*'
journalctl -u newapi-image-retention.service -n 30 --no-pager
df -h /
python3 -B -m unittest discover -s scripts/server-storage -p 'test_*.py' -v
```

暂停新增镜像清理用 `systemctl disable --now newapi-image-retention.timer`。删除本次新增的 `hourly.conf` 后执行 `systemctl daemon-reload` 并重启原维护 timer，可恢复原每日计划；如安装前存在该覆盖文件则恢复备份。已经删除的旧镜像可按已知摘要从原仓库重新拉取，前提是远端仍保留；当前及回滚镜像始终受保护。

## 容量边界

MySQL 使用记录和保留的压缩日志会继续增长。镜像/备份维护不会为业务数据设保留期；数据库历史归档、磁盘扩容需要单独制定保留与恢复方案，不能用删行或 `OPTIMIZE TABLE` 代替容量规划。当前 binlog 已配置七天自动到期，本次不缩短恢复窗口。

## 2026-10-17 现场结果（北京时间）

- 维护前96 GiB系统盘使用91%，可用约8.2 GiB；主要为MySQL约38 GiB（使用记录表约26 GiB）、部署备份约14 GiB、应用文本日志约8.9 GiB，Docker共73个镜像。
- 原维护服务校验最近三份完整备份后删除两份旧备份，释放5666324657字节；保留当前回滚备份。本次没有日志符合压缩条件，历史日志未删除。
- 新镜像任务预览41个、实际删除41个、再次预览0个，Result=success。当前、回滚、剩余备份引用和MySQL/Redis镜像保留。
- 15:12左右系统盘使用81%，可用18580074496字节（约17.3 GiB，`df -h`显示18G）；相对清理前精确快照净增约9.1 GiB，期间仍有业务写入。
- 两个timer均enabled，systemd配置验证通过；安装的Python及三个配置SHA-256与仓库文件一致。应用仍为main-135966d，三个业务容器均无重启，数据库和Redis健康。
- 外部HTTPS复验：`/api/status`返回200且success=true；不带凭据的`/v1/models`返回401，鉴权边界保持。没有发送收费模型请求。
- 覆盖前timer快照位于服务器 `/opt/newapi/.storage-maintenance-backups/20261017-image-retention/`。没有应用版本更新或应用Release。
