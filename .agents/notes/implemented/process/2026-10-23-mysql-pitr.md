# Agent Note: MySQL 加密连续归档与事务位置恢复

Status: implemented

## Problem

每日 R2 全量备份已验证上传内容，但没有快照对应的 binlog 坐标，无法恢复两次备份之间的事务。生产 MySQL 8.0.46 使用 InnoDB、ROW/FULL、GTID OFF、CRC32、sync_binlog=1 和 innodb_flush_log_at_trx_commit=1；只有 new-api 业务库。磁盘紧张，不能积累另一份全量转储或重复复制所有 binlog。用户要求正常运行 RPO 不超过五分钟，并完成真实全量与增量恢复验收。

## Decision

复用现有 crypt remote、全备流式管道、内容读回、原子回执与 systemd。原每日任务保留作回退。新增 opt-in 全备锚点，从同一次 mysqldump --single-transaction --source-data=2 输出提取坐标。容器内独立 watchdog 限制初始全局读锁阶段，超时精确 KILL CONNECTION，不能仅终止客户端；报告启动至解锁确认的上界，不将其称为精确锁时间。

全备采用 LOCK INSTANCE FOR BACKUP 防并发 DDL，只有取得实例锁时短持发布锁；受保护发布入口检测实例锁并快速退出。归档及远端读回不持发布锁。binlog 按完整事件的小区间保存，FDE 的 IN_USE 位规范化，验证 CRC、位置、事务边界、连续文件及实例身份；不轮转、不清理、不缩短源 binlog。先落 pending，再上传、解密读回、远端 hash 链回执，最后推进 checkpoint。远端 root 与回执枚举支持丢失本机状态后的恢复。

独立监控比较实时源位置与远端已验证事务位置、覆盖采样时间和服务状态，超过五分钟或出现缺失/错误时告警；空闲无事务不能误报。恢复工具只创建隔离网络且无生产挂载的 MySQL，验证全备和连续链后按明确事务位置回放。生产验收以 A 全备加增量至 T 与独立 B 全备（锚点 T）的全表摘要、schema、行数及账务汇总比较为准。

## Existing notes audit

- [每日 R2 全备](../../implemented/process/2026-10-22-continuous-r2-database-backup.md)：部分取代。不采集坐标和长持发布锁的旧取舍由本篇 opt-in PITR 路径替代；旧任务、密钥保管、流式低磁盘策略仍成立。
- [恢复包与日志离机](../../implemented/process/2026-10-22-verified-recovery-archives.md)：相邻能力，日志退役不属于本轮。
- [发布安全](../../implemented/process/2026-10-21-release-entrypoint-safety.md)：部分重叠，仅增加备份实例锁快速拒绝，不更新或停止业务容器。
- [存储维护](../../implemented/process/2026-10-17-server-storage-maintenance.md)：保留全部备份、LAST_BACKUP_DIR 与七天 binlog，不改变清理规则。

## Alternatives considered

- mysqlbinlog --raw --stop-never 使用官方协议且减少自行读取源文件的代码，但需要本机重复存储 binlog，断线不自动重连，仍需处理活跃文件分段及校验。选择受限格式解析和小区间归档，遇未知格式停止而不猜测。
- 周期 FLUSH BINARY LOGS 后上传整文件更简单，但可能触发既有过期清理，且大文件读回使五分钟 RPO 难保证，因此不主动轮转。
- 独立 SHOW MASTER STATUS 作为全备坐标不增加短暂全局读锁，但不与一致性快照原子对应，不能用作恢复锚点。
- 将同一全备和增量恢复两遍只能证明确定性。第二份独立一致性全备提供同坐标来源证据，代价是额外全库扫描和隔离恢复时间。

## Rollout stages

Linux 单测和真实 MySQL 集成测试覆盖锁阻塞、进程中断、失败上传、断点、丢段、损坏、轮转及事务边界。CI 执行相同测试。先完成代码与隔离验收，再安装候选工具、验证真实锚点与连续链；通过后允许候选归档和监控持续运行，完成真实 A/B 恢复后才切换每日全备任务。观察远端已验证位置的实际 RPO；告警发送、恢复通知、旧任务回退与应用容器未变均需证据。

## Consequences

全量扫描及网络背压延长快照和 DDL 保护时间；原有应用 DML 继续运行，但发布需等待快照结束后重试。未支持的 binlog 格式必须停链并告警。客户端加密材料和独立存储账号必须可恢复。主机或网络灾难不能保证五分钟 RPO，目标仅指正常运行；失败时明确暴露欠账而不宣称零丢失。

## Verification

隔离 MySQL 8.0.46 演练使用真实 `backup.execute(--pitr-anchor)`、`Archive.run/Store/CryptRemote` 和恢复入口：48 个连续分段，从 binlog.000001:3250 回放至独立 B 锚点 binlog.000002:5569，六张测试业务表的 schema、行数、全字段摘要及账务聚合一致。覆盖充值、扣费、退款、目标后事务排除、DDL、轮转、发布预检临时库、归档中断恢复、空闲幂等、指定位置导出再导入及 SIGTERM 清理。

