# Agent Note: OVH 全机迁移的隔离环境准备

Status: implemented

## Problem

用户准备将 NAN 的 New API 及依赖整体迁入已有 OVH 主机。新机 Ubuntu 26.04.1、RAID 1、Docker 和 SSH 防护已存在，但缺少 Compose；旧 rclone 读 R2 成功、写入曾返回 501。现有生产版本是 compat.91、MySQL 8.0.46、Redis 7.4.9，不能用 main 或可变镜像标签替代生产基线。既有 PITR 恢复器仅管理临时隔离容器，不能当作永久目标同步工具。

## Decision

复用 `/opt/newapi`、`newapi_default`、`newapi-mysql`、`newapi-redis` 和真实 bind mount；准备配置只定义数据库与缓存，不含应用服务。固定已实查的生产镜像摘要；空 MySQL 首次初始化后进入持久只读状态，使用新 UUID/server_id，保留 ROW/FULL、GTID OFF、持久化 1/1 和七天 binlog。两种依赖不发布宿主端口，位于 internal 网络。

安装 Ubuntu Compose 包，预取原应用镜像，仅在无网络、无生产配置的独立临时容器验证版本与健康。MySQL 初始 buffer pool 为 16 GiB、容器上限 24 GiB，Redis maxmemory 为 1 GiB/noeviction、容器上限 2 GiB；保留内存给未来应用和其他服务，真实负载性能另验收。运维工具复用现有源码，PITR timer 不启用，旧发布状态和归档 checkpoint 不作为新机事实。复用现有 R2 连接，在独立测试前缀验证新机客户端的小文件、流式和 multipart 加密上传及完整解密读回；保留原客户端和测试证据。

## Existing notes audit

- [PITR](../../implemented/process/2026-10-23-mysql-pitr.md)：部分重叠，复用工具和恢复范围，未改变恢复协议。
- [备份故障闭环](../../implemented/bug-fix/2026-10-26-pitr-readback-and-recovery-alerts.md)：部分重叠，本轮解决新主机写入前置条件，历史故障原因与验收仍保留。
- [发布安全](../../implemented/process/2026-10-21-release-entrypoint-safety.md)：相邻约束，准备阶段不执行应用 init/update，不创建伪部署状态。
- SSH 防护、七云反代迁移及桌面/公告等检索结果：非本轮范围，不改变既有决定。

## Alternatives considered

- 直接启动生产恢复副本的完整应用：能较早看到业务页面，但启动迁移、定时任务和支付/上游外联会产生副作用，因此先只准备依赖与离线镜像测试。
- 同时升级应用、拆日志库或转托管数据库：可一次完成更多架构工作，但改变数据范围、权限和恢复协议，难以区分迁移问题；保持单库和现行镜像。
- 将旧归档状态原样复制：减少配置工作，但新 UUID 与旧源绑定不一致；新机正式归档必须从新锚点建立独立链。

## Verification

2026-10-26 新机 MySQL 8.0.46 / Redis 7.4.9 健康；数据库唯一业务范围 `new-api`、零表，新 UUID 与源机不同。双只读、ROW/FULL、CRC32、GTID OFF、持久化 1/1、七天 binlog、PITR 所需 performance_schema 可观测性通过实际查询；写入被 1290 拒绝。重新创建两个依赖容器后 UUID 与只读保护仍成立。无应用服务和数据库端口发布，internal 网络、宿主监听及外部端口探测相互验证。Nginx 仅监听回环准备端点，健康页 200、其他路径 503。

原 compat.91 镜像在无网络、无生产配置的临时 SQLite 容器通过版本与健康测试，之后回收。该测试不是生产数据的 MySQL 业务验收。既有发布/PITR 工具按 ef7157370 部署，未运行 release init，三个新机 PITR timer 均 disabled/inactive。

相同原连接下，旧 rclone v1.60.1-DEV 复现 501；与生产 SHA 一致的 v1.75.0 完成小流式、文件和80 MiB multipart上传及完整解密读回。systemd PATH/EnvironmentFile 及旧 PITR 证书读回验证通过；旧客户端保留。确认客户端兼容问题得到解决，但具体不支持的请求/头未定位，不回填历史生产故障原因。

只读配置校验器以规范化 Compose JSON 检查无应用/端口/internal 网络及固定挂载契约，拒绝额外服务、危险权限、可变镜像和自动创建挂载目录，不输出配置、环境或 Docker stderr。本机与 Linux 十一项失败回归、真实 Compose 门禁通过；现有 release-safety CI 纳入同一检查。运行态仍需实际检查，工具明确输出 runtime_verified=false。Ubuntu Compose 2.40.3 实查会在 JSON 路径展开 env 文件并以 bind:{} 表达 false；门禁支持该规范化形态，全文仅在进程内处理，不以 no-env-resolution 作为保密边界。CI 使用临时模板副本及 dummy env，覆盖新旧结构与秘密不回显；首次现场拒绝的诊断保留，含展开凭据的临时 JSON 已移除。

生产三容器身份、启动时间和重启计数与开始时相同，业务健康。原备份、归档、业务数据、配置和 DNS 未修改。详细范围、目录、镜像、验证及后续条件见 [环境准备记录](../../../../docs/installation/ovh-migration-preparation.md)。

## Consequences

准备就绪不代表已同步业务数据或可切换；空库不能通过备份的非空业务表门槛。初始化脚本只在空 datadir 运行，持久只读由 mysqld-auto.cnf 加载，后续播种必须显式管理它，不能靠改 cnf 绕过。配置门禁不能验证宿主符号链接、权限、真正运行状态或 R2 内容。

迁移要保留应用目录、SESSION/CRYPTO 密钥、附件路径及全部写入入口，切换和接收新写入后的回滚另行实施。新机 R2 写入验证不能替代新源的全备/PITR 恢复验收。准备阶段仅运行两个空依赖和回环 Nginx；源码与证据保留，停止本阶段新机服务即可回退，无需改动旧生产。
