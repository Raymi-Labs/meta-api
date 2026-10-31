# Agent Note: 阿里厂商别名与套餐来源归一化

Status: implemented

## Problem

线上厂商表有「阿里巴巴」16、Alibaba 25、Alibaba Token Plan 26。后两者分别关联 11 个 Qwen 模型和 6 个 Qwen Image/Wan/HappyHorse 模型。社区目录以提供方而非模型作者区分国际站、国内站、订阅套餐；旧同步把这些名称直接写成厂商，本地默认规则又使用中文名称。前端局部别名不能修复管理列表和模型外键。

## Decision

复用厂商表、模型 vendor_id、启动数据规范化和既有同步入口。普通阿里名称归并到已有「阿里巴巴」；套餐来源需结合严格模型名称识别，阿里模型归阿里，已知第三方模型按实际厂商，未知名称保持未知。Token Plan 原始 provider 继续保存在同步来源中，不根据目录来源给真实渠道添加套餐标签。

启动事务仅移动旧阿里别名下可明确识别的阿里模型，保留其他人工厂商、混合/未知归属、软删除模型的引用和自定义图标；仅软删除无引用别名。同步自动所有权的 vendor 快照随实际移动更新，其他来源字段原样保留。价格、渠道、分组和协议不变。旧目录同步、冲突预览、自动能力同步和模型广场默认归属使用相同规则，防止下次同步重新建套餐厂商。

## Existing capability review

- [厂商别名复用](../../implemented/feature/2026-10-21-inception-vendor.md)与[厂商回退](../../implemented/bug-fix/2026-10-28-pricing-vendor-fallback.md)部分重叠：复用保守名称规则和人工归属保护，不扩大渠道协议推断。
- [属性来源](../../implemented/feature/2026-10-30-model-attributes-official-sources.md)与[资料默认值](../../implemented/feature/2026-10-31-model-official-defaults.md)部分重叠：保留来源、自动所有权和乐观更新契约。
- [四类筛选](../../implemented/feature/2026-10-31-model-catalog-facets.md)继续使用服务端模型归属；不为本次缺陷新增前端独立厂商表。

## Alternatives considered

- 仅改前端显示最少，但管理列表、详情和同步仍保留不同外键，会再次分裂。
- 将所有 Token Plan 模型直接归阿里最简单，但套餐可能包含第三方或未知模型，会制造错误归属。
- 仅手动清理生产可以立即见效，但旧同步仍会重建别名；采用持久化规范化和同步入口修复一起发布。

## Verification

`TestAlibaba*` 在 SQLite、MySQL 8.0.46（由生产仅导出 models/vendors DDL，utf8mb4_unicode_ci，无生产数据）和 PostgreSQL 16 通过。覆盖 17 条真实模型名称的归并、重复运行、混合别名、软删除引用、规则匹配记录、NULL 厂商、自定义厂商/图标与自动快照。HTTP 同步及预览回归验证连续同步不重建别名、不产生虚假厂商冲突；能力目录保留 provider 并区分第三方与未知作者。

`go test ./model` 与 `go test ./controller` 全包回归、前端构建、Go 构建和 `make verify-notes` 通过。普通阿里社区源对第三方保持原先的未知归属；套餐来源仅对可识别的实际作者给出厂商。

发布后的生产验收经受保护更新入口执行，核对厂商列表、关联模型、公开 pricing 和页面，并对比渠道及价格配置摘要。迁移只触及 vendors 和 models 的归属/对应自动快照，不写 options、channels 或 abilities。

## Consequences

未知或混合旧记录会保留供人工判断，不能以清空列表为目标强制改作者。只合并明确的阿里别名；不推广到其他提供方，不改变套餐权限或计费。
