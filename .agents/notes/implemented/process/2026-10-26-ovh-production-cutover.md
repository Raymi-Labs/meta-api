# Agent Note: OVH 同版本全机迁移与单写入源切换

Status: implemented

## Problem

NAN 磁盘无法扩容，用户已通知维护并授权将业务整体迁入已准备的 OVH。空依赖、R2 可写和旧源 PITR 演练都已验收，但不能据此直接启用新应用。生产还包含文档站、LibreSpeed、私有 WireGuard/Pulse API 与日志库连接，以及可能固定旧源 IP 的反代入口。

## Decision

保留当前 compat.91 镜像、单一业务库及全部秘密配置语义，先导入已验证全备 B，再按准确文件/位置同步增量。源机在准备期间继续服务；目标持久只读，新 UUID 不变。导入使用单连接关闭会话 binlog，失败恢复只读，不删除源备份、日志或业务表。原始全备、校验与私有操作回执保留。

应用迁移复用受保护发布工具，新增仅创建固定镜像容器的 migration-create 和排空后停止指定容器的 migration-stop；共享发布锁，不借迁移升级应用、不拉取可变镜像、不重启依赖。先在真实隔离 MySQL 中验证生产 schema 的两次应用启动，再允许创建实际应用。停止旧应用前关闭新请求，排空 API 与私有入口；冻结源库、记录最终位置，追平并核对财务和表结构后才允许新库写入和新应用启动。

保留 SESSION/CRYPTO 密钥及应用文件，迁移文档、测速站、证书与自动续期。用户明确同意新机安装 PHP。Pulse 私网和反代实际入口必须一起核实、切换并验证；DNS 变更或首页成功都不等于所有入口完成迁移。目标新写入开始后不得直接恢复旧源写入，回退必须先将新增数据追平回唯一恢复目标。

新机使用独立 UUID 的全备/归档链，不复制旧 checkpoint；正式开启前验证锚点与连续链，并保留原备份任务与完整旧源。恢复验收必须区分旧源已完成演练与新源尚待验证的状态。

## Existing notes audit

- [环境准备](../../implemented/process/2026-10-26-ovh-migration-preparation.md)：部分重叠，沿用环境，不把准备事实改写成已切换。
- [发布安全](../../implemented/process/2026-10-21-release-entrypoint-safety.md)与[快速恢复](../../implemented/process/2026-10-21-fast-container-restart.md)：部分重叠，扩展其受保护入口，保留依赖不重启的约束。
- [PITR](../../implemented/process/2026-10-23-mysql-pitr.md)：部分重叠，沿用锚点、校验、恢复与告警实现。
- [反代迁移](../../implemented/architecture/2026-10-11-qiyun-reverse-proxy-migration.md)：部分重叠，业务源站变化要求复核仍指向旧源的入口。
- 其他检索命中的桌面、渠道、账务与界面决定：无关，不改变其行为。

## Alternatives considered

- 停服后才重新制作和导入全量备份：操作链短，但全库恢复耗时会扩大停机窗口；优先在线播种并追平。
- 复制整个 MySQL datadir 或实例级无过滤复制：能保留更多实例状态，但会复制 UUID、系统账户或预检临时库，破坏目标身份和权限；保持已验证业务范围。
- 迁移同时升级应用或拆日志库：可合并工作，但增加 schema、余额和恢复语义变化，不利于定位与回滚；本轮保持生产版本。

## Verification

发布与恢复包65项本机回归通过，迁移停止14项Linux失败/中断测试通过。真实MySQL 8.0.46验证只读导入且本地binlog不变、业务ROW/DDL复制、preflight与账户排除、源UUID及最终执行位置。OVH使用源schema-only文件和原生产镜像完成两轮启动及九项停止/创建检查，依赖身份保持不变。Redis 7.4.9的2162键测试覆盖绝对过期、多DB、RDB内部校验、旧空AOF错误、AOF重建后的强制终止恢复与正常重启。

这些结果证明迁移工具与隔离流程，不能代替生产切换验收。执行中的播种、连续追平、最终停写对账、新源全备/PITR及公网/Pulse切换另存私有回执；操作说明见[同版本生产迁移](../../../../docs/installation/ovh-production-migration.md)。

## Shared lock directory failure and regression

迁移创建触发了共用发布锁的既有缺陷：`install -d -m 700 "$(dirname "$LOCK_FILE")"` 会修改已经存在的目录，而默认锁文件位于公共 `/run`。现场目标 `/run` 在 17:10:42 UTC 变为 0700，导致非 root 的 resolved/networkd 被拒绝访问。源机也存在历史遗留的 `/run` 0700，resolved/timesyncd 从 10 月 15 日起失败；固定公共 DNS 暂时掩盖了解析故障，不能据此认为系统服务正常。现场另行将两端 `/run` 恢复为 0755，目标网络服务恢复，源机时间同步恢复；业务容器未因这个修复重启。

