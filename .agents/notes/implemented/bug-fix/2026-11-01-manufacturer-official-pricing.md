# Agent Note: 官方参考价仅采用模型厂家来源

Status: implemented

## Problem

OpenRouter 接入资料将聚合平台报价存入 official_pricing，公开 pricing 原样输出，详情的官方参考价和分组折扣因而把代理报价当成厂家价格。带厂商命名空间的模型名也不满足现有官方资料源的裸 ID 校验。

## Decision

在公开 pricing 统一校验模型厂商与来源域名，缺少厂家来源的价格不输出，不参与任何官方折扣。原始资料保留供管理员核对，不批量删除生产数据。扩展同一后台同步：明确的厂商命名空间可查对应原生 ID；Cohere 的明确别名通过官方模型页身份和价格区块校验。旧 OpenRouter 来源允许被成功核验的厂家价替换，失败保留原数据但不公开为官方价；原人工厂家价、禁用同步和并发保护继续有效。

## Existing notes audit

- [官方参考价](../../implemented/feature/2026-10-30-model-official-pricing.md)：部分重叠，收紧任意来源链接均可公开的规则，保留人工维护与计费隔离。
- [官方同步](../../implemented/feature/2026-10-31-official-pricing-sync.md)、[自动资料默认值](../../implemented/feature/2026-10-31-model-official-defaults.md)：复用同步与所有权，增加来源约束及命名空间解析。
- [OpenRouter 接入](../../implemented/feature/2026-10-31-openrouter-vendor-onboarding.md)：保留渠道和销售价，修正其参考价展示语义。
- 分组折扣笔记仅消费同一 pricing，布局和折扣算法不变；其余能力及视频计价笔记无关。

## Alternatives considered

- 只修改“OpenRouter 标准价”文案：金额仍来自聚合平台，不能实现厂家来源要求。
- 对全部第三方价格删库并覆盖人工内容：能清理数据，但破坏管理员资料及失败恢复；采用读侧过滤，仅在有厂家证据时替换明确的 OpenRouter 旧价。
- 为所有开源模型套用其他托管商价格：覆盖多但不存在统一厂家 API 价格，不满足需求；未知就保持空态。

## Verification

- `go test ./...`、Go 服务端构建通过；首次全量测试在前端构建完成前因缺少嵌入的 web/dist 失败，前端产物生成后完整重跑通过。
- Cohere 四个实际下载的官方页面解析通过：Command A、R 08-2024、R+ 08-2024、R7B 12-2024；身份错误、单位变化、缺失字段、负值和平台变体拒绝。A+ 页面仅提供有速率限制的免费说明及 Model Vault 入口，不推算统一 token 单价。
- SQLite、MySQL 8.0、PostgreSQL 16 隔离实库通过官方同步所有权回归，包括旧 OpenRouter 替换、人工零价、禁用/删除和并发写保护；集成测试覆盖命名空间 HTTP 获取 → 持久化 → 公开 pricing → 源失败保留。
- 官方价/分组折扣8项前端测试、既有桌面/窄屏浏览器回归、主站及图片应用构建、17语言资源门禁和五页 Chrome 预渲染通过。浏览器使用合成 API，真实后端来源过滤由集成测试覆盖。

## Consequences

所有模型共用来源规则，但自动取价覆盖不等于全厂商覆盖：尚无厂家公开单价或尚未适配的模型显示暂无数据，不能计算官方折扣。新增厂家需要登记来源域名，新增解析器需要验证模型身份和条件；域名校验不等同于对人工金额和时效自动背书。原始资料保留可审计，网页改变时失败保留旧值。没有 schema 或计费迁移；发布不等于生产部署。
