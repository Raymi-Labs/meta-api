# Agent Note: 模型管理与广场供应商口径一致

Status: implemented

## Problem

线上 main-2d4a335 的广场有12个OpenAI模型，管理页OpenAI标签却只有gpt-image-2。自动建档保留vendor_id=0；广场有展示回退，管理列表、筛选、计数仍只读取持久化值。此前目录补齐验收没有覆盖按厂商进入的完整流程。

## Decision

将广场现有厂商解析复用于管理查询，列表图标/供应商、标签计数、ID或名称筛选在分页前使用同一个解析快照。新增只读display_vendor_id，vendor_id继续返回存储原值，编辑保存不会把展示推断固化为人工配置。返回同一快照的供应商目录，避免新目录与列表标签不同步。非零人工归属、软删除、禁用记录和未知模型沿用原规则；管理计数以配置记录为单位，不冒充广场可调用模型数。

## Existing notes audit

- [广场供应商回退](../../implemented/bug-fix/2026-10-28-pricing-vendor-fallback.md)：部分重叠，扩展消费入口，仍不回写models.vendor_id。
- [目录补齐](../../implemented/feature/2026-10-31-model-catalog-reconciliation.md)：部分重叠，补上厂商筛选验收，不修改建档/删除策略。

## Alternatives considered

- 批量写供应商能快速改变线上标签，但会固化推断且需要额外所有权规则；沿用展示回退。
- 只修改前端标签能显示图标，但服务端筛选和计数仍错，因此在服务端分页前统一。

## Verification

使用1个显式+11个零供应商OpenAI模型完成 SQLite、MySQL 8、PostgreSQL 16 回归：厂商计数12、ID筛选12、分页10+2且不重复，名称和关键词联合筛选、空结果、人工归属、未知与软删除边界通过；读取及无关字段编辑不固化推断，逐项与广场解析结果相同。管理员 handler 集成回归覆盖首次搜索自动建档及列表/搜索的目录、计数一致。

隔离 SQLite + 实际管理接口 + 完整前端构建的 Chrome 页面验证：OpenAI 标签12，第一页10条、第二页包含 codex-auto-review 与 gpt-image-2；OpenAI/sol 联合搜索3条、总标签仍12；自动归类模型编辑中供应商保持未配置。Go 全量测试、Go 构建、相关 go vet、前端/文档构建通过。前端构建首次因既有手册语言检查超过5秒超时，原命令重跑通过。

## Consequences

管理查询增加轻量ID/名称/厂商快照读取，完整元数据仍仅加载当前页；现有名称推断不作为路由或能力证据。代码发布不等于生产部署。
