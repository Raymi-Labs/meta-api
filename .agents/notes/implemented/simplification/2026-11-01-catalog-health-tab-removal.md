# Agent Note: 模型广场移除重复分组健康入口

Status: implemented

## Problem

模型详情的分组价格已支持点击 Uptime 查看健康历史；目录额外提供“分组健康”页签，用户认为重复，明确要求去掉。

## Decision

PricingContent 直接显示模型结果，将双页签改为“模型列表 + 筛选结果数”标题，去掉 activeTab 与 GroupHealthOverview 挂载。保留空态、失败重试、分页定位、卡片与表格切换。现有概览源文件不再由产品入口引用；共享统计接口、组件及其许可证信息保留，不扩大为全站健康功能删除。

详情页 Uptime、性能图表、跨模型对比与令牌健康摘要仍复用原接口和权限。卡片继续请求模型级摘要；目录不请求整组健康概览。

## Existing notes audit

- [目录浏览](../feature/2026-10-25-model-catalog.md)：部分取代保留健康页签的决定，其他目录功能保留。
- [健康筛选](../bug-fix/2026-10-17-group-health-filters.md)：目录入口停用；共享筛选函数与权限语义继续保留，不修改实际统计。
- [四类筛选](../feature/2026-10-31-model-catalog-facets.md)：部分取代“模型列表与健康概览同时筛选”的入口关系，模型列表和管理协同仍有效。
- [卡片摘要](../simplification/2026-10-31-model-card-summary.md)：无关，本次不改变卡片价格与性能值。

## Alternatives considered

- 仅隐藏页签文字：改动少，但保留无意义的选择状态和隐藏页面逻辑，无法清晰收窄产品入口。
- 删除全站分组健康接口：会破坏用户仍需要的 Uptime、对比与令牌摘要，超出请求。

## Testing

- 32 项相关检查通过：model-catalog-facets、model-health-view、healthUtils、model-detail-navigation；覆盖筛选、语言、权限和数据隔离。
- 完整 `bun run --cwd web build` 通过，含语言与正文检查、生图子应用、主站构建和公开页预渲染。
- `model-catalog-facets.browser.mjs` 通过，覆盖四类筛选、分页、空态、重试、详情返回、17 语言和模型管理联动；模型列表直接显示筛选后的结果，无页签和健康概览。
- `model-protocol-display.browser.mjs` 在桌面及移动深色模式通过；目录不请求整组健康接口，详情 Uptime 可展开/收起历史且请求始终携带当前模型；复制、选择、卡片跳转、表格及代码示例仍正常。
- 浏览器验收使用合成 API 数据，不修改生产。桌面截图人工检查通过，保持四列卡片与结果标题。

## Consequences

无法再从广场一次浏览所有分组汇总；改由模型详情逐组查看，这是用户明确选择的入口收敛。发布不自动部署生产。
