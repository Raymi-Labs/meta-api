# Agent Note: 公司更新源与 Meta API 服务器管理命令

Status: implemented

## Problem

业务服务器仍从个人仓库获取更新，只提供 newapi-update/restart/rollback。用户要求更新资源切换公司仓库，并通过 meta-api 菜单或 meta-api-update 等命令操作。公司镜像此前私有，服务器无登录配置，实测读取返回 unauthorized；用户明确选择允许公司公开 Packages 并公开 meta-api 镜像。

## Decision

- 复用 scripts/newapi-release 的更新、恢复和回退，不另造停服再更新流程。新增 meta-api 菜单和快捷名，旧命令保留；普通 sudo 用户可直接输入 meta-api，由固定安装路径提权。
- 镜像与 GitHub 来源通过 root 管理的 /etc/meta-api/release.conf 配对设置；仅解析两个允许的键，不执行配置内容，不重写旧部署目录、容器名或回滚状态。原环境变量显式覆盖仍可用于隔离演练。
- 增加共享发布锁下的 stop，必须明确确认，仅停止已核实 ID 的应用；停止后检查同一 ID/镜像和终态，并记录结果。start 对运行中容器不重启；恢复仍检查健康。保留停服后不能直接更新的既有规则。
- 公开公司镜像后，服务器以无凭据方式验证镜像读取；本轮仅安装工具和切换更新源，不实际更新/停止/重启生产业务。完整生命周期只在隔离容器验证。公司 Release 增加管理脚本/配置/说明压缩包及 SHA256，安装工具独立于应用升级。

## Related notes audit

- [公司仓库迁移](../../implemented/process/2026-10-27-company-repository.md)：部分重叠，本篇承接此前明确留出的生产更新源迁移。
- [快速恢复](../../implemented/process/2026-10-21-fast-container-restart.md)：部分重叠，保留恢复契约，扩展 start/stop 与菜单。
- [受保护发布入口](../../implemented/process/2026-10-21-release-entrypoint-safety.md)：继续适用，共享锁、在线备份与预检不变；stop 不用于更新的前置步骤。

## Alternatives considered

- 单纯 shell alias：最省代码，但没有菜单、统一安装和停止确认；保留原引擎，增加薄命令入口。
- 复制本机个人 GitHub 凭据到服务器：可以访问私有镜像，但扩大个人凭据暴露面；用户选择公开已有公开源码的镜像，无需服务器凭据。
- 全量修改 new-api 容器/目录/内部命名：外观一致，但破坏已部署配置和上游保护约定；新增命令名，保留现有资源身份。

## Verification

- 87 项发布工具回归通过，覆盖公司来源配对、非法/不安全配置、PTY 菜单确认/取消、错误参数、启动幂等、停止结果校验、锁冲突和超时。
- 业务服务器上使用独立 Bun HTTP 测试容器、随机网络和回环端口完成真实 Docker 演练：创建后启动、重启、start 幂等、stop/start、SIGKILL 恢复、锁冲突和预览均通过，容器/镜像身份保持不变，测试资源已清理；未改动的 MySQL 快照查询使用独立替身，不访问生产数据库。
- 公司组织允许公开 Packages，meta-api 镜像页面显示 Public；服务器匿名读取 compat.100 双架构清单成功。root 安全安装后状态显示公司 GitHub/GHCR 来源；应用、MySQL、Redis 的 ID/镜像/启动时间、Compose 和发布状态校验一致，应用继续运行 compat.97。安装备份在服务器 `/root/meta-api-management-20261027-064244/`。
- 服务器真实 PTY 菜单查看状态与取消停止通过；meta-api-update 预演已从公司源解析 compat.100 固定摘要，业务仍在线。
- 13 项发布附件回归、Bash/工作流语法、发布策略及笔记检查通过；完整 Release 产物以对应 Actions 完成和附件/镜像核实为准。

## Consequences

stop 会中断现有请求，交互操作须确认；手动停止的容器在 Docker daemon/主机重启后可能按原 restart policy 启动。公开包以后可被匿名下载；本轮用户已明确授权，其他包可见性不改变。

后续目录统一见 [部署目录迁移](2026-10-27-deployment-directory.md)：实际目录改为 /opt/meta-api，保留 /opt/newapi 兼容链接及容器/网络/回滚身份；菜单和公司来源继续复用。