受保护发布所创建的临时库不进入业务恢复范围；原始 binlog 完整保存。恢复严格识别既有预检库/账户操作，范围不明确的跨库或复杂 DDL 拒绝自动回放。代码与隔离验证完成不代表生产 PITR 已验收；真实 A/B 数据、生产 RPO 与邮件投递仍须按分阶段流程记录。

Linux 存储套件 155 项中 147 项通过、8 项独立真实 MySQL 实验按入口跳过；单独运行锚点套件 15 项全部通过（含八项真实 MySQL 实验）。发布安全套件 46 项全部通过。监控 17 项、邮件配置四项均通过；既有备份 35 项 Linux 回归通过。完整 Linux/MySQL E2E 通过后由 CI 再执行同一入口。

## Production candidate evidence

2026-10-23 首轮候选开始前，代码提交 fc464ea55 的 Linux/MySQL 完整 CI、Notes 与镜像构建全部通过。已安装候选工具并保留原每日任务；应用、MySQL、Redis 容器及发布状态不变。管理员 SMTP 的 TLS/认证通过，带 TEST 标记的通知获 SMTP 接受，私有配置 0600。

首轮候选 dump 的客户端确认标记在约 7.145 秒出现，超过启动守卫的五秒上界。该标记位于 UNLOCK 之后的数据库准备步骤之后，因此无法凭此推断真实全局持锁时间。发现 late-ready 竞态后取消候选任务，回执标记 failed、没有发布 SUCCESS；随后确认备份 SQL 连接及 BACKUP/GLOBAL/COMMIT 锁消失，业务健康 200。继续修复守卫证据与真实延迟测试，不能把候选安装或邮件发送当作 PITR 恢复验收。

守卫修复改用单调时钟和精确会话的服务器证据：成功 UNLOCK history 或已开始 ACTIVE REPEATABLE READ 事务且无 GLOBAL/COMMIT 锁，并硬校验实际 instrumentation/consumer/session 可观测性；记录 startup_unlock_evidence。真实测试覆盖解锁后 SHOW CREATE 延迟七秒仍成功、SIGSTOP 使守卫晚于五秒调度时仍取消未完成大表，以及 SQL 连接取消与输出管道生命周期的区别。

隔离恢复客户端与服务器包上限为 256 MiB，真实 20 MiB 大行产生 41,943,102 字节 ROW 事件；54 段、七表 A/B 比较、导出再导入及信号清理全部通过。归档解析消除不必要的整事件复制，768 MiB/no-swap cgroup 内实际 Source/Archive/Store/crypt 处理精确 64 MiB framing 边界，中断续传及幂等通过，峰值 289345536 字节且 OOM=0；此合成边界测试不替代前述真实 ROW 回放测试。

修复后的 Linux 存储套件共 161 项，150 项通过、11 项真实 MySQL 用例在该入口按配置跳过；独立锚点套件 18/18 通过（七项单测、11 项真实 MySQL，包括明确观察到 FTWRL 等待后的取消）。旧备份 35/35、监控与邮件配置 21/21 通过。首次根用户运行 Linux bind mount 因宿主文件归属不匹配而触发 helper 安全拒绝，使用宿主 UID 运行后全套通过；没有放宽该归属检查。

bb0066884 的 CI 在前 17 个锚点用例通过后，最后 SIGSTOP 用例持续未结束，人工取消以保留日志；不能视为通过。补齐测试 Docker 调用、流式入口与 CI 步骤的超时，先取消和回收进程再删除协调文件，并每 45 秒打印阻塞栈。Linux Python、Docker CLI 与独立 MySQL 的完整 18 项和 SIGSTOP 单项随后均通过，但未复现原 CI 卡点，故此改动只定性为测试生命周期及诊断加固，不声称已定位根因；生产代码和五秒启动标准保持不变。

af475bf5c 的 CI 随后有界失败。用独立 amd64 MySQL 与旧取消代码复现后确认：不消费大段 Docker stdout 会连带阻塞同容器的其他 docker exec 观察命令；恢复消费后可看到 timed-out、cancel-confirmed 和服务器连接已消失。因此不能把协调文件读取超时等同于容器内守卫失效。故障用例改用容器内输出背压，保留 Docker 观察通道；同时真实生产外部中断路径也先解除本次宿主输出桥，再独立取消并确认 SQL 连接。

取消流程另补统一的容器内八秒预算，SQL 超时随剩余时间收紧；首次连接查询失败也尝试核验 PID、starttime 和进程名后终止本次客户端，最终仍要求服务器确认连接消失。不能仅终止宿主 docker exec 客户端来约束容器内取消脚本，也不能把客户端退出当作服务器确认。测试保留原始异常与附加清理结果，诊断仅采集有限进程和连接元数据。

修复后 Linux 存储套件 165 项中 150 项通过、15 项独立 MySQL 用例按入口跳过；真实 Linux/MySQL 套件 22/22 通过。amd64 单独验证容器内 FIFO late-ready 竞态，以及 4 KiB 宿主上传管道背压下的 deadline 和 SIGTERM，均确认 SQL 连接消失和实例锁释放。新取消测试在旧实现上的反证分别得到首次查询失败直接退出、慢重试超过 12 秒外部上限；新实现通过同样故障注入。
