# Agent Note: 模型广场目录浏览与价格卡片

Status: implemented

## Problem

现有广场将厂商、搜索、显示设置和价格表堆在固定高度区域中。常用筛选藏在抽屉里，卡片价格挤在模型名下，长名称和多维价格难以比较。用户希望参考 AIHubMix 的模型目录改善浏览体验。

## Decision

[模型发现与接入路径](2026-10-26-model-discovery-experience.md)更新搜索与筛选布局、缺失状态、快速开始入口和首页手动公告规则；本文其余数据及权限契约继续有效。

沿用 `/pricing` → `/api/pricing` → `GetPricing` 的角色可见模型、可用分组和用户倍率。集中展示厂商、计费类型和真实标签；端点等高级条件及价格显示设置保留在既有抽屉。保留分组侧栏和分组健康页，默认使用两列价格卡片，支持表格切换、排序、搜索、批量复制和分页。卡片与表格共用普通/首档价格提取，动态价格明确标示参考价，未知值不填零。页面使用自然滚动，分组侧栏独立滚动。

不推断模型的输入输出模态、上下文长度、上线日期或 TTFT；没有本地数据支持的竞品指标不展示。不接入竞品模型或价格，不改变后端、调用权限或计费。请求失败显示可重试状态；无结果可清空筛选。

## Existing notes audit

- [厂商与分组布局](../../implemented/feature/2026-10-21-pricing-wallet-layout.md)：部分取代桌面默认表格和固定视口布局，保留分组、钱包和计费规则。
- [筛选抽屉](../../implemented/feature/2026-10-21-pricing-filter-drawer.md)：部分重叠，复用抽屉与显示设置。
- [厂商切换](../../implemented/bug-fix/2026-10-21-pricing-vendor-transition.md)、[价格排序](../../implemented/feature/2026-10-17-model-price-sorting.md)、[筛选可见性](../../implemented/feature/2026-10-11-pricing-filter-visibility.md)和[健康筛选](../../implemented/bug-fix/2026-10-17-group-health-filters.md)：保留既有语义。
- 视频计费提案、缓存计费修复、钱包、首页及企业页笔记不在本次变更范围内。

## Alternatives considered

- 仅修改颜色和间距最易保留行为，但不能解决常用筛选难找、默认表格信息过密的问题。
- 全盘复制参考页并移除分组侧栏更接近参考，但会降低用户对不同线路倍率的比较效率，并引入本地没有来源的性能/模态信息。

## Verification

Chrome 本地项目壳使用公开定价快照及隔离模拟接口验证：桌面与 390/320px、德语深色、供应商和搜索联动、手机抽屉选组、免费组、真实按小时模型及首档参考价、复制与选择不误开详情、卡片/表格价格一致、空结果重置、分页回到列表顶部、分组栏独立滚动、503 后原页重试恢复、匿名健康页登录边界。

翻页后回到列表顶部，避免自然滚动布局停在新一页底部。共享首档辅助函数增加可选 K/M 单位，千 Token 价格保留六位小数，默认仍为 M，详情及日志的原调用保持不变；回归覆盖 K 价/单位、零倍率和无首档。currency、tiered-billing-display、billing-price-coefficients、pricing-filter-items、pricing-vendor-selection、upstream-optimizations 与 17 语言资源检查通过。Bun 直接执行 upstream-optimizations 的 data URL 导入不兼容，按项目既有脚本使用 Node 运行通过。

最终作用域 ESLint/Prettier、`make verify-notes`、发布策略检查、完整 `bun run build`（含生图子应用和 5 页预渲染）通过。发布范围仅包含模型目录与价格展示；不部署生产。

## Consequences

卡片降低单位面积模型数量，通过表格切换保留密集比较；筛选多时增加页面高度，使用自然滚动避免模型区域被挤没。价格设置收进抽屉后必须保留当前价格模式和单位提示。

## Related decision

[独立模型详情](../../implemented/feature/2026-10-25-model-detail-page.md)依据用户新选择取代目录中的分组展示与侧栏详情入口；其余价格、筛选和钱包决定保持原有范围。
