# OVH 同版本生产迁移

本流程将 NAN 的 New API、MySQL、Redis、应用文件、文档站、LibreSpeed 和 Pulse 私网连接迁入既有 OVH。它不升级应用、不拆日志库、不删除源备份或 binlog。环境准备见 [准备记录](ovh-migration-preparation.md)，恢复协议见 [PITR](mysql-pitr.md)。实际切换状态以当次私有验收回执为准，工具和演练成功不代表生产已切流。

## 已核对的基线

- 应用固定为生产 compat.91 镜像摘要；MySQL 8.0.46、Redis 7.4.9，唯一业务库 `new-api`，76 张 InnoDB 表。
- 保持 GTID OFF、ROW/FULL/CRC32、持久化 1/1、七天 binlog；目标使用不同 UUID/server_id，准备与追平期间保持持久双只读。
- 源应用的 SESSION/CRYPTO 及 Pulse 签名密钥保持一致；目标 MySQL/Redis 使用自己的运行凭据，配置在 root 私有环境文件中，不写入仓库。
- Pulse worker 的日志连接只需要 `SELECT new-api.logs`；Pulse 自身数据库留在原 Pulse 主机，权益/余额变更继续走 New API 私有 API。
- 主站反代有固定源 IP，nan/ai/docs 的 DNS 也须分别处理。只改 DNS、仅测试首页或仅看到容器健康都不能作为完整切换验收。

## 在线播种和追平

`scripts/server-migration/mysql-target-seed.py` 仅在已准备的新目标本机执行。它绑定明确的源/目标 UUID、远端 SUCCESS、manifest SHA、压缩文件 SHA/完整性及 dump 内锚点，再与此前完整恢复的 `reference_audit` 比对全部表。目标必须为空、无复制通道；导入仅关闭 SRO，保持 `read_only=ON`，单连接 `sql_log_bin=0`，失败恢复 SRO，部分数据保留且禁止自动重跑或 DROP。

导入被中断时，通过连接 ID 和本次会话标记确认并取消实际 MySQL 连接，不能只终止本地 `docker exec`。客户端与服务端使用 256 MiB 包上限；新 UUID、系统库与本地 binlog 保留，不运行 RESET MASTER。

增量使用临时受限 SSH 身份及复制专用账号。隧道只绑定目标 Docker 私网网关，源 MySQL 不向公网开放。口令不出现在 argv/日志；MySQL 原生复制口令长度不得超过32字节。连接先启动 IO 线程并核实真实源 UUID，再启动 SQL 线程，固定文件/位置、单线程应用。

