# Agent Note: 使用日志筛选常驻与紧凑统计

Status: implemented

## Problem

日志页需要先展开筛选才能编辑条件；统计块字号较大，标题、说明和已应用范围占据多行。用户已在本地预览确认筛选常驻、五项统计缩小并下移，以及去掉标题和说明文字，现在要求同步项目并提交。

## Decision

复用日志页面、表单与 useLogsData；完整筛选表单始终挂载且可见。顺序为工具栏、筛选、13px 数字统计摘要、结果；不显示页面标题、筛选说明、统计说明及已应用范围文字行。工具栏保留独立列设置、普通用户摘要/完整模式、桌面紧凑切换及管理员汇总/榜单；查询使用实色按钮，刷新为次级操作。

统计提取为只负责展示的 LogsStats 组件，保留五项指标的真实值、未知值、加载态和错误重试。只改变展示；查询提交、重置、分页、刷新、统计、导出、榜单和汇总仍使用原已应用条件快照，普通用户/self与管理员权限、计费和日志内容不变。样式限定日志页，使用现有 Semi 颜色变量，不依赖下载包的全局设计系统。

## Existing capabilities and notes

- 页面与 hooks → router/api-router.go 的日志、自身日志、统计、导出、榜单与分组汇总 → controller/log.go、controller/log_export.go → model/log.go、model/log_scope.go、model/log_export.go。服务端权限和统计计算完整复用。
- [原日志布局与查询一致性](../../implemented/feature/2026-10-28-usage-log-experience.md)：部分替代标题、折叠筛选和可见范围的展示决定；保留全部查询快照、失败状态和渠道时间边界。
- [派生视图](../../implemented/feature/2026-10-29-model-detail-log-view.md)：保持普通用户摘要/完整模式及列偏好。
- [日志导出](../../implemented/feature/2026-10-17-usage-log-export.md)、[渠道用量](../../implemented/feature/2026-09-29-channel-usage-in-management.md)、[厂商分组](../../implemented/feature/2026-10-21-inception-vendor.md)：仍成立，不改数据和权限契约。
- 看板与视频相关改动独立于本任务；不从预览或原工作区引入。

## Alternatives considered

- 只默认展开保留折叠按钮改动更少，但保留了用户明确不要的隐藏入口，因此筛选常驻。
- 继续展示已应用范围可减少草稿与结果混淆，但用户明确要求去掉该文字区；仅移除可见副本，保留内部快照和提交查询行为。
- 直接引入下载包的全局 console.css 可复制所有主题规则，但会改变其他页面且超出本次日志页范围；使用日志页局部样式与现有颜色变量。

## Verification

`bun test scripts/usage-log-query.test.mjs scripts/usage-log-view.test.mjs scripts/usage-log-content.test.mjs` 共16项通过；修改的JSX通过 ESLint，五个源码文件通过 Prettier，完整 `bun run build`（含生图子应用和静态预渲染）通过。未新增镜像实现的断言测试。

项目源码使用隔离合成 API 在浏览器验收：管理员筛选常驻，统计位于下方；提交模型后再输入新草稿，刷新仍显示已提交模型；统计失败隐藏数字并保留可调用的重试，重置恢复数字和默认条件。列设置弹窗可打开；普通用户无管理员工具，摘要/完整切换正常。390px浅色和深色模式下筛选与五个指标换行正常，无页面横向溢出。截图保留在本地验收产物中，未引入预览接口或数据。

基于公司主线 eb6d3c461 独立分支完成，排除原工作区未提交改动。未实测真实数据库、生产账号或真实CSV下载；这些服务与查询调用未改动。

## Consequences

去掉已应用范围后，正在输入的草稿与当前结果范围不再有独立文字对照；仍需点击查询应用条件。筛选常驻在手机上占用更多行。不迁移真实数据或生产配置。
