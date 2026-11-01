# Agent Note: OpenRouter Claude 纳入企业级

Status: implemented

## Problem

用户要求将 OpenRouter 当前可调用的 Claude 接入企业级，已有型号保留现价，新型号按原厂美元价格 ×6.6×5 定价。此前 Claude 属于批量接入排除项，本次明确扩展该厂商；既有采购限价、特殊入口排除继续有效。

## Decision

复用类型 20、管理员渠道 API、模型映射和现有采购覆盖生成器，新建单个 `OpenRouter · Claude` 渠道，企业级分组倍率保持 1。当前目录 30 条中 15 个 batch 继续跳过，15 个普通型号逐项核价和实测。沿用 14 个站内名称及全套既有售价，内部映射到 `anthropic/` ID；新 Opus 4.1 使用官方完整 ID `claude-opus-4-1-20250805`，原厂输入/输出 15/75 美元每百万 Token，后台 495/2475 能量，缓存分量同比换算。

保留 price 排序、限价内回退、审核后的服务商允许列表、默认服务等级、拒绝未知型号与附加收费托管工具。地区加价端点排除；旧型号虽从原厂直接 API 退役，合作云平台生命周期独立，只有 OpenRouter 实测成功才启用。Haiku 100k 和 Sonnet 4.5 200k 档仍使用保守低档上游硬限价，长上下文可能因无满足限价端点而拒绝，不为成功提高上限。

生产凭据在服务器内复用当前 OpenRouter 账号，不导出。新渠道先停用，备份并配置唯一新价格、元数据、映射与限价；校验后启用，验收真实网关、目录、分组、日志及扣费。失败保留关闭状态并按本次字段恢复，不更新其他渠道或放宽账号权限。

## Existing notes audit

- [原厂限价](../../implemented/feature/2026-11-01-openrouter-price-routing.md)：复用，原 Claude 排除范围由本次扩展；缓存仍依赖审核快照，不宣称动态硬限价。
- [按厂商接入](../../implemented/feature/2026-10-31-openrouter-vendor-onboarding.md)：复用单厂商渠道、API 配置及批量入口排除。
- [Claude 现价乘 6.6](../../implemented/process/2026-11-01-claude-retail-multiply-66.md)：保留所有已配置的 14 个在用名称价格，不重复乘 6.6。
- [账号迁移](../../implemented/process/2026-11-01-openrouter-account-key-migration.md)：沿用已验证的当前密钥，不重新替换其他渠道。
- Gemini 语音、厂商 Logo、官方来源验证无规则变更。

## Alternatives considered

- 直接加入 Gemini 或混合厂商渠道更省记录，但不符合一个厂商一个渠道的原要求；单独建立 Claude 渠道。
- 所有型号以新 `anthropic/` 名称入库可以省映射，却复制已有模型和售价；对已有名称明确映射，仅不存在的原厂型号新增价格。
- 只按目录声明启用最快，但生命周期/账号限制并不能由目录证明；短请求确认可调用后再开放。

## Verification

15 个普通型号全部通过当前账号受限价直接请求，实际输入/输出成本不超过原厂上限。195 项真实覆盖引擎检查、relay/common 参数覆盖回归、18 项新 Opus 4.1 计费分量核验通过。

生产渠道 1639 已启用，企业级倍率 1；公开 pricing、令牌目录与 15 条能力一致。14 个旧售价及其他选项保持，新增两项为 Opus 4.1 的 ModelRatio 247.5 与 CompletionRatio 5，原有缓存比例继续生效。15 笔 Chat、1 笔流式、1 笔 Messages 全部成功，单条消费日志、分组、计算金额与测试令牌扣费相符；1 笔 web_search_options 拒绝且零扣费。所有测试令牌删除。

保存设置被后台规范化导致初次字符串校验失败，自动保持渠道停用并恢复新增价格；按保存语义验证后在同一渠道/元数据上继续并成功。MySQL 名称查询排序规则、测试搜索限流与恢复查询列名问题均在前置/只读步骤发现并修正；保留已完成请求，未重复已成功推理。plugins 是 Chat DTO 不接受的字段，原拒绝测试得到普通 Chat 响应；没有放宽规则，改用真实支持的 web_search_options 完成拒绝验收，区分原始请求和转换后请求的边界。

[配置说明](../../../../docs/channel/openrouter-claude.md)、[逐项审计](../../../../docs/channel/openrouter-claude-audit-2026-11-01.json)和生产私有操作目录记录完整过程。无应用程序、schema 或版本变更，不新增 Release。

## Consequences

其中3个旧型号的价格决定已由[按官方价修正](2026-11-01-retail-official-correction.md)部分取代；新记录按官方USD×33重建输入、输出、缓存及适用长档，其他范围和本篇历史验收记录保留。

服务商及原厂价格会变化，本次无自动追价；缓存没有独立上游动态限价。短请求不证明完整长上下文、全部工具、多模态、缓存命中能力。配置新增可恢复，单次上游测试会产生少量真实费用；不修改历史账单。
