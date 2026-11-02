# Agent Note: 画廊优先生图专用组，规划能力不再决定图片选组

Status: implemented

## Problem

用户已建立价格与稳定性合适的「生图和视频」分组，要求先完善第一个画廊。旧会话入口优先选择同时支持 gpt-image-2 与 gpt-6-sol 的组；专用组只提供图片时会被绕开。画廊的 executeTask 直接调用 Images API，参考图走 edits，查看、收藏、下载使用本地图片，不依赖文本规划。

## Decision

系统管理的图片工作台令牌按「生图和视频→福利→用户组→AutoGroups→其他可用组」选择第一个支持默认图片模型的分组，严格保留权限、倍率存在性与启用渠道检查。不再为了规划能力跳过图片首选组。规划能力在选定图片组之后，按该令牌真实的首选组及 AutoGroups 回退范围检查；范围外的文本渠道不能误开启电商。不改普通 API 令牌、全局 AutoGroups、模型、价格、视频路由或页面布局。已有临时令牌在重新进入会话时刷新，清除过时路由与模型限制，保留 key、到期时间。图片能力查询出错时拒绝刷新，不能当作无渠道而持久化更贵的默认组；规划能力查询异常不阻止画廊。

## Alternatives considered

- 只把首选名称从福利换为专用组：双模型优先仍会跳过它，不能解决根因。
- 强制将所有用户请求改到专用组：会覆盖用户明确路由、扩大权限及本次画廊范围，不采用。
- 新增模型分组配置或独立规划令牌：现有首选组回退已按请求模型筛选渠道，当前无需新契约；不在 AutoGroups 的规划组不会隐式加入。
- 始终关闭电商：忽略现有可达的规划备用组，造成不必要退化。

活跃笔记审计：旧[福利双模型默认](../../implemented/bug-fix/2026-11-01-image-playground-defaults.md)的选组决定被本篇替代，旧理由保留并互链；[首选组回退](../../implemented/feature/2026-10-24-preferred-group-fallback.md)为复用机制；[模型分组](../../implemented/feature/2026-10-27-token-model-groups.md)仅为未采用的扩展方案；亲和性优先、故障通知、音色、令牌创建等其他命中不变。

## Verification

定向 Go 回归 `go test -p 2 ./controller ./service ./middleware -run 'ImagePlayground|TokenRouting|PreferredGroup' -count=1 -timeout=120s` 通过。新增八组对抗场景覆盖专用组仅生图、规划实际回退范围、无权限、倍率移除、禁用渠道/能力、无图片、查询失败不改旧路由；实际选渠、HTTP 会话入口复用及 DB 持久化、key/到期时间保持、清理旧限制和刷新幂等。既有首选回退与计费分组测试通过。SQLite 隔离数据库，未执行 MySQL/PostgreSQL 实机测试，无 schema 改动。

图片子应用 18 个文件、194 项 Vitest 测试通过；新增生成和参考图编辑合成请求回归，断言只调用一次 images/generations 或 images/edits，模型保持 gpt-image-2，没有文本/规划调用。`make verify-notes` 通过（214 篇笔记及归档封印），`git diff --check` 通过；测试子进程已退出，本轮遗留测试进程为零。页面布局未修改，不重新生成静态产物。无收费模型调用；供应商真实生成、线上发布与部署不属于这次离线回归。

## Consequences

进入页面重新刷新才使旧临时令牌采用新默认；无实时健康探测承诺。失败回退可能改变费用，沿用现有按实际组计费。数据库结构和全局路由不变；单元测试不能证明供应商线上成功。合并不等于生产部署。
