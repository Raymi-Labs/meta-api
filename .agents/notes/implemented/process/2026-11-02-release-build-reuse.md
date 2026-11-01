# Agent Note: 发布共享前端产物并稳定依赖缓存

Status: implemented

## Problem

v1.14.1同提交触发main镜像、正式双架构镜像、Linux附件与Windows桌面，前端共构建5次。正式AMD64镜像Go依赖下载289.5秒，而ARM64仅24秒；Docker在依赖层之前声明每次变化的提交/分支参数，导致依赖缓存随版本失效。正式发布8分14秒，Windows8分13秒。用户要求减少重复构建与等待。

## Decision

- release.yml作为正式发布编排入口，先构建一份与源码commit/版本绑定的前端+文档产物，再由Linux附件、双架构镜像、Electron复用。保留所有既有产物、固定SHA Actions、版本/架构校验与只补缺失附件规则。
- 镜像和Electron改为可复用工作流，保留workflow_dispatch手动补包；未提供共享产物时沿用源码构建，历史标签仍可补包。发布工具从当前受信工作流提交独立检出，不要求旧标签包含新工具。
- Docker保留默认源码构建，正式CI可选择预构建前端阶段。版本/提交参数移到依赖下载之后；GitHub托管CI显式使用Go官方代理和校验服务，保留用户自定义代理入口。
- main仅在VERSION变化且短时等待内确认正式tag精确指向当前提交时跳过重复镜像构建；正式镜像发布同提交sha标签，受保护更新地址保持可用。未发布/标签指向其他提交/手动main构建仍执行原流程。
- 构建流程不修改生产备份、数据库、线上应用或安装包矩阵；用真实Actions验证时长与完整产物。

## Existing notes audit

- [Linux优先发布](../../implemented/process/2026-10-21-linux-first-release.md)：部分重叠，保留产物/更新入口，扩展源码构建为可复用前端。
- [公司Actions](../../implemented/process/2026-10-27-company-actions-policy.md)：继续固定获准源SHA、PR和按架构缓存。
- [Electron公司发布](../../implemented/bug-fix/2026-11-01-electron-company-actions.md)：继续完整安装包及仅补传规则，改变自动触发为父发布工作流调用。

## Alternatives considered

- 只删除Windows、ARM或主分支镜像：可以省时，但缺少已有产物/更新地址，不采用。
- 多个独立工作流跨run轮询寻找前端：保留触发方式，但引入来源与并发竞态；使用同run依赖图与不可变artifact。
- 只增加Docker cache mount：临时GitHub执行器的mount不会自动进入registry层缓存，不能声称跨run有效；先稳定依赖层缓存并消除重复前端。
- main一律跳过VERSION提交：标签可能未创建，sha镜像会丢失；仅精确tag确认后复用正式构建。

## Verification

- 29项发布回归通过，包含产物commit/版本/哈希、缺失入口、路径穿越/链接拒绝、精确tag去重、网络失败/超时回退及既有Linux/Windows只补附件规则。
- 自动发布只有一份前端构建，保留两架构镜像、Linux程序、文档/管理包及Windows两种exe/校验文件。
- 正式sha标签与版本清单相同；普通main和旧标签手动入口仍可工作。
- 完整前端、文档和5页Chrome预渲染通过；真实静态产物打包/恢复以及嵌入Go构建通过。策略/actionlint及笔记校验通过。本机Docker daemon未运行，Docker图检查加入PR CI；真实发布与耗时需在GitHub完成后核实，不能用静态配置声称提速。

## Consequences

- 共享前端失败会阻断下游，这属于所需共同输入失败；不悄悄用旧包。
- 首次新缓存需要预热，平台排队/网络仍影响速度，不保证固定分钟数。
- 手动旧版本源码与新工具解耦，不能改变旧源码和既有Release资产。

官方语义依据：[Docker ARG 与缓存失效](https://docs.docker.com/reference/dockerfile/#impact-on-build-caching)、[GitHub 可复用工作流](https://docs.github.com/en/actions/how-tos/reuse-automations/reuse-workflows)。
