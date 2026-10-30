# Agent Note: 数据看板五项指标与局部设计整合

Status: implemented

## Problem

用户已在本地预览确认五项指标、左侧周期右侧筛选、全宽模型分析及四个信息模块同排，要求仅将这个界面融合进当前项目。整包包含跨页面全局样式、侧栏与系统设置重组，不能直接套用。

## Decision

仅迁入 dashboard 组件和局部样式，保留项目全局主题、侧栏、其他页面及所有权限开关。头部显示已应用周期，移除标题、问候、用户/粒度/时区摘要与重复导航；浏览器时区及筛选草稿规则沿用。首屏使用总消费、请求数、Token 用量、缓存命中率和每百万 Token 综合单价；模型分析全宽，下方 API 信息、公告、问答与可用性按 4/2/1 列响应式排列。

前三指标与综合单价复用 data 聚合；综合单价为汇总消费 / 汇总 token_used × 1,000,000，沿用站点币种，仅为已结算用量平均值，不当作模型报价。缓存命中率使用既有日志统计 cache_global_rate（缓存读取 Token / 全部输入 Token），不同于只统计命中请求的 cache_hit_rate。普通用户仅调用 self，管理员名称在应用筛选时统一去除首尾空白，日志查询带 name: 前缀以保持精确匹配（包括纯数字用户名）。缓存请求独立加载、15 秒超时、独立失败与重试，沿用请求版本保护，不阻塞其他卡片或图表。失败或缺失不补零，零 Token 时单价和缓存率显示破折号。

## Existing capabilities and notes audit

已核对 dashboard → useDashboardData → router/api-router.go 的 data 与 log/stat 路由 → controller/usedata.go、controller/log.go → model/usedata.go、model/log.go、model/log_admin_query.go。data 有持久化聚合与缓存，缓存率来自 retained consume 日志；两者按相同查询范围过滤，但刷新延迟和日志清理可能导致覆盖不同，tooltip 明确来源，不新增后端或数据库契约。

- [概览状态](./2026-10-27-dashboard-overview.md)：部分取代其旧四卡与常驻摘要，保留查询草稿、角色/身份隔离、失败和旧请求保护、剪贴板决策；互链保留历史理由。
- [普通用户层级](./2026-10-28-ordinary-user-ui-hierarchy.md)、[参考布局](./2026-10-26-reference-layout-alignment.md)：不改令牌、钱包或模型目录，保留其范围。
- 下载包的 console-design-system 是参考设计，不引入其全局 CSS 或未合入的其他功能。

## Alternatives considered

- 直接套用整包最接近预览，但会修改侧栏、所有表单和系统设置，超出用户范围；只移植本页样式。
- 为五卡重做统一后端聚合可统一日志保留和时间精度，但扩大到统计数据和数据库变更；复用已存在的服务端口径并明确统计来源。
- 将缓存与 data 请求放在同一个 Promise.all 等待简单，但日志扫描可能很慢；保持独立加载和失败状态。

## Verification

- 31 项前端回归通过：五项指标计算、零/非法值、精确名称与 self 路由、主查询/缓存错误和加载的独立展示、周期栏、USD/CNY/自定义币种、既有查询和 17 语言资源。
- 浏览器使用隔离本地 API fixture（不提交），核对 1440/820/390 宽度、浅色/深色、中文/英文、长路线/URL/问答无整页溢出；信息模块开关全部关闭时无空网格。
- 普通用户只发送 data/self 与 log/self/stat，界面无管理员筛选或图表；管理员输入带空白的纯数字用户名，实际发出 username=12345 和 username=name:12345。筛选取消保留已应用条件，确认后周期和数据同步。
- 缓存失败与 15 秒超时展示独立重试；12 秒延迟期间其余指标已显示，切换周期的新缓存率显示后，旧响应返回仍保持新值。空用量展示 0 和无分母破折号。
- ESLint、Prettier、完整前端构建通过；本地预渲染因缺少 /usr/bin/chromium 按现有脚本跳过，生产发布工作流负责预渲染。提交前再次执行 Vite 构建和 make verify-notes。

## Delivery boundary

按现有版本流程提交推送并发布；生产升级与真实生产数据验证不在本次范围。原工作目录已有未提交修改，整合在基于最新主线的独立工作树进行。

## Consequences

不同统计来源有既有刷新/保留差异。仅前端与翻译资源变更，不改变真实计费、权限、后端或生产配置。
