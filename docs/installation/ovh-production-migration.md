# OVH 同版本生产迁移

本流程将 NAN 的 New API、MySQL、Redis、应用文件、文档站、LibreSpeed 和 Pulse 私网连接迁入既有 OVH。它不升级应用、不拆日志库、不删除源备份或 binlog。环境准备见 [准备记录](ovh-migration-preparation.md)，恢复协议见 [PITR](mysql-pitr.md)。实际切换状态以当次私有验收回执为准，工具和演练成功不代表生产已切流。

## 本次切换完成状态与运维入口

2026-10-27 北京时间 03:13:57，公共三入口首次验收恢复。New API、MySQL、Redis、文档站和测速服务已迁至 OVH `147.135.39.173`；应用仍使用 compat.91 固定镜像，本次没有升级应用或创建新应用 Release。

| 用途 | 当前入口与边界 |
| --- | --- |
| 新生产运维 | SSH 别名 `basedate-us-west-1` 或 `newapi-prod`，使用 `ubuntu` 登录并通过 `sudo` 操作；没有新增 root 登录 |
| 应用发布、同版本恢复和回退 | 在新生产主机使用受保护的 `sudo newapi-update`、`sudo newapi-restart`、`sudo newapi-rollback`，继续遵守其参数、锁和验收要求 |
| 旧主机 | `NAN` / `NAN-legacy` 仍指向 `104.168.114.150`，仅作只读保留及过渡桥接，不再向其部署；旧机退役须另行验收 |
| Pulse | 自身数据库和 worker 仍在原 Pulse 主机，仅私网连接改接新的 New API 数据源，不能写作 Pulse 整体迁入 OVH |

本次 Pulse 验收时连接和摄取已恢复，但仍有约 15 小时历史摄取积压及 period close 失败，单独跟进；连接检查通过不代表历史积压或结算问题已经消除。具体私有验收回执和秘密配置不写入仓库。

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

## Pulse 私网入口与开机顺序

日志库使用 `pulse-logdb.socket` 的 `10.77.0.1:3306`，每连接模板通过受保护的 `pulse-logdb-proxy.py` 核对 MySQL 容器、固定镜像、数据目录和 internal 网络，再转发到当前容器地址。私有 API 使用 `pulse-newapi-api.socket` 的 `10.77.0.1:3000`，保留既有 API 代理服务。两者都只监听 WireGuard 私网地址。

两份 socket 显式设置 `DefaultDependencies=no`，在 WireGuard 与 Docker 之后、`multi-user.target` 之前启动，并保留 `Before=shutdown.target` 和 `Conflicts=shutdown.target`。不能同时保留 socket 默认的 `Before=sockets.target` 与 `After=docker.service`：`sockets.target → basic.target → docker.service → socket` 会形成启动循环。仅修改 `WantedBy` 不能解除这个循环。

当监听地址和 Socket 节配置未变、只是修复 Unit/Install 依赖时，核对新文件后执行 `systemctl daemon-reload` 和 `systemctl reenable pulse-logdb.socket pulse-newapi-api.socket`，**不加 `--now`，也不重启 socket、代理、worker、应用或数据库**。`reenable` 用于移除旧 `sockets.target.wants` 链接并建立 `multi-user.target.wants` 链接；只运行 `enable` 可能留下旧链接，`disable --now` 则会中断监听。现有 accepted 连接保持原服务实例。

读回须确认两个 socket 仍为 active、ListenStream 与 Accept 未变、After 包含 WireGuard/Docker、Before 不含 sockets.target 且含 multi-user/shutdown、WantedBy 仅为 multi-user，并核对旧安装链接已移除。随后从 Pulse 做真实只读日志连接与私有 API 检查。`test_pulse_socket_units.py` 使用 Linux `systemd-analyze --root` 在临时根目录验证依赖，同时复现旧依赖及仅改安装目标时的循环；这是隔离启动图验证，不宣称重启过生产主机。

## Redis 状态

Redis 包含 Pulse 防重放 nonce、限流、渠道冷却和 Responses 亲和状态，不能一律当作可丢缓存。`redis-migration-audit.py` 只核对 Redis 7.4.9，绑定完整容器 ID、镜像 ID 和运行状态。采样前必须停止全部写入者，并保持冻结直到 RDB 导出和核对完成；目标也不能在验收前接入业务。工具逐库执行只读 Lua，不输出键名、成员、分值或凭据；每库最多 10,000 个键、64 MiB 审计数据，适用于已核实的小型冻结实例，不应无条件用于大型在线 Redis。

### 清单版本与 zset 编码

`snapshot` 默认生成 v2。两个版本都检查完整数据库范围、键集合、类型和 `PEXPIRETIME` 绝对过期毫秒；额外键、未到期键丢失、类型变化和任何过期时间变化都会失败。仅允许键在目标观察时刻已经自然到期而消失。切换前仍须核对两机时钟同步，不能用迁移后的剩余 TTL 重新设置过期时间，否则会延长 nonce 或亲和状态的有效期。

