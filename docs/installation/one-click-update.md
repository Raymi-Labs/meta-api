# 一键更新与回退

生产环境不要直接长期运行可变的 `latest` 标签。`newapi-update` 会先拉取
`latest`，确认它对应 GitHub `main` 的最新提交，再将 Compose 固定到不可变的
`sha-xxxxxxx` 镜像。

## 安装

```bash
install -m 755 scripts/newapi-release /usr/local/bin/newapi-release
ln -sfn /usr/local/bin/newapi-release /usr/local/bin/newapi-update
ln -sfn /usr/local/bin/newapi-release /usr/local/bin/newapi-rollback
newapi-release init sha-上一稳定版本
```

默认部署目录为 `/opt/newapi`。可通过 `NEWAPI_APP_DIR` 等环境变量覆盖脚本顶部的配置。

## 更新最新版本

```bash
newapi-update
```

更新流程包括：部署锁、最新提交校验、完整 MySQL 备份、临时数据库迁移预检、
固定 SHA 部署、有超时上限的健康检查、发布状态原子切换和失败自动恢复。
备份和迁移预检期间旧服务保持在线，不要提前停止应用。更新开始时检查源容器
和状态接口；已离线时在拉取/备份前报错，应先恢复服务或使用 `newapi-rollback`，
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
覆盖备份、预检、部署、健康检查失败，三种中断信号，离线更新拒绝及离线回退：

```bash
python3 -B -m unittest discover -s scripts/server-release -p 'test_*.py' -v
bash -n scripts/newapi-release
bash -n scripts/server-release/retired-entrypoint.sh
```

这类运维工具独立安装，不需要切换应用镜像。生产验收应核对安装前后业务容器
ID/启动时间、状态接口、鉴权边界和退役入口的拒绝行为；不能为测试恢复逻辑
而主动停止正在服务的生产容器。
