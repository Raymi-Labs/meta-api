# Agent Note: 模型性能补值不显示来源说明

Status: implemented

## Problem

模型性能缺失时继续展示最近有效值，但图表悬浮提示、分组行、Uptime 摘要和模型卡片同时展示“沿用有效值”及来源时间，重复信息干扰直接比较。用户明确要求移除截图中的说明和下方相同说明。

## Existing capabilities and impact

- [性能图表](../../../../web/src/pages/ModelDetail/ModelMetricChart.jsx)、[分组详情](../../../../web/src/pages/ModelDetail/ModelGroupRows.jsx)、[健康摘要与小时详情](../../../../web/src/components/group-health/index.jsx)、[模型对比](../../../../web/src/pages/ModelCompare/index.jsx)及[目录卡片](../../../../web/src/components/table/model-pricing/view/card/PricingCardView.jsx)此前共用 `formatHealthReference` 显示来源时间，本次删除该展示函数；底层 `getHealthMeasurement` 与 `chartMeasurement` 决定是否使用历史值。
- [72小时性能参考笔记](../../implemented/bug-fix/2026-10-31-uptime-72h-measurement-reference.md)记录了此前来源时间展示；[模型卡片摘要笔记](../../implemented/simplification/2026-10-31-model-card-summary.md)记录卡片悬停来源提示。两者的数据补值决定仍有效，来源展示部分由本次新决定取代。
- [OpenRouter P50 笔记](../../implemented/feature/2026-11-01-openrouter-performance-p50.md)已要求不展示外部来源标签，与此展示方向一致；P50 和本站历史参考的取值优先级不变。
- 只调整已授权健康数据的呈现，不改变接口、历史回填、模型/分组隔离、真实零值优先、样本数或 Uptime 计算。

## Decision

移除所有用户可见的历史参考值标签、来源时间 tooltip 和来源说明；保留参考值本身及其数值格式。图表 tooltip 仅展示时间与各分组指标，原始样本数口径不变且不因补值增加；使用历史参考值时不另显示样本数说明。保留非来源类提示，例如样本不足、统计时间范围及缓存口径。

## Alternatives considered

- **只移除图表 tooltip** — 下方分组行、详情和卡片仍展示同一来源信息，不满足“下面的也是”的要求，因此统一处理所有性能展示入口。
- **删除历史补值逻辑** — 会让缺测区间重新变为空值并改变性能图连续性；用户要求删除的是说明文案，不是数值机制，因此保留。

## Verification

- 完整前端构建通过。
- 图表与 Uptime 定向回归、生产源码 ESLint、语言资源36项检查、笔记门禁通过。
- 现有浏览器流程通过：卡片、分组行和性能图保留历史吞吐数值且无来源提示；真实值优先、未知值、390px布局通过。
- 通用 i18n lint 报661项潜在问题，包含现有 HTML 属性和 P50 等文字；独立语言资源完整性检查通过。

## Consequences

用户无法从界面区分实时测量与历史补值；保留真实样本数，避免把历史值伪装成新增样本。若后续需要来源审计，应在独立诊断界面呈现，而不是恢复重复行内标签。