过滤使用 `REPLICATE_DO_DB=new-api`。ROW 按实际表所属库过滤，语句级操作按默认数据库过滤，不能用表通配符推断账户 DDL 也被排除。[MySQL 官方过滤规则](https://dev.mysql.com/doc/refman/8.0/en/replication-options-replica.html) 因此还要求先用 `audit-source-binlog.py` 对锚点至固定终点做事件 CRC、连续性、事务边界和 QUERY 范围审计，并在追平期间冻结发布和人工 DDL。审计脚本按事件流式读取活跃文件的已提交前缀，不输出 SQL。

动态复制过滤在重启后不保留；目标必须保持 `skip_replica_start=ON`。发生重启要重新检查身份、持久状态及过滤，再显式续跑，不能自动启动或跳过复制错误。

## 停写与应用切换

1. 在源站所有回源应用的路径返回真实503，包括 GET 支付回调、webhook、任务通知和 docs 公开 API；Pulse 私有3000也停止接受新连接。静态文档与测速可继续服务。
2. Nginx reload 后等待老 worker、SSE/WebSocket 和3000已有连接自然排空。不得因为新请求已经503就杀掉仍在计费的旧请求。
3. 通过 `newapi-release migration-stop CONTAINER_ID IMAGE_ID --ingress-frozen` 停源应用。该入口共用发布锁、两次确认入站连接为零，保存原重启策略私有收据，先将自动重启设为 `no` 再停止；只有退出0、无 OOM 且策略为no才通过。失败和中断不恢复旧写入者。
4. 确认应用退出、无剩余写事务，再将源 MySQL 设为持久双只读并记录最终坐标。目标使用 `SOURCE_POS_WAIT` 到达最终位置后停复制，核对实际执行位置和 IO/SQL 错误，不以接收位置或延迟数字代替。
5. 核对账务、日志及文件最终增量。切换同一 WireGuard 身份之前停止旧隧道，再更新 Pulse 对端 endpoint；验证实际日志连接和私有 API，而非只看握手。
6. 目标 `migration-create IMAGE_DIGEST SCHEMA_SHA256` 只创建已有固定镜像容器，不启动、不拉取、不重启依赖。解除目标只读并用 `newapi-restart` 启动同一容器，真实健康后执行 `newapi-release init` 建立新主机发布状态。
7. 恢复目标 API 入口，切换已核对的反代 upstream 和源站 DNS；旧源只作缓存过渡桥接，不恢复旧应用或旧库写入。逐项验收静态文件、PHP、API、鉴权、Pulse、日志和异步任务。

最终停写对账使用 `mysql-final-audit.py`，绑定独立恢复 B、目标完整播种和最终连续 binlog 审计的指纹；源/目标均须双只读、无写事务，目标须停复制且执行位置精确匹配。它核对76表结构、非logs全部行/财务、B之后logs行/财务及全表与旧前缀数量。旧logs前缀不再次扫描哈希，数量相同不能独立排除旧行修改，其正确性依赖已完成的B全审计和唯一复制写入链。该限制保存在私有报告中，不能称为最终全库重新哈希。

源暂停前须核对 `BATCH_UPDATE_ENABLED`；批量更新没有可假定的退出前统一 flush。应用启动后会运行任务轮询、额度维护和结算，即使公网仍是维护页也可能产生新写入。

## Redis 状态

Redis 包含 Pulse 防重放 nonce、限流、渠道冷却和 Responses 亲和状态，不能一律当作可丢缓存。`redis-migration-audit.py` 在写入者停止后保存键摘要、类型、DUMP摘要和绝对过期毫秒；不输出键名、值或凭据。它针对已核实的小型实例设有键数/字节上限，不应无条件用于大型在线 Redis。

源 manifest → 导出最终 RDB → SHA256与 `redis-check-rdb` → 再验源 manifest。目标先关闭自动重启、停止空 Redis 并归档原数据目录，防止两个进程同时使用数据目录。新目录放入RDB，使用同版本隔离临时容器、原配置并仅在命令行覆盖 `appendonly no` 载入；核对manifest后开启AOF并等待重写成功，再停止临时容器。正式Redis按永久AOF配置启动、再次核对manifest，恢复预期重启策略。自然过期允许，原TTL不得延长。

只将RDB放进已有空AOF目录会被Redis忽略。真实测试包含此失败、AOF重建后的强制终止恢复和正常重启；切换前仍须核对两机时钟同步。

## 恢复资产和回退

发布恢复包包含 Compose、既有 `.env`，以及存在时的 `.env.db`、`.env.app`、`config/mysql.cnf`、`config/redis.conf`、`config/99-preparation-readonly.sql`。固定清单在dump前拒绝链接、特殊文件或变化，副本0600、子目录0700；最终封存仍覆盖隐藏文件与后写入元数据。不会递归打包整个config、datadir或任意外部挂载。

配置副本含秘密，必须加密异地保存。附件、Redis、Nginx/TLS/WireGuard、管理员告警与解密材料仍是独立恢复资产；数据库部署包没有覆盖它们。空盘初始化SQL会保持SRO，恢复数据后需要按受控切换显式解除，不能仅启动应用绕过保护。

包内文件统一root私有权限；还原到运行路径时要恢复服务读取权限：当前镜像的Redis配置为UID999/GID1000、0600，MySQL非秘密cnf与初始化SQL为0644，环境文件仍root0600。数据库单库备份不含MySQL账号；Pulse reader 的持久凭据索引是Pulse主机 `/opt/meta-pulse/.env`，只重建已核实的表级读取权限，不导入整个mysql系统库。

新应用启动前可撤销切换，恢复旧源原重启策略和写权限，再用受保护入口恢复服务。新应用启动后按已经产生新写入处理：先冻结目标、保全新增 binlog，完成回传与一致性核对后才可恢复旧库。禁止直接改回 DNS 或重启旧写入者。

新目标建立独立 UUID 的全备和归档链，不复制旧 checkpoint。原任务和旧备份保留；新源实际全备加连续binlog恢复、账务比对与管理员邮件验证完成后，才可称新生产PITR已验收。旧机退役另行执行。

## 验证命令

```bash
python3 -B -m unittest discover -s scripts/server-release -p 'test_*.py' -v
python3 -B -m unittest discover -s scripts/server-migration -p 'test_*.py' -v
NEWAPI_MIGRATION_DOCKER_TEST=1 python3 -B -m unittest discover -s scripts/server-migration -p test_mysql_target_seed.py -v
NEWAPI_REDIS_MIGRATION_IMAGE=redis@sha256:6ab0b6e7381779332f97b8ca76193e45b0756f38d4c0dcda72dbb3c32061ab99 python3 -B -m unittest discover -s scripts/server-migration -p test_redis_migration.py -v
make verify-notes
```

真实MySQL与Redis测试只使用其创建的隔离容器。生产镜像与schema的两轮启动演练另用 `scripts/server-release/exercise-migration.py`，需显式 `--execute`，无生产挂载、无对外端口。CI覆盖Linux失败/中断、实际复制过滤、Redis持久化恢复与既有PITR恢复测试。
