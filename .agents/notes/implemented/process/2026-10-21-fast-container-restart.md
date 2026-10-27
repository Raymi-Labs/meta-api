# Agent Note: 同版本容器快速恢复入口

Status: implemented

## Problem

用户在应用被停止后尝试 `newapi-restart` 无法恢复。源站 PATH 与 root 交互式 Shell 均没有该命令或别名；现有正式发布工具只提供更新、回退、初始化和状态查询。更新需要健康源容器及备份/预检，回退会改变版本，均不能充当快速同版本恢复入口。

## Decision

在现有 `scripts/newapi-release` 内增加 restart 子命令并安装 `newapi-restart` 链接，共用发布锁。对既有容器按实际状态调用 Docker start 或 restart，保留容器 ID、镜像、环境和挂载；不依赖 Compose、MySQL 备份和远程镜像拉取。缺失/暂停等不支持的容器状态明确失败，不猜测版本重新创建。输出操作、等待进度、成功或失败原因，失败返回非零；支持只预览。

恢复健康以容器 ID/镜像一致、当前启动时间稳定及状态接口成功为准，连续两次通过后成功。避免复用更新健康检查中的累计重启次数为零和全历史致命日志条件，历史故障不应阻止本次恢复。Docker 调用和健康等待均设超时。记录操作结果，不改发布版本状态。

## Existing notes audit

- [发布入口保护](../../implemented/process/2026-10-21-release-entrypoint-safety.md)：部分重叠；复用锁和正式工具，新增同版本恢复契约，不翻转在线更新/退役决定。
- [存储维护](../../implemented/process/2026-10-17-server-storage-maintenance.md)：相邻决定；继续共用发布锁，不改变清理策略。
- SSH/安全/业务监控笔记：无关；不变更权限和业务接口。

## Alternatives considered

- 直接提供 `docker restart` 别名：最短、足够操作容器，但没有发布互斥、超时后的明确结果和应用健康验收；使用正式工具封装这些边界。
- 使用 compose up 或触发更新：能够创建缺失容器，但可能应用未部署配置、切换镜像并运行迁移；本入口仅负责既有容器恢复。
- 复用旧健康函数：减少代码，但累计重启次数和历史日志会错误否决已恢复的同一容器；另设恢复验收。

## Verification

- 本地 macOS 和服务器 Linux 的25项隔离回归均通过（含子用例），覆盖更新原有恢复、真实命令名和 restart 子命令、停止/运行中恢复、历史重启计数、错误健康响应、持续崩溃、缺失/暂停容器、发布锁、超时、非法参数和 dry-run。完整流程使用命令替身，不代表生产故障演练。
- 源站真实 Docker 演练使用已缓存 Bun 镜像的独立 HTTP 测试容器，无挂载、独立 bridge 网络、仅回环端口；创建后启动、运行中重启、手动停止、SIGKILL 退出137后的恢复、锁冲突、预览均通过，同一容器/镜像保持不变。临时容器和网络全部清理；演练脚本为显式执行入口，不在日常回归中自动操作 Docker。
- 服务器正式工具已原子安装，`/usr/local/bin/newapi-restart` 指向 `newapi-release`，真实命令 dry-run 通过。主应用/MySQL/Redis 容器 ID、镜像、启动时间、运行状态和重启次数安装前后相同；主应用仍为 v1.0.0-compat.48，发布状态 synchronized。
- 安装备份、哈希/容器状态核对和演练记录保存在源站 `/root/newapi-restart-safety-20261021-rN8ZVo/`。新工具与仓库源码 SHA-256 一致。Bash 语法、演练 CLI、差异检查和 `make verify-notes` 通过；故障回归接入现有 release-safety CI。


## Consequences

恢复命令补齐既有发布工具的离线恢复入口，启动只操作当前容器并明确显示结果，避免借升级流程长时间等待。运行中的容器重启会中断其连接；Docker 或数据库底层故障不由恢复入口自动修复。此命令不保证 SIGKILL、断电下的脚本日志完整性，也不自动创建已删除容器或启动依赖服务。只验证当前部署可提供状态接口，不代表上游模型请求全部正常。

后续扩展见 [公司更新源与管理命令](2026-10-27-server-management-cli.md)：保留本篇原有边界，独立安装公司来源配置及管理工具；meta-api 包已获授权公开，不修改 Actions 允许名单、SHA 固定和主分支 PR 保护。
