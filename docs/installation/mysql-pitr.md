# MySQL 事务位置恢复（PITR）

本工具扩展已有 R2 周期备份：带精确坐标的全量快照、连续加密 binlog、独立覆盖监控、隔离恢复和账务核对。上传成功、SHA 校验成功和恢复通过是不同状态；只有真实全量加增量回放并通过独立快照对照，才记为生产恢复验收。

生产范围为 MySQL 8.0.46、唯一业务库 `new-api`、全部基础表 InnoDB、ROW/FULL、CRC32、GTID OFF、未压缩且未在 MySQL 内加密的 binlog、`sync_binlog=1`、`innodb_flush_log_at_trx_commit=1`。源 binlog 已经通过 rclone crypt 加密保存到现有 `r2-backup`。不更改七天本地保留，不执行 FLUSH/PURGE，不删除旧备份或业务行，不更新应用版本。

## 组件与状态

| 组件 | 作用 | 私有状态 |
| --- | --- | --- |
| `mysql-r2-backup.py run --pitr-anchor` | 同一 mysqldump 的一致快照与坐标，流式上传和完整读回 | `/var/lib/newapi-pitr-backup` |
| `mysql-binlog-archive.py` | 每分钟归档完整事件区间；远端读回后推进连续链 | `/var/lib/newapi-binlog-archive` |
| `pitr-monitor.py` | 独立查询源位置、重读远端回执及尾段、校验全备新鲜度和磁盘 | `/var/lib/newapi-pitr-monitor` |
| `pitr-alert-config.py` | 复用现有 SMTP 与唯一启用的超级管理员邮箱 | `/root/.config/newapi-pitr/alerts.json` |
| `mysql-pitr-restore.py` | 只创建独立隔离 MySQL，按明确位置恢复或做 A/B 对照 | 用户指定的新建 0700 工作目录 |

新工具及相邻 Python 模块安装到 `/usr/local/lib/newapi-storage`。原 `/usr/local/sbin/newapi-mysql-r2-backup`、`newapi-r2-backup.service` 和 timer 保留为回退入口。两种全备共用 job lock，避免同时运行；启用 PITR 日常全备时只停用旧 timer，不删除其文件或历史。

## 快照锚点与锁

`--single-transaction --source-data=2` 在开始时短暂获取全局读锁，建立一致快照、输出坐标，然后解锁。元数据的 `source.binlog_anchor` 必须与该 dump 头部坐标一致；独立查询的 `SHOW MASTER STATUS` 不能替代这一锚点。

容器内 watchdog 使用单调时钟检查五秒启动期限，并给整轮转储设置时限。预检要求 MySQL 的 statement history、当前事务和元数据锁可观测；守卫按精确 mysqldump 会话核验已成功完成 UNLOCK，或已进入一致性事务且不再持有 GLOBAL/COMMIT 锁。不能仅凭“没有锁记录”认定已解锁。小型 dump 提前断开时，只有期限内的解锁后标记可作为回退证据；晚到标记不能解除超时取消。

上传背压可能堵住 Docker 输出桥，连带阻塞同容器的新 `docker exec` 控制命令。外部中断先关闭本次宿主输出桥，再执行精确 SQL 取消与消失确认。容器内取消共用八秒预算，首次状态查询失败也会核验 PID/starttime/进程名后终止本次客户端；宿主 CLI 或客户端退出都不能代替服务器确认。未确认时保留失败状态和原始错误，不能发布 SUCCESS。

超时先按 mysqldump PID 与客户端连接属性精确 `KILL CONNECTION`，再取消客户端并确认服务器连接消失。仅终止宿主 docker exec 或客户端不能证明等待读锁已经取消；反过来，Docker 输出管道仍等待消费，也不能证明数据库会话仍存活。回执中的 `startup_lock_window_upper_bound_seconds` 是启动至确认解锁的保守上界，不是精确持锁时长，`startup_unlock_evidence` 记录采用的固定证据类型。

独立连接持有 `LOCK INSTANCE FOR BACKUP` 防止转储期间结构变化，普通业务读写可继续。获得实例锁时短持发布锁，此后释放发布锁；`newapi-update`/`newapi-rollback`/`newapi-restart` 检测实例锁并快速报错，运维待快照结束后重试。重启入口在停止运行中容器之前检查，避免启动迁移等待 DDL 锁。完整转储读完后释放实例锁，远端读回不持锁。心跳失联的实例锁连接通过十秒空闲租约释放。网络背压仍可能延长数据库快照和 DDL 保护时间，不能将流式上传说成无影响。

