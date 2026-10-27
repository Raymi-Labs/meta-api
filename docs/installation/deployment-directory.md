# Meta API 部署目录

公司部署的实际目录为 `/opt/meta-api`；`/opt/newapi` 保留为指向它的兼容符号链接。
两条路径访问同一份数据，不是两套部署。日常管理使用：

```bash
cd /opt/meta-api
meta-api status
meta-api-update --dry-run
```

发布工具默认选择新目录，只有旧目录存在时兼容旧部署。两套独立目录同时存在会报错，
必须核对后显式设置 `NEWAPI_APP_DIR`；不要把两个不同数据库目录合并覆盖。

## 迁移边界

本次只移动宿主机目录的名字，不复制或修改业务数据。Compose 必须显式保留
`name: newapi`；容器、数据库、Docker 网络名称与既有 bind mount Source 保持原身份。
Docker 中仍显示 `/opt/newapi/...` 是正常的兼容记录，后续容器启动通过旧链接访问新目录。
不需要为了显示名称去重建 MySQL/Redis，也不要删除兼容链接。

历史备份的绝对路径、`LAST_BACKUP_DIR`、发布状态、校验清单和归档链保持原值。
Nginx 静态文件和 Pulse 私网代理继续通过有效旧路径读取；既有私网代理的挂载身份白名单
不放宽。持续 binlog 归档只接受固定、受信任的 `/opt/newapi -> /opt/meta-api` 迁移别名，
其他符号链接、错误目标、可写目录和不可信归属仍然拒绝。

## 已部署服务器的迁移

使用本版本完整源码中的 `scripts/server-release/migrate-deployment-directory.py`。
管理压缩包不会自动移动数据库目录。先核对服务器部署、备份服务及静态文件路径，保存
现有脚本；把同版本 `scripts/newapi-release` 和 `scripts/server-storage/mysql-binlog-archive.py`
分别安装到 `/usr/local/bin/newapi-release` 与 `/usr/local/lib/newapi-storage/mysql-binlog-archive.py`。
核对已有相邻备份 helper 与本版本一致；安装必须使用临时文件后原子替换，并保持 root 归属，
不得修改正在执行脚本的 inode。

```bash
# root，从相同版本的完整源码目录运行。默认仅核对。
python3 -B scripts/server-release/migrate-deployment-directory.py
python3 -B scripts/server-release/migrate-deployment-directory.py --execute
```

工具仅针对 `/opt/newapi` 与 `/opt/meta-api`，共用发布锁，核对三个运行中的容器、显式
Compose 项目名、已安装兼容脚本、目录/数据 inode、发布状态及健康接口。Linux
`renameat2(RENAME_EXCHANGE)` 一次交换目录与预置链接，使旧路径没有缺失窗口；不支持时
报错并清理本次临时链接，不退化为有缺失窗口的两次移动。原目录是挂载点或新路径被占用时拒绝。

操作前后记录写入 root 私有的 `/root/meta-api-directory-*`。有失败记录时先检查路径和
容器状态，不能只因目录已经存在就认定迁移成功。应用升级/回退仍使用原受保护入口。

完成后验证新旧路径同 inode、三个业务容器启动时间未变、`meta-api status` 显示新目录、
更新/回退预演成功，且至少两轮 binlog 定时归档继续推进、PITR 监控无新失败。
本轮不因目录改名触发新的全量备份或真实业务启停。

未在此服务器安装的历史日志退役/迁移脚本不能直接照旧命令恢复执行；其通用安全检查仍拒绝
含链接的根目录，需要单独审核新绝对路径与既有回执根目录，不能手改回执跳过保护。

## 隔离验证

```bash
python3 -B -m unittest discover -s scripts/server-release -p 'test_deployment_directory.py' -v
python3 -B -m unittest discover -s scripts/server-storage -p 'test_mysql_binlog_archive.py' -v
# Linux root，仅创建随机命名的 MySQL 测试容器、独立临时目录，无网络/生产挂载；不拉取镜像。
python3 -B scripts/server-release/exercise-directory.py --execute --fixture-image mysql:8.0.46
```

真实演练验证迁移前后写入、相同 inode/UUID、旧/新路径下归档 Source 的读取，以及仅对隔离
容器重启后数据仍可访问。临时容器与临时数据清理，不操作生产 MySQL。
