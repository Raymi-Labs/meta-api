# New API 源站磁盘维护

适用于 `/opt/newapi` 的既有部署。独立 root 运维工具，不改变应用版本，不清理数据库记录、发票附件、Docker volume 或 MySQL binlog。既有维护仍只压缩历史文本日志；新增离机归档工具的显式 `retire` 才允许退役已经过远端读回验证的旧压缩日志。

## 现有能力与保留规则

源站已有 `/usr/local/sbin/newapi-backup-retain.py` 和 `newapi-log-archive.py`，由 `newapi-storage-maintenance.service` 顺序执行：

- 备份保留最近三份完整部署备份；删除旧备份前校验保留备份的 SHA-256，并保护发布状态中的 `LAST_BACKUP_DIR`。缺少完整性标记的目录不作为可删除备份。
- 文本日志仅对关闭且至少一天未更新的文件做无损 gzip，校验压缩结果、再次确认原文件未改变且未被打开后替换；历史不丢弃。
- `hourly-maintenance.conf` 将现有每日检查改成每小时的第20分钟加0—5分钟随机延迟，尽快回收连续更新产生的额外备份。任务与发布共用锁，已有 CPU/IO 限制保持。

这两个已有脚本仍以服务器已安装版本为准，本目录不覆盖它们，也不是新服务器的一键安装包。

## 超过30天的文本日志离机归档

`log-offload.py` 复用应用已有轮转及 `newapi-log-archive.py` 的 gzip 产物，不增加另一套 logrotate。第一阶段保持三份数据库备份和 `LAST_BACKUP_DIR`，不清理镜像、binlog 或业务数据库。保留期不得小于30天；扩大至14天需要另行调整策略及验收。

默认仅预览 `/opt/newapi/newapi_data/logs` 一级目录中超过30天的 `oneapi-*.log.gz` 普通文件，不创建状态目录或锁，不访问远端：

```sh
python3 scripts/server-storage/log-offload.py
```

测试通过后可独立安装，不需要重启应用或依赖服务；首次不启用定时任务：

```sh
install -m 0755 scripts/server-storage/log-offload.py /usr/local/sbin/newapi-log-offload
newapi-log-offload preview
```

预览大小只是候选量，打开状态、gzip完整性、源文件稳定性和远端可恢复性必须在实际操作中通过。实际操作要求 Linux、完整的 `/proc` 访问权限及 rclone。所有目录与配置应由执行用户拥有，不允许符号链接、硬链接或不安全权限。

先确认独立存储桶/专用前缀及加密密码、可选 salt 的独立保管位置。配置文件必须为私有文件，目标 remote 的实际类型必须为 `crypt`；仅将普通 remote 命名为“encrypted”不符合要求。工具不创建存储服务或加密凭据，不使用继承的 `RCLONE_*` 配置覆盖。归档回执不得包含密码和访问令牌。

下面的 `crypt-archive` 和配置路径都是占位值，必须替换为已经验证的目标。首次操作先核对 `preview`，然后只上传与校验：

```sh
python3 scripts/server-storage/log-offload.py archive \
  --remote crypt-archive:newapi/logs \
  --rclone-config /etc/newapi/rclone.conf
```

上传后从 crypt remote 流式解密读回，校验 SHA-256 和 gzip，不在源站额外存放一份完整下载文件。回执位于 `/opt/newapi/.log-offload`（目录0700、文件0600），记录源路径、设备/inode、大小、修改时间、SHA-256、远端位置及验证时间。完成 archive 不删除源文件。

在独立环境确认凭据可恢复、首批回执复核通过后，使用相同参数执行单独的退役阶段：

```sh
python3 scripts/server-storage/log-offload.py retire \
  --remote crypt-archive:newapi/logs \
  --rclone-config /etc/newapi/rclone.conf
```

退役再次读取远端，并在发布锁下复核源文件仍为同一普通文件、内容和年龄一致、没有被进程打开，然后逐文件删除。任一检查失败均保留该源文件并返回非零；之前已成功退役的文件不会自动恢复，可用回执中的归档位置取回。中断后重复执行相同命令收敛已有回执，不将“上传命令返回成功”当作验证通过。

归档任务使用独立任务锁阻止重复运行；与更新共享 `/run/newapi-release.lock` 的只有关键状态变更和退役，网络上传及读回期间不持发布锁。锁竞争会停止，不删除锁文件强行运行。首次验证成功前不接入生产 timer；后续定时执行应复用已验证的参数和回执流程，并报告失败与磁盘增长。

归档回执、加密配置及独立保管的密码需要一同纳入恢复安排。数据库周期备份和 PITR 是另一条流程：现有发布备份不等于周期备份，七天 binlog 不等于已经验证的七天恢复链。本工具不承担数据库归档或恢复。

隔离验证命令：

```sh
python3 -B -m unittest discover -s scripts/server-storage -p 'test_*.py' -v
# Linux且安装rclone时，显式启用随机测试凭据、临时本地crypt后端验证；不使用生产remote。
sudo env NEWAPI_RCLONE_INTEGRATION=1 python3 -B -m unittest discover -s scripts/server-storage -p 'test_log_offload_rclone.py' -v
```

2026-10-22 已完成源站隔离验证和工具安装，原发布工具及核验记录保存在 `/root/newapi-archive-tools-20261022-w6ogt0pt/`；业务容器及发布状态保持不变。实际日志仅预览30个候选、约3.58GiB，存储桶/前缀与独立凭据保管尚待指定，未执行生产上传或退役。该记录不代表异地归档、数据库恢复或磁盘回收已完成。

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