## 归档协议与失败行为

远端目录为 `r2-backup:pitr/<server_uuid>/<epoch>`，包含不可变 `root.json`、`segments/` 和递增 `receipts/000000000001.json`。回执包含文件、起止字节、SHA-256、前条回执摘要、解析状态、源采样位置及时间、最后完整事务位置。序号和字节区间必须连续；跨文件必须连续并验证 ROTATE。

目标分段 16 MiB，按完整事件结束；单个较大事件有 64 MiB 上限。worker 的 systemd 内存上限为 768 MiB；CI 在相同限制、禁用 swap 的隔离环境中执行最大事件的真实 crypt 上传、读回、中断恢复与幂等测试。事务可以跨分段，恢复终点必须是完整事务或安全元数据边界，不能在半个事务中停止。只规范化 FDE 的 `BINLOG_IN_USE` 位，避免活跃文件关闭时改变首段摘要；其余内容不改写。每个事件验证长度、位置及 CRC32。XA、压缩事务载荷、未知事件、语句模式 DML 明确失败。

先原子记录 pending，再上传、解密读回、上传并读回回执，最后推进 checkpoint。中断后重放同一 pending，不能跳到新位置。UUID 改变、文件缩短/复用、FDE 改变、源文件已过期、范围不连续或远端已有对象不一致都停止归档。缺口不能靠重新命名 epoch 或跳过坏对象掩盖：需要先保留故障证据，恢复缺段，或建立一份新的验证全备及明确的新恢复链。

远端对象存在性通过 `rclone lsjson --stat --files-only` 检查，兼容 R2/S3 对缺失文件返回 JSON null 的语义。默认 `--stat` 可能把不存在的桶内路径报告为虚拟目录，不能据此判断已有归档内容。认证和传输错误仍会停止，不能当作对象不存在后继续上传。

本机状态丢失时，远端 root 与有序回执仍可用于灾难恢复。正常归档不会擅自重建已有远端链的本机 checkpoint；应先离线完整验证，显式恢复状态或建立新的全备链。恢复材料包括 crypt 密码、可选 salt 和远端配置；这些材料独立保管，不能与唯一生产主机一起丢失。

## 分阶段安装与启用

1. 在独立 Linux/MySQL 8.0.46 环境通过单测、失败中断测试和真实完整流程；CI 运行同样的安全门禁。
2. 确认当前生产库、binlog 设置、文件和持久化来源；记录容器 ID、启动时间、重启次数、发布状态与磁盘。不能将 AI OS 主机当成 New API 主机。
3. 备份原发布工具、原备份任务与待安装文件摘要。取得 `/run/newapi-release.lock` 后安装保护入口与新工具；新 timer 保持未启用。`systemctl daemon-reload` 不重启业务容器。
4. 手动执行一份候选带锚点全备 A；从其锚点所在文件的 offset 0 启动归档，等待内容读回与连续链校验完成。首轮追赶可给较长 deadline，正常单轮保持 45 秒。失败保留 A、原任务和所有源文件。
5. A 的锚点和连续链验证通过、SMTP 测试通知获接受后，可启用候选归档与独立监控，让隔离演练期间的增量继续异地保存；此时仍保留旧每日全备任务，不能宣称 PITR 已验收。取得独立全备 B，其锚点作为目标 T；归档必须覆盖 T。在工作站恢复 A 加真实增量至 T，再独立恢复 B，核对整库摘要与账务。恢复不在生产机运行。
6. 完整恢复验收通过后启用每日北京时间 04:10 的新全备 timer。停用旧全备 timer，但保留 service 和原程序作回退。观察多个周期的远端已验证位置，不能只检查 timer 为 active。

切换前确认新旧全备 service 均已成功结束、没有活动进程、排队任务或自动重试，且最新独立监控仍正常。停用旧 timer 必须同时停止其当前调度（`disable --now`）；单独 `disable` 只取消开机启用。若持有备份 job lock 和发布锁做最后复核，须在启用新 timer 前释放，避免即时补跑取锁失败。`Persistent=true` 可能根据已有触发时间记录立即补跑，手动执行全备不等于 timer 已触发；保留其记录，检查 `LastTriggerUSec`、`NextElapseUSecRealtime`，若发生补跑则继续等到新回执验证成功。

