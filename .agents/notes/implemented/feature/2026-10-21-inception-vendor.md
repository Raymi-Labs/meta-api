# Agent Note: Inception 厂商分类与 Mercury 元数据

Status: implemented

## Problem

Inception 的 Mercury 模型使用兼容 Chat Completions 的接口，但渠道管理、模型选择、分组和日志筛选没有对应厂商。模型广场已有供应商表与默认归属机制，缺少 Mercury 的识别及默认资料。

## Decision

扩展现有展示厂商，内部键为 inception、显示名称为 Inception；渠道 Type 与转发协议保持原值。Mercury 必须是独立名称片段，渠道内全部模型（应用模型映射后）须归属明确且一致才自动分类；仅有 inception 命名空间不足以推断。显式展示厂商和跟随协议设置继续优先。

渠道厂商识别对完整名称 `codex-auto-review`（忽略首尾空白和大小写）补充 OpenAI 归属，模型映射后的同名目标复用该规则。它没有 GPT 名称片段，原规则会让包含它的 OpenAI 渠道回退到渠道类型分类；不把通用 `codex` 片段加入厂商表，避免推断未确认的其他别名。混合厂商、未知模型、显式厂商和跟随渠道类型的边界保持，模型广场与模型选择器的独立识别规则不在本次修改范围。

模型广场复用默认供应商映射，为实际可用的 Mercury 模型补充 Inception 名称、图标标识与简介；仅填补无归属元数据，不覆盖管理员非零 vendor_id。共享名称规则须复制后应用展示回退；复用 Inception / Inception Labs / InceptionLabs 已有供应商，保留自定义图标和简介，不新增启动迁移。官方同步使用同一名称规则。令牌分组、渠道关联分组及使用日志统一识别 Inception/Mercury 别名，不改变组标识或权限。

不新增渠道类型、定价、模型路由、密钥或可用模型。不把 Decide、Edit 或扩散过程可视化视为已适配的聊天接口。初始图标库无 Inception，曾沿用首字母图标回退；此显示妥协由[厂商 Logo 补齐](../bug-fix/2026-10-31-openrouter-vendor-logos.md)取代，现使用本地真实品牌组件，分类与协议决定保持不变。

[渠道思考适配](2026-10-25-channel-reasoning-profiles.md)在显式启用 auto 时复用厂商归属选择精确模型模板；单独更改展示分类仍不改变选渠与协议，旧渠道缺省保留原行为。

模型广场对已有零供应商元数据的展示补空范围，按[统一回退规则](../../implemented/bug-fix/2026-10-28-pricing-vendor-fallback.md)扩展为所有默认可识别厂商；MiMo/Inception 特殊名称边界、非零人工归属和共享规则复制保护继续适用。

## Existing capability review

- 渠道编辑/分类 → channel controller → channel_vendor.go，已有 category 与 vendor 筛选、计数和持久化。
- 模型广场 → /api/pricing → pricing_default.go；模型同步 → ResolveVendorNameForModel / ensureVendorID；供应商 CRUD 为管理员权限。
- tokenGroupUtils.js 与 log_scope.go 按相同分组名称别名筛选；渠道分类关联已有分组目录。
- 活跃笔记检索命中 [令牌选择器](../../implemented/feature/2026-10-20-token-create-selector.md)，部分重叠，仅扩展其既有厂商别名，保留筛选不赋权的约束。未发现拥有展示厂商机制的其他活跃笔记。

## Alternatives considered

- 新建 Inception 专用渠道类型可容纳特殊接口，但本次目标是分类且普通聊天可复用既有协议，新增适配器会扩大转发和计费风险。
- 仅添加前端标签改动少，但后端拒绝未知厂商，模型广场与日志筛选也会不一致。
- 自动写入全部型号及促销价格能让目录立即出现，但没有真实渠道能力支撑且价格会过期，因此仅对已有可用模型派生资料。

## Testing

`codex-auto-review` 补充回归覆盖原名、大小写/空白、映射后归属、混合/未知及相似名称、显式设置、分类计数与筛选；`go test ./model ./controller ./service`、渠道分类与模型辅助函数的 32 项 Bun 测试和笔记校验通过。

Go 全量测试通过；model/controller 的 vet 和厂商/分类/日志范围定向竞态检查通过。覆盖自动/映射/混合/含糊名称、显式厂商、列表和搜索分页计数、日志筛选统计、共享元数据不污染、显式归属保留、别名复用及重复执行。Bun 分类/模型/分组工具 46 项测试与图标测试 2 项通过。

本地隔离 SQLite 完整应用使用虚构渠道，地址指向本机未监听端口，不调用上游。管理员页面验证 Inception 两条分类计数、编辑选项、模拟保存失败保留选择、重试保存及刷新（channel_vendor=inception，type=1）。模型广场筛选只显示两条 Mercury，默认简介与 I 图标正常；普通用户可读目录，访问管理员渠道接口被拒绝。390px 暗色模型广场无横向溢出。普通用户令牌创建弹窗的 Inception 筛选合并两种命名分组，保留实际 group 值且筛选不自动选组。前端生产构建、使用本地 Chrome 的五页预渲染及 Go 二进制构建通过。

前端命名判断与后端保守片段规则保持一致；未来扩展已知厂商片段时需一起维护。已有 Browserslist、lottie eval、大 chunk 等构建提示不属于本次改动。

## Consequences

分组厂商是命名约定，与渠道展示厂商及模型元数据各有数据来源，不能据此改变权限或路由。图标暂为首字母回退。分类发布不表示创建生产渠道、真实模型验收或上线部署。
