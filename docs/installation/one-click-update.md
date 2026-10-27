# 一键更新、回退与恢复

生产环境不要直接长期运行可变的 `latest` 标签。`newapi-update` 会先拉取
`latest`，确认它对应 GitHub `main` 的最新提交，再将 Compose 固定到不可变的
`sha-xxxxxxx` 镜像。

## Meta API 管理入口

公司服务器安装后，直接输入 `meta-api` 打开菜单：状态、更新、启动、重启、停止、回退。
普通 sudo 用户由固定的 `/usr/local/bin/meta-api` 提权；交互变更操作会询问确认，空输入默认取消。
也可直接使用：

```bash
meta-api status
meta-api-update                 # 等同 meta-api update
meta-api start                  # 已运行时只检查健康，不重启
meta-api restart                # 同版本恢复/重启
meta-api stop                   # 确认后仅停止应用
meta-api rollback               # 使用已记录的上一镜像
meta-api-update --dry-run       # 拉取和解析目标，但不部署/备份
meta-api-stop --dry-run         # 只预览
```

`meta-api-start`、`meta-api-restart`、`meta-api-stop`、`meta-api-rollback` 为对应快捷入口。
自动脚本停止须显式 `meta-api stop --yes`。所有变更共用原发布锁，仅操作已核实 ID 的应用；
MySQL/Redis 保持运行。stop 会中断请求，不是更新的前置步骤；停止后先 start 恢复在线再更新。
stop 保留原 restart policy，主机或 Docker daemon 重启可能使应用自动启动，不适合永久退役。

## 安装

从已验证的公司发布源码或 Release 的 `meta-api-management-<版本>.tar.gz` 解包后安装。
应先备份现有脚本与 `/etc/meta-api/release.conf`，安装期间持有 `/run/newapi-release.lock`，
避免与更新/恢复并发。以下在 root shell 内执行：

```bash
# 复用已有锁文件，不删除锁或更改 /run 权限；若被占用则退出并等维护完成。
exec 9>>/run/newapi-release.lock
flock -n 9 || exit 1
bash -n scripts/newapi-release scripts/meta-api
install -m 755 scripts/newapi-release /usr/local/bin/newapi-release.next
mv /usr/local/bin/newapi-release.next /usr/local/bin/newapi-release
install -m 755 scripts/meta-api /usr/local/bin/meta-api.next
mv /usr/local/bin/meta-api.next /usr/local/bin/meta-api
for command in update start restart stop rollback; do
  ln -sfn /usr/local/bin/meta-api "/usr/local/bin/meta-api-$command"
done
install -d -m 755 -o root -g root /etc/meta-api
install -m 644 -o root -g root scripts/server-release/company-release.conf /etc/meta-api/release.conf.next
mv /etc/meta-api/release.conf.next /etc/meta-api/release.conf
flock -u 9
exec 9>&-
meta-api status
meta-api-update --dry-run
```

配置文件仅允许以下两个键，按数据解析而不执行 shell；文件及其父目录必须安全、由 root 管理，
不能由普通用户写入或使用符号链接。缺失默认文件时保留原个人源兼容行为，显式指定
`NEWAPI_RELEASE_CONFIG` 时缺失/无效配置直接报错；显式 `NEWAPI_IMAGE_REPOSITORY` /
`NEWAPI_GITHUB_REPOSITORY` 环境变量仍优先，隔离演练可使用。

```text
NEWAPI_IMAGE_REPOSITORY=ghcr.io/raymi-labs/meta-api
NEWAPI_GITHUB_REPOSITORY=Raymi-Labs/meta-api
```

公司镜像公开，无需在服务器保存个人 GitHub 凭据。新旧命令读取同一配置；迁移只改变未来
更新目标，不重写当前镜像、Compose、数据库、容器名和回滚记录，旧个人镜像仍可作为回滚目标。
安装工具不等于升级应用。状态输出中的 `update_*_repository` 用于核对实际来源。

旧命令安装方式继续兼容（已有发布状态时不要重新 init）：

```bash
install -m 755 scripts/newapi-release /usr/local/bin/newapi-release
ln -sfn /usr/local/bin/newapi-release /usr/local/bin/newapi-update
ln -sfn /usr/local/bin/newapi-release /usr/local/bin/newapi-rollback
ln -sfn /usr/local/bin/newapi-release /usr/local/bin/newapi-restart
newapi-release init sha-上一稳定版本
```