安装时将同版本的 `mysql-r2-backup.py`、`pitr_anchor.py`、`pitr_binlog.py`、`mysql-binlog-archive.py`、`pitr-monitor.py`、`pitr-alert-config.py` 一起部署；不要只复制入口漏掉相邻模块。systemd 文件在 `scripts/server-storage/`，使用已有 `/root/.config/rclone/r2.env`，不把 AWS 凭据写入 unit。

```bash
# 先预览；输出只有配置是否齐全，不包含地址或凭据。
python3 /usr/local/lib/newapi-storage/pitr-alert-config.py
# 已选择现有管理员邮箱时，保存独立 0600 配置，不发送邮件。
python3 /usr/local/lib/newapi-storage/pitr-alert-config.py --write

# 候选全备，旧入口保持可用；与旧任务共用 job lock。
systemctl start --no-block newapi-pitr-backup.service
systemctl show newapi-pitr-backup.service -p ActiveState -p Result -p ExecMainStatus
# 必须等待任务结束，检查 latest-success.json 的真实新 run 和远端 SUCCESS。

# 归档首轮：用 A 的真实文件名替换占位值，不能随意跳过历史。
python3 /usr/local/lib/newapi-storage/mysql-binlog-archive.py \
  --start-file binlog.XXXXXX --deadline 600

# A 锚点、链及测试告警验证后，可在演练期间持续归档；仍未完成 PITR 验收。
systemctl enable --now newapi-binlog-archive.timer newapi-pitr-monitor.timer

# A/B 完整恢复验收通过后才切换每日任务。systemd 自动加载已有 AWS 环境。
systemctl disable --now newapi-r2-backup.timer
systemctl enable --now newapi-pitr-backup.timer
```

不要直接将 `binlog.XXXXXX` 示例用于生产。手动运行 rclone 相关命令时，凭据必须由 root 私有环境文件加载；不要复制到 shell 历史或聊天。

## RPO 与告警

正常运行目标是远端已验证的事务覆盖落后不超过 300 秒。记录源位置、采样时间、远端读回完成时间及最新完整事务位置；欠账以最近已完全覆盖的源采样时间计算，不能拿 binlog 事件时间或“最近上传时间”代替。完全空闲时以实际位置相等确认没有欠账；FDE/ROTATE 等安全元数据不伪造新事务。

监控每分钟独立执行，重新核对远端尾段内容及回执。源 UUID/位置倒退、worker 观测超过 180 秒、全备超过 36 小时、磁盘不足 5 GiB、归档错误或覆盖超过五分钟都会失败。通知按错误状态去重，一小时仍未恢复再提醒，恢复后发送恢复通知。投递失败不推进通知状态，下一轮继续尝试；`last-delivery.json` 记录 SMTP 接受或失败，不将 SMTP 接受等同用户已读。

生产监控 unit 还通过重复的 `--check-unit` 检查新全备和归档 service 的最近运行结果。服务缺失、失败、超时、信号退出或自动重试会在下一次监控时告警，不能被尚未过期的上一份成功全备回执掩盖。正常启动、运行和成功结束不报警；候选阶段不要求新全备 timer 已启用。手动运行监控器时如需同样检查，须传入这两个 service 参数。

确认全备服务失败后，监控在私有状态目录持久保存 `backup-failure.json`。重试进入 activating/running、Result重置为success或上一份成功全备仍新鲜，都不能解除故障；只有故障观测后启动的新全备通过远端SUCCESS、manifest及最终覆盖检查，才删除水位并发送恢复通知。水位保留原始固定诊断，提醒仍按现有周期去重。systemctl查询超时、不可用或输出无效只表示本轮观测失败，不新建或抬高全备失败水位；水位损坏则失败关闭。不得手动删除水位来消除告警。

全备读回中明确的读取进程失败最多重读同一对象三次，尝试结果写入 `readback_attempts`；不重新持有快照、不放宽gzip或内容哈希校验。若发生信号中断、总期限、真实完整性错误或重试耗尽，仍保留原成功回执且本轮失败。故障修复与邮件验证边界见[决策记录](../../.agents/notes/implemented/bug-fix/2026-10-26-pitr-readback-and-recovery-alerts.md)。