共用锁只为不存在的私有目录创建 0700 权限，已有目录的权限、所有者和 inode 保持不变。路径检查拒绝直接父目录或锁文件软链接、可被其他用户写入的目录/锁文件、硬链接和特殊锁文件；系统祖先别名先规范化，再检查实际目录的所有者和写权限。锁以不截断的方式打开，继续保留原 inode 与共享 flock 语义。已受影响主机的公共目录需要单独核实并修复；发布工具不猜测目录用途或自动 chmod 系统目录。

回归调用实际 Bash `acquire_lock`，用隔离的 0755 `run` 目录复现权限问题，并覆盖缺失私有目录、已有锁内容与 inode、锁冲突、软/硬链接、FIFO、可写祖先和路径越界。本机发布工具76项回归及Linux独立锁目录11项回归通过，笔记校验通过。仅检查锁文件创建成功无法发现此缺陷；验收必须断言公共父目录权限保持不变。其他全备/日志归档的 Python 共用锁不 chmod 父目录，私有目录 helper 对不符权限的既有目录直接拒绝。

完整 Linux CI 还暴露了迁移中断测试自身的竞态：fake Docker 直接截断重写 `runtime.json`，并发轮询偶尔读到空 JSON。测试替身改为临时文件加原子替换，模拟 Docker inspect 返回完整状态；真实入口、信号发送点及“保留开始证据、不自动恢复写入”的断言不变。修复后 Linux 非 root 完整 76 项回归、两个中断场景各 15 轮共 30 项重复检查及本机 14 项迁移回归通过。

## Final schema serialization equivalence

最终冻结对账中，源与目标的行指纹、行数、财务汇总及日志覆盖范围全部一致，但 73 张表的旧结构指纹不同。逐表读取完整 `SHOW CREATE` 并绑定原始审计的 raw DDL 指纹，再比较 information_schema 的 76 表、1100 列、457 索引行及约束、引用和分区元数据，确认有效结构完全一致：导入后的列定义只是多写了与明确 COLLATE 对应的冗余 `CHARACTER SET utf8mb4`。

共用 `pitr_audit.normalize_create` 通过保留引号和注释字节的 lexer，只折叠代码区中紧邻显式同字符集 COLLATE 的冗余 CHARACTER SET；排序规则本身和真实字符集差异仍保留。默认值、注释、enum/set、生成列表达式中的字符串及反引号标识符不能被替换，原 AUTO_INCREMENT 高水位规范化也限定在代码区。拒绝直接忽略 schema hash 或做全局字符串替换；它们可能掩盖真实结构或业务默认值的变化。

Linux 34 项恢复审计回归及真实隔离 MySQL 8.0.46 证明：等价字符集序列化与有效列元数据相等，真实 collation 变化仍不等，五类受保护字符串保持原样。生产只增加只读结构核验，不执行 DDL。原始失败报告保留；新的结构等价收据绑定双方原报告 SHA、完整 DDL/有效元数据证据及 normalizer 代码 SHA，派生报告仅重算 schema 指纹，原行数据/财务/计数保持不变后完成离线比较。

## Pulse private MySQL socket recovery

切换后 WireGuard 已握手，但 Pulse 日志数据库连接收到 EOF。只读排查确认：目标 Docker 29 的 internal bridge 上，MySQL 虽显示发布到 `127.0.0.1:3306`，该宿主机端点实际拒绝连接；容器内部地址的 MySQL 握手正常。原 `systemd-socket-proxyd` 固定转发到这个不可达的 loopback 端点，因此不能靠 WireGuard 握手或容器健康判断日志读取恢复。

保留既有 `pulse-logdb.socket` 的私网监听，改为 `Accept=yes` 与每连接模板，调用 [受保护端点解析器](../../../../scripts/server-migration/pulse-logdb-proxy.py)。解析器仅从本机 Docker socket 读取容器名称、固定镜像 ID、运行状态、唯一 internal bridge 和 `/opt/newapi/mysql_data` 的读写 bind mount；每次重新取当前 IPv4，并确认它属于网络的私有子网。检查失败关闭该连接，不输出数据库配置；不读取 Env，不保存密码，不执行 Docker 修改。容器重建导致 IP 变化时无需改固定地址，但 MySQL 镜像、数据目录、网络名称或隔离方式变更必须同时审核解析器的白名单并重跑连接验收，不能只修改部署配置。

拒绝硬编码容器 IP，也不为修复私网读取而放宽数据库网络或发布公网端口。数据传输复用 `socat`，systemd 将 accepted socket 交给 fd 0，使用 `FD:0,shut-down` 与 `StandardOutput=null`。隔离 Linux 实测发现 `STDIO`、`STDIO,shut-down` 及不带 shut-down 的 `FD:0` 都不能向客户端正确传播后端半关闭，因此保留双向二进制流及两种半关闭回归，避免只测连接或请求发送成功。

