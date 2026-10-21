# Agent Note: Linux 优先发布与按需跨平台附件

Status: implemented

## Problem

生产部署使用 Linux Docker 镜像，但每个应用标签仍触发 Linux、macOS、Windows 程序构建。compat.50 至 compat.52 的 macOS ARM64 作业因 GitHub 执行器容量不足在排队约 15 分钟后取消；其中 compat.51、compat.52 两次重试都未进入编译，后补附件也不会让原 Actions 记录转为成功。另一个耗时来自 ARM64 镜像：两个构建阶段随目标架构运行，Bun、Chromium 和 Go 编译在 x86 构建机上经过 QEMU。

## Decision

- 保留应用标签的 Linux AMD64/ARM64 程序附件和双架构镜像；macOS、Windows 移到独立的手动作业，选定已发布标签和一个平台后按需补充附件。绑定解析后的提交，跳过已完成附件，不重写发布说明或 draft 状态。
- Docker 前端及 Go 编译阶段固定 BUILDPLATFORM；现有 CGO_ENABLED=0 配合 TARGETOS/TARGETARCH 交叉编译，运行时镜像仍使用目标架构。每个镜像实际检查 ELF 架构和版本，防止只改标签而放入错误架构程序。
- 保留主分支 SHA 镜像流程，受保护更新入口依赖其不可变版本映射；不为减少一次构建而改变线上更新协议。按架构隔离构建缓存；版本和 latest 清单使用本次版本的架构镜像。
- 发布仅生成产物，不触发生产升级。桌面客户端独立工作流不属于本次服务端程序包调整。

现有笔记审计：[受保护发布入口](../../implemented/process/2026-10-21-release-entrypoint-safety.md)部分相关但管理生产切换，本篇管理 CI 产物，两者独立保留；[项目流程](../../implemented/process/2026-09-29-project-decision-notes.md)及工具包分发不改变。未找到持有多平台应用发布选择的现行笔记。

## Alternatives considered

- 只重试 macOS：配置变化最小，但实际两次排队都失败，且服务器不消费该附件，不能继续作为默认发布依赖。
- 只关掉 Mac/Windows：能消除非必要任务，但 ARM64 的模拟编译仍会让镜像耗时较长。
- 将 ARM64 改到原生托管执行器：不改变编译方式，但引入另一个执行器资源池依赖；现有 Go 镜像已禁用 CGO，直接复用 x86 构建机交叉编译更合适。
- 删除所有独立程序包或主分支镜像：任务最少，但改变现有 Linux 下载和更新入口契约，不在本轮范围内。

## Testing

- 发布策略检查、9 项手动发布边界回归、bash 语法和 actionlint 通过。Linux/可选附件流程的 checkout 与 setup-go 更新到当前 v7，避免旧 Node 运行时被 actionlint 拒绝。
- 本机使用 CGO=0、GOOS=linux 和两种 GOARCH 交叉编译通过，检查 ELF 机器类型，并在隔离的双架构最小容器中执行 `-version`，均返回 compat.53。复用已有前端构建文件，本轮没有前端代码变更。
- 本机完整 Docker 构建在安装 Chromium 时受 Docker Desktop 内存限制失败；它不证明应用构建失败，也不计为完整镜像验收。正式镜像流程必须重新构建前端、Go 与运行时层，在各平台实际验证程序后才能合并清单。
- 发布验收以 compat.53 的 Linux 附件、双架构镜像及手动 Windows 作业实际结果为准；另验证重复补包跳过构建机。macOS 不属于本轮必需产物，无需为验收排队。生产没有在本轮部署。

## Consequences

服务器发布不再等待闲置用途的平台资源，ARM64 编译阶段不再经过模拟器。跨平台用户需要手动请求附件；手动作业仍可能遇到该平台容量不足，但不阻塞服务器发布。交叉编译必须保持 CGO=0，未来增加依赖目标架构原生编译的组件时需重新审查。首次独立缓存需要预热，构建耗时必须以实际运行记录为准；当前单架构 SHA 镜像协议继续保留。