```bash
systemctl list-timers 'newapi-*'
journalctl -u newapi-binlog-archive.service -u newapi-pitr-monitor.service --since '30 minutes ago'
# 已完成配置与链验证后，发送明确标注 TEST 的邮件。
# 此手动命令同样需要私有 AWS 环境。
python3 /usr/local/lib/newapi-storage/pitr-monitor.py --test-alert
```

这是正常运行的实测目标，不是主机损坏、网络中断时的零丢失保证。监控独立于应用和归档 worker，但同主机断电仍需要外部存活检测；SMTP 故障也不能保证邮件抵达。

## 在独立环境恢复

先准备可访问现有 R2 的私有 crypt 配置和凭据、Python3、rclone、Docker，以及足够的工作站/虚拟机磁盘。工具不接受现有 MySQL 地址或容器作为恢复目标。容量必须分别核对：

- 宿主工作目录容纳 A、B 两份压缩全备、从 A 所在文件起点至覆盖 T 的分段末尾的 binlog，以及至少一个分段的临时空间。显式位置导出还需一份 `recovered.sql.gz` 的空间。
- Docker 数据盘容纳一份恢复后的完整数据库、回放 binlog 的额外副本，以及 InnoDB redo/undo、索引构建和排序临时文件余量。A 增量恢复容器清理后才创建 B 对照容器，两份恢复数据库不会同时存在。

Docker Desktop 虚拟磁盘或独立 Docker 数据分区的可用空间不能用宿主工作目录的 `df` 代替；同一物理磁盘还要合计上述占用。工具下载前的余量检查只覆盖工作目录，不代表 Docker 数据盘容量已通过验收。

恢复仍从 epoch 起点逐段下载并校验至 T，以验证完整回执链和解析状态；早于 A 所在文件的分段通过校验后即删除，只保留验证摘要，不拼接保存历史文件。网络读取量、验证时间和回执摘要规模仍随 epoch 历史增长，不能将较新的全备理解为只需读取 A→T 的远端对象。远端连续归档自身的存储量也会随历史增长，本工具不自动删除它。

恢复工具每读完一个回执或分段即关闭对应读取管道，异常路径同样释放，避免数千段长链耗尽文件描述符。不要用提高系统限制代替升级修复；这项资源释放不提供下载断点续跑，也不改变内容或连续链校验。

`--timeout-seconds` 默认 21600（六小时），覆盖下载、gzip 校验、两次恢复及整库审计的全过程，可按独立环境的实测速率调大。恢复和行哈希均流式处理；整库行哈希、行数与财务分组仍需扫描大表，无主键表还可能需要磁盘排序，六小时不是恢复时间承诺。失败后保留私有工作材料，但当前不支持在原目录续跑；重试必须使用新的工作目录并重新校验，先把已有材料占用纳入容量预算。

隔离 MySQL、恢复用 `mysql` 和导出用 `mysqldump` 显式使用 256 MiB 的 `max_allowed_packet`，避免客户端默认 16 MiB、服务端默认 64 MiB 对大行及 base64 binlog 回放 SQL 的限制；生产配置不因此改变。归档单事件仍限制为 64 MiB。包上限不是预分配内存，但全字段审计的 `HEX` 和 JSON 构造在服务端会放大单行的临时内存，不能把客户端流式读取等同于服务端恒定内存。真实集成测试覆盖 20 MiB 二进制行、约 40 MiB 的更新事件、完整恢复/回放和导出再导入。

```bash
docker build -t newapi-pitr-tools:8.0.46 -f scripts/server-storage/Dockerfile.pitr-tools .
python3 scripts/server-storage/mysql-pitr-restore.py \
  --backup-a 'r2-backup:full/YYYY/MM/A-RUN' \
  --backup-b 'r2-backup:full/YYYY/MM/B-RUN' \
  --epoch 'r2-backup:pitr/UUID/EPOCH' \
  --rclone-config '/absolute/private/rclone.conf' \
  --work-dir '/absolute/private/new-pitr-drill'
```

归档保留全部源事件；恢复仅回放已声明的业务库。受保护发布入口的 `newapi_preflight_<14位时间戳>_<非零PID>` 临时库及对应账户操作会经过严格识别后排除。普通业务库的 TABLE/INDEX DDL 仅在范围可确定时允许；复杂存储程序、歧义注释/引号、跨业务库 DDL 会拒绝自动回放，不能靠强制跳过完成验收。