默认部署目录为 `/opt/newapi`。可通过 `NEWAPI_APP_DIR` 等环境变量覆盖脚本顶部的配置。
恢复包清单和快速恢复需要服务器安装 `python3`；快速恢复另需 GNU `timeout`（coreutils）。

## 应用停止后的快速恢复

```bash
newapi-restart
```

已退出或尚未启动的容器执行 start；运行中或自动重启中的容器执行 restart，
运行中重启会短暂中断连接。操作保留同一个容器的镜像、环境、挂载，
无需拉取镜像、备份数据库或运行迁移预检，也不依赖 Compose 文件/发布状态完整。
它不修改发布版本记录，操作结果追加到发布历史。此入口只恢复既有应用容器，
不重启 MySQL/Redis、不自动创建被删除的容器。

命令先取得与更新/维护共用的锁，显示当前状态和操作；默认给进程30秒退出时间，
Docker start/restart 调用最长等待45秒（另有5秒强制终止客户端宽限），
随后最多120秒检查应用，连续两次健康且启动时间稳定后输出“恢复成功”。
历史重启计数或旧错误日志不会错误阻止恢复。失败返回非零并给出排查命令；
若发布锁被占用，先确认正在进行的任务，不要删除锁文件强行并发执行。
Docker 调用超时只终止客户端，后台操作可能仍在完成，重试前先检查容器状态。

```bash
newapi-restart --dry-run       # 显示将要执行的操作，保持当前运行状态
newapi-release restart        # 等价入口
```

可通过 `NEWAPI_RESTART_STOP_TIMEOUT` 和 `NEWAPI_HEALTH_TIMEOUT` 调整时间上限。
容器缺失、暂停、Docker 不可用或数据库故障会明确报错，需要修复对应原因；
本命令不代替版本回退或数据库恢复。

## 更新最新版本

```bash
newapi-update
```

更新流程包括：部署锁、最新提交校验、完整 MySQL 备份、临时数据库迁移预检、
固定 SHA 部署、有超时上限的健康检查、发布状态原子切换和失败自动恢复。
备份和迁移预检期间旧服务保持在线，不要提前停止应用。更新开始时检查源容器
和状态接口；已离线时在拉取/备份前报错，应先 `newapi-restart` 或使用 `newapi-rollback`，
不能让已停机的服务继续等待长时间备份。回退入口不要求源服务在线。
HUP、INT、TERM 会转为非零退出，并在切换已经开始时尝试恢复旧 Compose、
发布状态及应用。SIGKILL、断电、Docker 卡死和不兼容数据库迁移不受此保障。
如果容器已经是目标镜像，但 Compose 或发布状态被其他命令改乱，更新命令会固定
Compose 并校准回退状态，不会为此重启正常运行的容器。
指定版本时执行：

```bash
newapi-update sha-abcdefg
```

只检查、不修改生产环境：

```bash
newapi-update --dry-run
```

## 恢复包封存与离线校验

数据库转储完成时先生成 `DATABASE_SHA256SUMS`，它只证明 `mysql.sql.gz` 的文件校验值，不表示部署成功或整个恢复包已完整。健康检查、发布状态和部署日志全部完成后，工具再保存最终恢复元数据，并生成覆盖隐藏文件、嵌套文件和最后写入文件的相对路径 `SHA256SUMS`，随即复验。

完整包包括数据库转储、原 Compose、可选 `.env`、原有发布状态、切换前后状态、镜像标识及部署日志。它是应用数据库的逻辑备份和部署恢复材料，不是整个 MySQL 实例或磁盘的备份；账户授权、附件和其他应用数据仍应有独立恢复安排。秘密配置必须经加密后离机保存，解密密码和 salt 独立保管。

封存前失败或中断的目录不生成最终清单，保留可用的转储和诊断材料；不得为了通过保留策略而手工补一个完成标记。应用已经健康运行但封存失败时，命令明确返回失败，不自动回退健康的新版本。旧备份维护仍保留最近三份及 `LAST_BACKUP_DIR`；新清单的 `mysql.sql.gz` 相对路径兼容现有校验器。该保留器只校验转储文件，整包验证仍需执行下面的 `verify-backup`。

