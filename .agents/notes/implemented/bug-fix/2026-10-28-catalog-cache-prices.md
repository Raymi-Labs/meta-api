# Agent Note: 模型目录缓存价格一致性

Status: implemented

## Problem

模型卡片排除了名称含 cache 的价格项，普通计费缓存读写被隐藏，而动态计费的 tiered-cr/cc 仍显示。普通价格计算还读取了不存在的 create_cache_ratio，并未区分后端未配置时返回的零值与明确零价。详情分组行用同一字符串判断，动态缓存价被放入普通价格栏。

## Decision

沿用 /api/pricing、已有分组与时间倍率、币种和单位计算；普通价格以 supports_cache_read/creation 为前提，读取 cache_ratio、cache_creation_ratio 和显式 TTL 的 cache_creation_ratio_1h。只展示有效非负值，明确配置零价时保留零价。动态价格仍来自表达式首档，不受普通倍率支持标记限制。卡片展示所有有效价格项，详情使用共享语义分类将普通及动态缓存项放入缓存栏。无需更改后端价格、结算、权限或生产配置。

## Existing notes audit

- [模型目录与独立详情](../../implemented/feature/2026-10-25-model-detail-page.md)：部分重叠，继续复用同一价格与权限来源。
- [模型元数据与分组指标](../../implemented/feature/2026-10-25-model-catalog-metrics.md)：部分重叠，保留分组布局与指标口径。
- [厂商与分组布局](../../implemented/feature/2026-10-21-pricing-wallet-layout.md)：表格首档和缓存读取列继续有效。
- [Claude 缓存计费](../../implemented/bug-fix/2026-10-08-claude-cache-billing-consistency.md)：沿用显式 TTL 元数据，前端不根据模型名猜测缓存能力。
- [跨协议缓存语义](../../implemented/bug-fix/2026-10-24-cross-protocol-cache-semantics.md)：结算和协议转换与本次展示修复无关。

## Alternatives considered

- 仅删除卡片过滤最小，但会把未配置的后端零值显示成免费缓存，且仍漏创建价，因此同时修正规范字段和支持标记。
- 根据模型名补缓存价格可以减少空项，但没有本站配置依据，可能与真实计费不符；缺少显式配置时不补造价格。
- 改成另一套 price.jsx 计算器已有部分缓存逻辑，但缺少当前目录的时间倍率和动态计费语义，因此修正现用 helper。

## Verification

- 新增 10 项实际计价函数回归，覆盖普通/动态、未配置/明确零价、非法数值、TTL、K/M、币种、分组/时间倍率、倍率显示、按次及分类一致性；连同已有价格换算、阶梯展示、缓存语义和优化回归通过。
- Chrome 使用当前公开 /api/pricing 快照在本地真实项目壳验收：gpt-6.1-sol 补齐读写价，codex-auto-review 不补造缓存价，按次模型不显示 Token 缓存，Claude 显示显式 5m/1h；动态分组缓存栏及 K/M 换算一致。
- 主站与独立文档构建、语言资源检查、ESLint、make verify-notes 及发布策略检查通过；当前机器未提供默认 /usr/bin/chromium，构建的预渲染步骤按原脚本跳过，发布 CI 继续执行完整预渲染。

## Consequences

显示更多已配置价格可能增加卡片高度。支持标记表示可展示的独立计价配置，不等同于官方能力或实际缓存命中；未显示不表示免费。本地快照验收不更改生产配置，不执行模型调用或部署生产。