A/B 来自同实例但必须是不同快照；B 的锚点决定 T。工具下载验证 SUCCESS、manifest、gzip、dump 内锚点和连续 binlog，使用官方签名校验过的 MySQL 8.0.46 `mysqlbinlog` 实际回放。临时容器带独有标签、`network none`、无端口、无宿主目录挂载，关闭事件调度器；完成或失败只清理自己创建的容器与卷。私有工作目录保留结果和下载材料。

按指定位置恢复时使用 `--stop-file` 和 `--stop-position` 替代 `--backup-b`。位置是事务执行后的结束位置；先确定所需事务边界，工具拒绝缺段、CRC 错误、未知事件及半个事务。需要交付恢复数据时添加 `--export-restored-dump`，在私有工作目录生成 `recovered.sql.gz` 和内容指纹，需要额外磁盘。此模式提供隔离恢复审计；它不自带独立 B 快照对照，也不会自动切换生产数据库。

审计覆盖实际 schema、所有表行数、按主键排序的全字段 SHA-256；无主键使用保留重复行的行摘要排序。仅忽略非事务性的 AUTO_INCREMENT 分配器高水位差异，同时保留原始 schema 摘要。还核对钱包余额、付费资金、充值、预扣/退款、订阅、令牌、任务和使用记录。按币种/状态分组，不把套餐镜像充值重复计入钱包；异步日志和累计计数不能用一条简单等式强行“对平”。工具只输出摘要和汇总，不输出用户令牌、付款 payload 或秘密配置。

## 2026-10-23 生产验收记录

已在独立 MySQL 8.0.46 环境完成真实生产恢复：全备 A 从 `binlog.000182:999096542` 回放至独立全备 B 的 `binlog.000182:1039603177`，验证 65 份连续归档回执。两份恢复结果的 71 张表结构、行数、全字段指纹及 17 张表财务汇总一致，差异为零。窗口内新增充值类日志 14 条、扣费日志 3,509 条；充值类日志不能等同支付交易笔数。本窗口没有新增退款日志样本，既有退款相关数据状态参与全表核对，新增退款场景另由真实 MySQL 隔离演练覆盖。

验收证书绑定的连续监控窗口为 05:32:13–09:32:41 UTC，共 234 个正常周期，实测最大覆盖落后 90.541764 秒，满足正常运行不超过 300 秒的目标。证书已保存至现有 crypt remote 的 `pitr-validation/<B-run>/acceptance.json` 并完成解密读回校验；原始对账结果及该窗口的固定观测快照保存在私有验收目录。验收证书独立保存，原备份成功标记保持不可变。

09:36:20 UTC 完成每日任务交接：新 PITR 全备 timer 已启用，下一次计划执行为北京时间 2026-10-24 04:10；旧 timer 已停止调度，原程序、service、timer 文件和历史备份保留。本次没有触发即时补跑。归档和独立邮件监控继续运行，生产应用及数据库容器的 ID、启动时间和重启次数保持不变。

本次包含下载、两次恢复和整库审计的完整流程耗时约 3 小时 40 分 29 秒，不能用五分钟 RPO 推断恢复耗时。已验证链从本次带锚点全备开始；此前旧全备和本地七天 binlog 保留，不代表此前七天均已完成 PITR 验收。

## 回退与故障处理

若新全备失败或链验证未通过，保持旧任务运行，不清理任何备份或 binlog。已切换后需要回退时：

```bash
systemctl disable --now newapi-pitr-backup.timer
systemctl enable --now newapi-r2-backup.timer
# 有归档故障时保留其状态、pending、远端对象和所有源 binlog。
# 停用自动尝试不等于解决缺口；保留监控告警，按回执定位原因。
systemctl disable --now newapi-binlog-archive.timer
```

停止 timer 不会取消已运行的全备。需要取消本轮时使用 `systemctl stop newapi-pitr-backup.service`，等待工具取消自身 SQL 连接并确认实例锁消失；不能停止 MySQL 或 New API。不要删锁文件、跳过失败回执、使用 `--force-if-open`/`--force-read`，或用自动 purge 追求磁盘目标。

参考：[MySQL 一致性转储](https://dev.mysql.com/doc/refman/8.0/en/mysqldump.html)、[备份实例锁](https://dev.mysql.com/doc/refman/8.0/en/lock-instance-for-backup.html)、[按事件位置恢复](https://dev.mysql.com/doc/refman/8.0/en/point-in-time-recovery-positions.html)、[包大小限制](https://dev.mysql.com/doc/refman/8.0/en/packet-too-large.html)。
