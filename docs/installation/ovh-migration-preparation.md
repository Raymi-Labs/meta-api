# OVH 迁移环境准备

本阶段只准备新主机，不切换生产流量。NAN 仍是唯一生产写入源；新机数据库为空、持久只读，不能把环境就绪写成数据迁移完成。准备配置位于 [scripts/server-migration](../../scripts/server-migration/)，既有 [PITR](mysql-pitr.md) 和受保护发布入口继续复用。

## 已准备的环境（2026-10-26）

| 项目 | 状态 |
| --- | --- |
| 系统与存储 | Ubuntu 26.04.1；三个 RAID 1 阵列均 `[UU]`；准备后根分区约 3% 使用、约 1.6 TiB 可用；约 59 GiB 可用内存。这是当次快照，不是持续容量承诺。 |
| 主机防护 | 保留现有密钥登录、UFW 默认拒绝入站、SSH 三次失败封禁策略；本轮不修改 SSH、UFW 或 DNS。 |
| 容器工具 | Docker 29.1.3；新增 Ubuntu 包 Compose 2.40.3。 |
| MySQL | 固定生产实际 MySQL 8.0.46 镜像，独立 UUID / `server_id=14713539`；`new-api` 空库、零业务表，两个只读开关均为 ON。 |
| Redis | 固定生产实际 Redis 7.4.9 镜像；新独立密码、空缓存，AOF/everysec，未复制生产会话或租约状态。 |
| New API | 固定旧机 compat.91 镜像，已预取；无网络、无生产配置的临时 SQLite 容器通过启动及 `/api/status`，随后回收。未创建长期业务容器。 |
| Nginx | Ubuntu 包 1.28.3；仅监听 `127.0.0.1:8080`，`/healthz` 返回准备状态，其他路径 503。尚未安装生产站点、证书或开放公网 Web 入口。 |
| R2 | 原 `r2-backup` crypt 连接已就绪；小文件流式、文件上传、80 MiB multipart 均完整解密读回匹配。 |
| 备份与发布工具 | 复用源码 `ef7157370` 的发布/PITR 工具；三个 PITR timer 均 disabled/inactive，没有新全备、归档 epoch、告警状态或发布 state。 |

固定镜像：

- New API：`ghcr.io/nanashiwang/new-api@sha256:f401c43ce6358ca92beb98985696f7b357eb462373c6c7850c2ca1713fa204d4`
- MySQL：`mysql@sha256:7dcddc01f13bab2f15cde676d44d01f61fc9f99fe7785e86196dfc07d358ae2b`
- Redis：`redis@sha256:6ab0b6e7381779332f97b8ca76193e45b0756f38d4c0dcda72dbb3c32061ab99`

这组摘要来自当次生产检查；若旧机后来发布新版本，播种及切换前重新核对，不能自动拉取 `latest`。

## 目录与保护边界

准备 Compose 安装为 `/opt/newapi/docker-compose.yml`，只有 `mysql`、`redis` 两个服务；project/network 为 `newapi` / `newapi_default`，容器名为 `newapi-mysql` / `newapi-redis`。网络设为 internal，两种依赖均无 `ports`。现有防火墙之外，现场确认了 Docker 无端口发布、宿主无数据库监听；外部 IPv4 探测 80/443/3000/3306/6379/8080 均未连接成功，22 可连接。

真实 bind mount 使用 `/opt/newapi/mysql_data`、`redis_data`。应用预留 `newapi_data` → `/data`、`newapi_logs` → `/app/logs`，与源机一致；后续附件、桌面更新及其他实际文件均需按源容器挂载复制，不能只迁数据库。保留 `/data` 路径语义，避免数据库中的发票绝对路径失效。

`/opt/newapi/.env.db` 仅放新目标的 MySQL 凭据，root/0600；Redis 密码位于私有 `config/redis.conf`，仅容器 Redis UID 可读。准备凭据不替代生产 `SESSION_SECRET`、`CRYPTO_SECRET` 或 Pulse 签名密钥。含凭据的运行时配置、展开后的 Compose、原始 inspect 环境和 R2 解密材料不得提交。

MySQL 首次空目录初始化的最后一个脚本执行 `SET PERSIST_ONLY read_only=ON` 和 `super_read_only=ON`：临时初始化服务器可完成建库，最终服务器及后续启动加载持久只读设置。已实际验证写入被 1290 拒绝、零业务表；重新创建两个依赖容器后 UUID 不变、双只读仍为 ON、健康检查通过。该初始化脚本不会在已有数据目录再次执行，因此不能只看文件判断实际保护状态。

MySQL 持久设置保持 GTID OFF、ROW/FULL、CRC32、事务持久化 1/1、七天 binlog；事件调度器关闭，复制线程不自动启动。字符集/排序规则、事务隔离、SQL mode 和 151 连接上限与源机保持一致。新机初始 buffer pool 为 16 GiB，容器内存上限 24 GiB；Redis maxmemory 为 1 GiB/noeviction、容器上限 2 GiB。这是准备预算，迁入真实负载后再验收，不声称性能已调优。