从远端下载并解密至一个独立目录后，可离线验证完整文件集合：

```bash
newapi-release verify-backup /实际解密恢复包目录
```

此命令不连接 Docker 或数据库，不要求 root，不修改文件；额外、缺失、变化文件以及符号链接、硬链接、特殊文件均失败。它检查包完整性，不执行数据库恢复，不能代替在隔离 MySQL 中导入全量备份、核对关键账务、验证 binlog 衔接和记录恢复耗时。

旧版本的绝对路径清单不应直接作为新格式的完整恢复包使用。旧备份继续保留，迁移前逐项核对当时的发布结果、转储校验和所需文件；本次工具升级不自动重写旧备份，也不触发新的数据库转储。

## 回退上一版本

```bash
newapi-rollback
```

回退只切换应用镜像，不会自动恢复 MySQL，避免覆盖更新后产生的新业务数据。
数据库备份保存在 `/opt/newapi/backups/one-click/`，仅在确认需要时人工恢复。
如果新版本执行了不向后兼容的数据库迁移，旧镜像可能无法直接运行，此时应先
停止写入，再由管理员评估数据库恢复方案，不能盲目覆盖更新后的业务数据。

回退预演和状态检查：

```bash
newapi-rollback --dry-run
newapi-release status
```

所有更新都应通过这两个命令执行。旧的 `newapi-auto-update.timer` 应保持禁用，
不要再手工修改 Compose 为 `latest`，也不要运行旧的自动更新脚本，避免绕过备份、
迁移预检和健康检查直接覆盖线上版本。

## 已退役入口与验收

源站 `/opt/newapi-cache-release.py`、`/opt/newapi-cache-stage-prepare.py` 和
`/usr/local/bin/newapi-auto-update.sh` 原逻辑封存后，由无副作用的说明脚本替代，
执行返回 78；前两个文件即使通过 `python3` 调用也不会停服、写库或开启演练站点。
配套演练 Nginx 8443 入口停用，源码、SQLite 演练数据、候选镜像保留供审查。
退役阻止已知路径被误执行，不限制 root 故意恢复脚本；不能仅依赖 chmod。

正式更新工具不执行临时渠道 SQL。程序与配置需要联动时，应先设计兼容迁移顺序，
使用与生产一致的数据库引擎、schema 和排序规则验证，再走正式发布流程。
不得将 SQLite 演练或候选分支构建成功当作生产 MySQL 配置迁移通过。

本地/CI 使用隔离目录和 Docker/HTTP 命令替身验证真实 Bash 更新/恢复流程，
覆盖备份、预检、部署、健康检查失败，三种中断信号，离线更新拒绝及离线回退；
恢复入口另覆盖真实命令名、启动/重启、旧故障记录、发布锁冲突、超时、
错误健康响应、持续崩溃、非法参数和只预览：

```bash
python3 -B -m unittest discover -s scripts/server-release -p 'test_*.py' -v
bash -n scripts/newapi-release scripts/meta-api
bash -n scripts/server-release/retired-entrypoint.sh
```

这类运维工具独立安装，不需要切换应用镜像。生产验收应核对安装前后业务容器
ID/启动时间、状态接口、鉴权边界和退役入口的拒绝行为；不能为测试恢复逻辑
而主动停止正在服务的生产容器。

需要真实 Docker 恢复演练时，先确认本机已有 Bun 镜像，再显式运行：

```bash
python3 -B scripts/server-release/exercise-restart.py --execute --fixture-image <本地Bun镜像ID>
```

该脚本只操作随机命名的 HTTP 测试容器及独立网络，无生产挂载，端口只绑定回环地址，
不拉取镜像；覆盖创建后启动、运行中重启、手动停止、SIGKILL 后恢复、锁冲突及预览，
新增菜单入口的 start 幂等性和 stop/start；最后清理测试资源。未改动的 MySQL 快照锁查询在此演练使用独立命令替身，绝不读取生产 MySQL；其真实锁行为由 release-safety 的 MySQL 测试覆盖。命令替身回归与真实 Docker 演练分别验证不同层面的行为。
