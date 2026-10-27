# Agent Note: Meta API 部署目录迁移

Status: implemented

## Problem

公司服务器命令已统一为 Meta API，但实际部署目录仍是 /opt/newapi。应用、MySQL、Redis 的 Docker bind mount、旧备份和 Pulse 的身份检查引用旧路径；持续 binlog 归档拒绝任何符号链接数据目录，单纯 mv 后加链接会导致后续备份失败。

## Decision

- 实际目录迁至 /opt/meta-api，/opt/newapi 保留指向新目录的 root 管理兼容链接。使用同文件系统 renameat2 RENAME_EXCHANGE，一次交换实际目录和预置的自指向链接，旧路径无缺失窗口；不复制数据库、不停止或重建业务容器。
- 正式发布工具优先使用新目录，保留旧目录 fallback；两套独立目录同时存在时拒绝隐式选择。显式 NEWAPI_APP_DIR 继续支持隔离/自定义部署。状态显示实际使用目录。
- binlog Source 只额外接受固定 /opt/newapi -> /opt/meta-api 迁移别名，校验 root/当前用户管理的安全父目录、链接与目标，并把配置与 Docker 挂载源归一到同一真实数据目录；任意其他链接继续拒绝。inode、文件归属、容器/UUID/归档链检查保持原规则。
- Compose name:newapi、既有 mounts、发布状态和备份清单保留。私网代理仍检查 Docker 记录的旧 bind source，实际目录经兼容链接可达；旧安装材料仍可恢复。非本机已部署的旧维护脚本须明确新目录参数后使用，不能放宽其通用防链接检查。

## Related notes audit

- [公司管理命令](../../implemented/process/2026-10-27-server-management-cli.md)：部分重叠，本篇承接后续目录统一，不改变菜单和更新源。
- [OVH 切流与私网入口](../../implemented/process/2026-10-26-ovh-production-cutover.md)：保留容器、网络和挂载身份；不重新切流。
- [连续备份](../../implemented/process/2026-10-23-mysql-pitr.md)：归档数据来源须继续核实，本文只补固定目录迁移别名，未更改交易链格式或恢复语义。

## Alternatives considered

- 仅新增 /opt/meta-api 指向旧目录的链接：不影响任何后台路径，但实际部署目录未改名，不能完成用户希望统一的真实目录。
- mv 后创建兼容链接：实现简单，但存在旧路径短暂消失的窗口；Linux 原子交换可消除此窗口。
- 重建应用和数据库、全面替换所有旧命名：Docker 元数据也会一致，但造成不必要停机并改变既有身份边界；保留底层容器名和历史路径别名。

## Verification

- Linux 文件并发读取演练验证无旧路径缺失、原 inode/已打开句柄不变；失败只清理本次自指向临时链接。真实 MySQL 8.0.46 隔离 bind mount 演练完成迁移前后写入、同 UUID/inode、新旧路径归档读取及隔离容器重启后数据检查，测试容器已清理。
- 本地发布工具 91 项回归通过（Linux 原子交换一项在 macOS 跳过，服务器 Linux 四项目录测试全部通过）；存储 221 项回归通过（21 项环境依赖集成用例留给既有 CI）；13 项附件回归、Bash/工作流语法、发布策略及笔记检查通过。
- 首次隔离演练因临时源码归属和无同名 MySQL 标签提前拒绝，未接触生产目录；改为 root 自有临时 helper 及已缓存的确切镜像后通过。一次本地恢复测试受三秒测试窗口影响超时，单项及完整套件重跑通过，生产超时与健康要求未放宽。
- 首轮 GitHub 隔离 MySQL 演练在建表时失败：原就绪检查使用 socket，会把官方镜像初始化阶段的临时数据库误判为最终服务。改用仅在最终服务启动后开放的容器内 TCP 检查，并在失败时保留 SQL stderr 与隔离容器日志；不改变生产数据库连接或超时。
- 修复该检查后，真实目录演练通过；既有内存边界演练随后因随机密码以 `-` 开头被 rclone 识别为选项而失败。三个相关隔离测试使用 `obscure --` 明确参数边界，并让加密集成测试始终覆盖前导 `-`；不修改生产凭据或归档格式。
- 2026-10-27 在公司业务服务器完成原子迁移，记录 `/root/meta-api-directory-5qw9_2g6/`，原脚本与候选源码在 `/root/meta-api-directory-tools-k3v1o6jw/`。应用/MySQL/Redis 的 ID、镜像、启动时间，数据目录 inode/权限/归属，Compose 与发布状态哈希完全一致；应用仍为 compat.97。
- status 显示 /opt/meta-api、release_state=synchronized；公司更新和旧镜像回退预演通过。新旧数据路径下的真实归档 Source 均校验同一 UUID 并可读取 binlog；迁移后归档继续成功推进，PITR 监控正常。

## Consequences

兼容链接不能删除。Docker 显示的旧 Source 和历史备份地址是有效兼容身份，不代表存在第二份数据；不为显示改名而重建数据库。无 renameat2 或跨文件系统时拒绝操作。物理目录回退亦须持相同发布锁，并先保证工具仍兼容旧布局。
