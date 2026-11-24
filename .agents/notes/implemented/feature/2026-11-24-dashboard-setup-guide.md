# Agent Note: 数据看板的首个请求引导

Status: implemented

## Problem

zzshu.cc 的公开状态报告 v1.0.0-rc.51。该上游标签的 overview-dashboard.tsx 包含三步引导、请求预览和推荐操作。本站数据看板已有账户、统计和监控数据，旧逐页教程只能介绍操作，不能反映开户进度。

## Decision

在 /console 移植上游步骤和折叠交互，适配本站 React Router、模块开关、钱包和 Playground 导出。创建密钥看本人令牌总数，加额度看本人余额或历史消耗，发送请求看累计次数。按账号保存展开偏好。首进看板以卡片承接引导，保留账户菜单手动教程。服务健康使用现有监控，示例使用用户可见、目录声明兼容的模型。

打开或刷新引导时，从 /api/perf-metrics/summary?hours=24 读取当前账号可见分组内的站点请求次数，在候选聊天模型中取最大值。并列时优先 gpt-6-sol，其余沿用目录顺序。统计不可用或无样本时也按此顺序选取。目录和权限查询必须成功，才能生成示例。

## Alternatives considered

整体合并上游前端会引入其他路由、认证存储和组件系统。直接复制上游真实密钥到剪贴板会偏离本站 Playground 的环境变量约定。保留已有看板并移植引导，两个入口复用请求生成器。

仅固定 gpt-6-sol 会在其他模型流量上升时继续给出旧建议，也无法涵盖没有开通该模型的账号。社区模型榜反映独立社区评分，与站内请求次数口径不同。

## Verification

生产前端产物配合本地 API 夹具，覆盖新用户的三个真实路由、无密钥预览、模型交集、一个正常一个异常的监控、完成默认收起、按账号记忆、键盘展开、查询失败重试、无模型和隐藏模块。390px 宽度检查通过。看板组件、查询、指标、顶部导航、协议导出、17 语言资源及完整前端构建通过。

2026-11-25 在本地生产构建中刷新看板，验证榜首交替、并列、空统计、统计失败和 gpt-6-sol 无权限时的选择；每次核对 cURL 与状态行。排名更高的私有模型和图片模型均未进入示例。

## Consequences

账号可见模型仍需令牌分组授权。监控只说明已配置探针的状态。历史五卡布局决策继续管理统计和日期筛选，普通用户操作层级与剪贴板规范仍适用。

## Related decisions

[五项指标](./2026-10-30-dashboard-five-metrics.md)与[概览状态](./2026-10-27-dashboard-overview.md)继续负责日期范围和统计，本卡的进度使用本人累计数据。[普通用户操作层级](./2026-10-28-ordinary-user-ui-hierarchy.md)与参考布局笔记涉及其他操作入口，本次不调整。上游来源为 [v1.0.0-rc.51 的 overview-dashboard.tsx](https://github.com/QuantumNous/new-api/blob/v1.0.0-rc.51/web/src/features/dashboard/components/overview/overview-dashboard.tsx)。