| 清单 | 值校验规则 | 遇到编码变化 |
| --- | --- | --- |
| v1 | 所有类型严格比较原始 `dump_sha1` | 即使类型和 TTL 相同也拒绝；不自动降级为语义比较 |
| v2 | zset 按 `ZRANGE ... WITHSCORES` 返回的有序成员和分值，以长度前缀编码生成 `value_sha1`；其他类型仍严格比较 DUMP | 只有 zset 逻辑指纹、类型和绝对 TTL 全部一致时，才允许 DUMP 不同 |

Redis 可以把超过 128 个成员后缩小的 zset 保持为 skiplist，而从 RDB 载入时选择 listpack。底层编码变化会改变 DUMP 字节，但不必改变成员和分值。v2 同时保留原始 `dump_sha1` 与指纹算法标记；通过语义校验的编码差异计入 `zset_encoding_changes`。成员或分值变化仍拒绝，不把所有 DUMP 差异一概视为安全。

### 已有 v1 的连续性升级

旧 v1 没有记录逻辑指纹，不能从目标状态补造旧快照的证据。若本轮已有 v1，只能在**仍冻结且身份未变的源 Redis** 执行 `upgrade`：先核对原 v1 文件与此前记录的 SHA-256，再用新采样的原始 DUMP、类型、绝对 TTL、数据库与键集合严格核对 v1，成功后才写入独立 v2 文件。新文件的 `continuity` 记录旧清单 SHA、版本和连续性核对结果。

如果已经按 v1 导出并校验 RDB，可在同一冻结窗口保留原 RDB 及文件指纹，完成源端连续性升级后，再以 v2 核对恢复目标；两项都通过才能继续。升级不改写原 v1 或 RDB。

下面变量必须使用已经复核的不可变容器/镜像 ID、私有路径及此前保存的清单 SHA，不能把故障后修改过的 v1 重新计算 SHA 当作可信基准。清单须属于执行用户、为非符号链接的私有普通文件；新清单以 0600 创建且不覆盖已有文件。

```bash
# 在仍冻结的源端连续升级旧证据；使用已核实的认证来源，不传密码。
python3 scripts/server-migration/redis-migration-audit.py upgrade \
  --container "$SOURCE_REDIS_NAME" --container-id "$SOURCE_REDIS_ID" \
  --image-id "$SOURCE_REDIS_IMAGE_ID" --auth-command \
  --prior-manifest "$V1_MANIFEST" --prior-sha256 "$V1_RECORDED_SHA256" \
  --manifest "$V2_MANIFEST"

# 将已验证的 v2 安全复制到目标后，核对尚未接入写入者的恢复实例。
python3 scripts/server-migration/redis-migration-audit.py verify \
  --container "$TARGET_REDIS_NAME" --container-id "$TARGET_REDIS_ID" \
  --image-id "$TARGET_REDIS_IMAGE_ID" \
  --auth-config /usr/local/etc/redis/redis.conf --manifest "$V2_MANIFEST"
```

`--auth-command` 适用于源凭据在 `Config.Cmd` 的 `--requirepass` 参数中的情况；配置文件或环境变量模式分别使用 `--auth-config`、`--auth-env`，不把密码写入命令行。不要在目标运行 `snapshot` 作为新的源基准，也不要改写旧 v1 或手工填入并不存在的逻辑指纹。源端 DUMP 连续性已无法证明时停止升级，重新建立受控源快照和 RDB 证据；不能降低原清单的验证规则继续切换。

### 最终 RDB 和 AOF 验收

1. 冻结源端写入者，保存 v2（或按上述方式升级已有 v1），导出最终 RDB；核对文件 SHA-256、`redis-check-rdb`，再对源端执行同一清单的 `verify`。文件完整性和逻辑一致性两项都要通过。
2. 目标先关闭自动重启，停止尚未接入业务的 Redis 并归档原数据目录。使用新目录载入 RDB，保证只有一个进程使用该目录。仅将 RDB 放入已有空 AOF 目录会被 Redis 忽略，不能据此认定导入成功。
3. 使用同版本、`--network none` 且无对外端口的隔离临时容器，保留永久配置的 `appendonly yes`，仅用进程参数覆盖为 `appendonly no`，先加载 RDB 并执行 v2 `verify`。
4. 在该临时实例启用 AOF，等待 `aof_enabled=1`、`aof_rewrite_in_progress=0`、`aof_rewrite_scheduled=0`、`aof_last_bgrewrite_status=ok`、`aof_last_write_status=ok` 且 `aof_current_size>0`，再停止临时实例；不需要改写永久配置。
5. 正式 Redis 按永久 AOF 配置启动，确认 AOF 已启用、持久化策略符合预期，再次执行 v2 `verify`，通过后恢复预期自动重启策略。保留 RDB、v1/v2 清单及验证回执，之后才接入业务写入者。

真实隔离测试覆盖多 DB、nonce、亲和状态、绝对过期、skiplist 缩小后 RDB 重载为 listpack、成员/分值篡改拒绝、v1 连续性失败、旧空 AOF 错误、AOF 重写后的强制终止恢复和正常重启。中断用例只证明重写完成后的恢复，不能外推为未完成重写或任意时刻断电都不丢数据。

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