隔离 Linux 10 项通过，涵盖镜像/挂载/网络拒绝、动态 IP、双向各 2 MiB 全字节数据以及客户端/后端半关闭；CI 显式安装 socat，并再次运行该模块，不能以跳过真实传输测试视为通过。目标仅安装 socat、替换代理 socket/template/script；部署前后 MySQL 容器 ID、StartedAt 和 RestartCount 不变，未重启应用或数据库。2026-10-26 19:14:49 UTC 从 Pulse 经新 WireGuard 连续两次取得 MySQL 8.0.46 握手，连接实例正常退出；随后使用现有 Pulse 配置的实际 access-check 通过，日志库可读、账号只读、两条授权未扩大。握手检查不等于授权验收，二者分别保存私有回执。

原 socket/service 与部署前状态已私有备份。代理回退仅在暂停 Pulse worker 并排空私网数据库连接后恢复备份的 units、daemon-reload 和重启该 socket；不重启 MySQL 或 NewAPI。旧 loopback 端点在当前 internal 网络上仍不可用，因此回退旧代理只恢复配置，不能宣称功能恢复；失败时保留 worker 暂停，修复并通过真实只读 access-check 后再继续。新目标开始写入后，这个代理回退不允许将数据连接重新指回旧数据库。

## Private socket boot ordering

最终代码审查发现日志库和私有 API socket 均默认排在 sockets.target 前，却显式等待 WireGuard/Docker；这些 service 默认排在 basic.target 后，而 basic 又等待 sockets.target，因此手动启动成功仍不能证明重启安全。只修改 WantedBy 不会移除 socket 的隐含 Before，不能作为修复。

两份 socket 使用 DefaultDependencies=no，显式等待 WireGuard/Docker、位于 multi-user.target 前，并保留关闭阶段的 Before/Conflicts shutdown.target。监听地址、Accept 和代理传输语义不变。只替换依赖配置后 daemon-reload 与不带 --now 的 reenable 即可更新安装链接，保留正在运行的监听和连接；不能只 enable 留下旧链接，也不能 disable --now 中断服务。

真实 Linux systemd-analyze 在临时根目录完成三项测试：固定私网监听和激活契约、新依赖无环、旧依赖与仅改 WantedBy 均检测到 ordering cycle。systemd 可能删除 wanted job 打破循环后仍返回零，因此回归检查实际诊断而非只看退出码。CI 显式安装并检查 systemd-analyze，不能把测试跳过写作通过；该验证不等同生产重启演练。

2026-10-27 北京时间 03:35:12，目标按已测试文件摘要热更新两个 socket，仅 daemon-reload 和 reenable；旧 sockets.target 安装链接已移除，两个私网监听保持 active/enabled，应用、MySQL、Redis 的容器 ID、启动时间与重启次数均未变化。目标宿主机 systemd-analyze verify 通过，未主动重启生产主机做开机演练；操作回执保留在私有目录，不入仓库。

## Redis semantic snapshot continuity

Redis 7.4.9 的 zset 超过 listpack 阈值后会保留 skiplist 编码，即使成员随后减少；RDB 重载可重新选择 listpack。直接比较 DUMP 因此可能拒绝逻辑内容相同的恢复结果。v2 清单仍记录原 DUMP 指纹，但仅对 zset 增加按有序成员和分值、长度前缀编码的逻辑指纹；其他类型维持原 DUMP 比较，类型、完整数据库/键集合和绝对过期时间的门禁不变，不放宽 nonce 有效期。

旧 v1 没有逻辑指纹，不能用目标重采样替代原始证据。`upgrade` 必须绑定此前记录的 v1 文件 SHA，在仍冻结的源端重新采样，先以原 DUMP、类型、绝对过期及完整范围证明连续性，再生成独立 v2 与 continuity 回执。已有 RDB 只有在源连续性及目标 v2 校验均通过时才可复用；原 v1 和 RDB 不改写。拒绝忽略所有 DUMP 差异、手工补造旧逻辑指纹或重置目标 TTL，因为这些做法会掩盖丢键、值变化和防重放期限延长。

真实隔离 Redis 测试使用 2162 个键，复现 skiplist 缩小后 RDB 重载为 listpack，验证旧 v1 拒绝、新 v2 在源连续性成立后接受；单独篡改 score/member 仍失败。多数据库、自然过期、nonce/亲和状态、空 AOF 错误优先加载、AOF 重写后强制终止恢复及正常重启继续验收。旧清单 SHA 错误时不访问 Redis，连续性失败不生成新清单；私有清单非阻塞打开后拒绝 FIFO/特殊文件，格式损坏只返回固定诊断，不输出值或 traceback。

## Consequences

恢复包固定收集已核实的 Compose、.env/.env.db/.env.app 与三份数据库/缓存配置，拒绝链接、特殊文件及复制期间变化；不递归整个config或业务data。此处补齐环境格式变更带来的恢复缺口，不把数据库部署包称为完整主机备份。用户允许PHP安装，测速站需独立验证实际PHP与下载路径。

长时间流式请求、异步任务和支付重试可能跨维护窗口；仅检查网页或容器健康不足以确认完成。原入口可暂时作为转发过渡，但不能把仍依赖旧机报告为全机退役。GTID OFF 的位置复制需要验证过滤规则，不能让源系统账户 DDL 进入新机。最终验收保留未覆盖的实付支付与真实退款样本边界。
