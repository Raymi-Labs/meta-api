# Agent Note: 模型元数据补建后的供应商展示回退

Status: implemented

## Problem

模型广场通过 pricing 的 vendor_id 关联供应商。能力同步为启用但缺失元数据的模型补建记录，只写上下文/模态，vendor_id 保持零；pricing 默认映射却只对已有 MiMo/Inception 元数据补空，其他已知名称被跳过。2026-10-28 只读快照中 93 个公开模型有 49 个未知：47 个具有能力同步状态和零供应商，另有 codex-auto-review 与 minimax 名称规则遗漏。它们不是前端筛选重复计数问题。

## Decision

复用 initDefaultVendorMapping，所有已存在且 VendorID 为零的元数据均使用既有默认厂商规则补充展示值；按具体模型复制元数据，避免通配规则共享指针污染。非零人工归属保留，未知名继续未知，模型表不回写供应商。沿用已有供应商目录查找/创建，不从能力来源 Provider 猜模型厂商。补齐 codex-auto-review 完整名称与无其他已知厂商歧义的独立 minimax 名称片段；混合 MiniMax/Claude、MiniMax/GPT 名称保留未知；不扩展其他 Codex 名称或修改渠道协议。

## Existing notes audit

- [Inception 分类](../../implemented/feature/2026-10-21-inception-vendor.md)：部分重叠，原 MiMo/Inception 的补空限制扩展为所有已有默认识别厂商，显式优先与复制规则保留。
- [模型能力同步](../../implemented/feature/2026-10-27-model-capability-sync.md)：部分重叠，能力写入范围不变，在消费端补展示回退。
- [独立模型页](../../implemented/feature/2026-10-25-model-detail-page.md)：部分重叠，目录/详情继续共用同一 pricing 数据，不另做前端猜测。

## Alternatives considered

- 只给能力同步新建记录写供应商，可避免未来回退，但不能修复已有零值，而且把能力来源与人工厂商维护耦合。
- 批量回写 models.vendor_id 可以让管理页也显示推断值，但会固化推断、扩大数据变更，且没有区分人工未选与自动值的记录。
- 前端按名称补厂商实现直接，但目录/详情/其他 API 消费者易不一致；选择后端共用读路径回退。

## Verification

- `go test ./model ./controller`、相关定向回归及 `go vet ./model ./controller` 通过。集成测试覆盖真实同步 handler → pricing JSON，检查能力字段、供应商关联、数据库零值、人工归属、访客/用户/管理员可见性、隐藏及禁用模型、重复同步和 HTTP 429 失败保留。
- 将已有元数据条件临时恢复为旧逻辑时，新集成测试在同步后厂商归零处失败，确认覆盖此次根因。
- 隔离 SQLite 使用实际默认映射回放 93 个公开模型，未知从 49 变为 0；当前生产没有通过本轮写操作改变。
- 本地真实构建页面读取修前快照与实际代码生成的修后快照：修前未知筛选 49，修后未知入口消失；OpenAI 筛选 12，包含 codex-auto-review，卡片和详情一致。前端辅助函数/筛选/导航回归、完整构建、Chrome 五页预渲染和 Go 版本构建通过。

## Consequences

既有默认名称规则属于展示推断，不能作为路由或模型能力证据。非零但已失效的供应商引用继续交管理员处理；不借本次修复覆盖人工归属。发布与生产升级分别验证。

后续[管理供应商一致性修复](../../implemented/bug-fix/2026-10-31-model-management-vendors.md)将同一展示归类复用于管理列表、计数和筛选，补齐此前仅验证全量目录而未覆盖供应商标签的缺口，仍不回写模型供应商。