## R2 写入兼容性

相同原连接配置下，旧 `/usr/bin/rclone` v1.60.1-DEV 的小流式上传复现 HTTP 501；官方 v1.75.0 二进制与生产 SHA-256 一致，三种上传和完整解密读回通过。已安装为 `/usr/local/bin/rclone`，原客户端保留。具体不支持的请求或请求头尚未定位，不将该结论延伸为旧生产备份故障的确切原因。

新二进制 SHA-256：`f3f9aff817f9766029e50adf9a7963c169e475b8f10c7927823568a0d9443db7`。systemd 默认 PATH 及原 EnvironmentFile 已实际验证；新机还能独立解密读取上一轮 PITR 验收证书，指纹一致。

连接位于 `/root/.config/rclone/`（0700，两个文件0600）；离机恢复材料继续独立保管。唯一测试前缀 `r2-backup:migration-readiness/20261026-fb7113eeb8374ae4a0a0d41f106290a8` 保留，无旧备份删除或改写。此处证明新机存储读写可用，不证明新源 PITR 已成立。

## 校验入口与证据

```bash
# 使用临时模板副本和 dummy env 检查，不启动容器。
NEWAPI_PREPARATION_COMPOSE_TEST=1 python3 -B -m unittest discover \
  -s scripts/server-migration -p 'test_*.py' -v
# 已准备主机上检查实际配置，全文仅在进程内解析。
python3 -B scripts/server-migration/check-preparation.py \
  --compose /opt/newapi/docker-compose.yml
```

Ubuntu Compose 2.40.3 的 JSON 路径即使传 `--no-env-resolution` 也可能展开 env 文件；该开关不是保密边界。校验器只在进程内处理两种规范化结构，不回显原文，不能手动打印完整 JSON。`bind: {}` 是该版本对 false 的省略表示，显式 true、缺少 bind 对象及额外挂载属性仍被拒绝。校验器拒绝应用服务、额外服务、端口发布、外部网络、危险容器权限、可变镜像及错误挂载；输出不含配置或凭据。它不检查磁盘符号链接/权限、实际监听、数据库只读或 R2 内容，摘要明确 `runtime_verified=false`，不能替代现场验收。

本机及 Ubuntu 26.04 的十一项失败边界测试、实际 Compose 检查、安装脚本语法检查、MySQL 设置/可观测性查询、容器重建、Nginx 配置及回环响应、R2 实际传输均通过。CI 在现有 Release tooling safety 流程增加准备配置门禁。新机证据位于 `/root/newapi-migration-prep-20261026/`，工作站副本位于用户私有 `.config/newapi-backup/migration-prep-20261026/`；秘密不在本报告中。

准备完成后的生产核对：New API/MySQL/Redis 的容器 ID、启动时间、重启次数均与开始时一致，原应用仍为 compat.91 且健康。没有生产配置修改、DDL/DML、停服、备份或日志清理。

## 下一阶段的必要条件

1. 将带精确锚点的全备安全播种至永久目标，并设计连续追赶和重试；现有 PITR 临时恢复器不能直接接管已有目标或续跑。导入前显式管理目标的持久只读设置，导入后恢复保护；仅修改 cnf 不能覆盖 `mysqld-auto.cnf`。
2. 再核对源版本、全部挂载、站点/证书、其他服务及 IP 白名单。业务文件先同步一轮，最后停写窗口再次同步；不能复制正在运行的 MySQL 数据目录。
3. 将生产应用配置作为私有迁移材料处理；保持应用隔离，核对所有写入来源、长请求和内存批处理队列，避免两套独立数据库同时接收业务写入。
4. 演练最终位置追平、业务检查和回退。接收新写入后不能直接把 DNS 改回旧库；先解决新增事务的回传或补偿。
5. 新机从自己的 UUID、新全备锚点建立独立 PITR 链，完成真实恢复、RPO 和邮件验收后启用定时器。旧链、旧备份和旧机仍保留；不复制旧 checkpoint 或发布 state 冒充新机状态。

准备阶段回退只涉及新机：停止本次两个依赖及准备用 Nginx 即可，保留数据目录、配置和证据；无需改变 NAN。不要执行 `down --volumes`、删除数据目录、开启新机应用或解除只读来“验证”。应用正式启动及发布仍须适配受保护入口，不另写停服改库脚本。

参考：[MySQL 持久变量](https://dev.mysql.com/doc/refman/8.0/en/persisted-system-variables.html)、[Compose internal 网络](https://docs.docker.com/reference/compose-file/networks/#internal)、[决策记录](../../.agents/notes/implemented/process/2026-10-26-ovh-migration-preparation.md)。
