# Agent Note: 公司仓库遵循组织级 Actions 与 PR 规则

Status: implemented

## Problem

Raymi Labs 的组织级规则要求 main 通过 PR 合入，Actions 必须使用完整提交 SHA，只允许 GitHub 自有 Actions、oven-sh/setup-bun 等明确来源。仓库初始化完成后，直接推送 main 返回 GH013，尝试启用所有 Actions 返回 409。原发布流程依赖的 docker/* 和 softprops/action-gh-release 不在组织允许范围内。

## Decision

保留组织规则，通过 PR 合入公司迁移。常用发布、主线镜像和关联检查固定获准 Actions 的完整 SHA；Docker 使用 GitHub 执行器自带的 CLI，Release 使用 gh CLI。镜像按现有 alpha 流程已采用的原生 ARM64 执行器构建，避免依赖未获准的 QEMU Action；继续校验程序架构和版本，保留镜像标签、可选 Docker Hub、CGO 与附件契约。保留原自动发布和手动补包入口，补包不覆盖完整附件。

初次 compat.99 标签已导入公司仓库，但 Actions 当时关闭且 main 推送失败，没有把它认定为已发布；保留该标签，不重写，以包含策略适配的下一版本发布。

## Related notes audit

- [公司仓库迁移](../../implemented/process/2026-10-27-company-repository.md)部分重叠，补充实际发现的组织限制及失败记录。
- [Linux 优先发布](../../implemented/process/2026-10-21-linux-first-release.md)部分重叠：保留 Linux 默认矩阵和交叉编译能力，只改变 CI 命令入口和 ARM64 执行器选择；原生执行器容量风险仍保留。

## Alternatives considered

- 放宽组织允许名单和 SHA 限制：原工作流改动最少，但改变公司统一安全边界，不在本次仓库迁移授权范围内。
- 使用管理员绕过主分支规则：无需 PR，但违背公司现行合入流程；使用正常 PR 合入。
- 仅上传源码、跳过发布：迁移最短，但不能验证公司后续发布是否可用，不满足项目发布约定。

## Verification

- 组织配置只读核实：main 需要 PR，所需批准数为 0；仅允许 GitHub 自有与指定来源 Actions，完整 SHA 固定为强制规则。
- 六个相关工作流只引用获准来源和完整 SHA；固定版本经 GitHub commits API 解析。
- 发布策略与 13 项附件回归通过，六个相关工作流 actionlint 检查、笔记与 diff 检查通过；CI 构建继续以对应执行结果为准。

## Consequences

保留组织保护和审批策略，使用正常 PR 合入；公司首版递增为 compat.100，未发布的 compat.99 标签不移动。缓存由 Actions cache 切为按 main/架构隔离的 GHCR buildcache 标签；不依赖 Docker Action 注入缓存运行时凭据。原生 ARM64 执行器可能排队，新仓库需要首次构建；历史标签及未迁移的独立 nightly/alpha/Electron/工具包流程仍受组织限制，不能声称它们已在公司仓库可用，历史版本使用原仓库。
