# Agent Note: OpenRouter OpenAI 聊天模型纳入企业级

Status: implemented

## Problem

用户要求接入 OpenRouter 可调用 GPT，已有售价保持，缺少基础售价的型号按原厂美元价 ×6.6×5，分组企业级。用户明确本次仅 GPT、o 系列和 Codex 聊天模型；独立图片、语音、Embedding、Decisions 不纳入。

## Decision

复用现有类型 20、管理 API、精确模型映射、原厂采购限价覆盖和 options 价格；配置沿用管理 API。一个 OpenAI 渠道承载经逐项核价和真实调用通过的型号；对外沿用同名站内 ID，内部映射 openai/ 前缀。保留现有 19 套价格，候选 26 个新定价名称按官方价 ×33；9 个有 272k 档的新名称用原子表达式 bundle，其余普通倍率输入/输出/缓存分项计算。

OpenRouter 普通目录 76 条：20 个 batch/latest 特殊入口跳过，5 个图片/音频专用名称不在确认范围；51 个聊天名称中 45 个价格候选，3 个 gpt-oss 无原厂托管 API 报价，3 个 Azure 缓存报价超过原厂上限，继续不启用。全模态目录额外 15 个独立接口条目不纳入。新一代 8 个 Pro 名称是 OpenRouter 对基础型号 reasoning.mode=pro 的固定入口，官方明确按基础型号单价、汇总更多工作 Token 收费；据此核价，不误称独立官方模型 ID。旧 GPT-5.5 Pro 等保留自身较高官方价格。

复用 price 排序、限价内 fallback、服务商白名单、默认服务等级和附加工具拒绝；地区/加速等超价端点排除。长上下文采购仍保守使用低档硬上限；缓存只具备报价审核，不能声称有上游动态缓存硬限价。禁用未审核扩展缓存 TTL。原厂退役不直接推断合作云可用性，只有真实 OpenRouter 请求成功才配置开放。

## Existing notes audit

- [原厂限价](../../implemented/feature/2026-11-01-openrouter-price-routing.md)、[按厂商接入](../../implemented/feature/2026-10-31-openrouter-vendor-onboarding.md)：复用渠道与限价能力，本次用户明确解除 OpenAI 排除。
- [Claude 企业级接入](../../implemented/process/2026-11-01-openrouter-claude-enterprise.md)：复用先核价、短请求验证、停用创建、核对后启用的操作方法。
- [OpenAI 现价乘 6.6](../../implemented/process/2026-11-01-openai-retail-multiply-66.md)：当前售价作为既有基准，不重复放大；不覆盖同期分组修改。
- 官方参考价校验、模型目录补齐、缓存 TTL 与 Gemini 语音笔记涉及关联能力，不改变其既有行为。

## Alternatives considered

- 所有 OpenAI 新建带前缀名称会重复已有价格和模型；复用站内同名 ID 并显式映射。
- 只看目录就启用省测试，但旧型号原厂退役及账号/云服务商限制并不由目录证明；逐型号实测。
- 将新一代 Pro 当成原厂无价型号暂停会漏掉可核实的模式定价；官方 Pro 规则和 OpenRouter 固定入口描述共同证实后按基础单价核算。
- 接入所有模态需要另外核验独立协议/限价/计量，用户已明确本次聊天范围，不扩大为新功能发布。

## Verification

45 个型号均通过限价直接请求和站内 Chat；另有流式、Codex Responses、Pro Responses 流式共 48 笔成功，逐笔公式、企业级分组、日志和令牌扣费一致。搜索和扩展 TTL 返回 400 且零扣费；全部测试令牌清理。591 项覆盖引擎、16 项缓存时长策略、498 项新定价助手/结算检查通过。19 套现价保留，新增 26 套价格、37 个元数据。

初次校验误认为旧型号一定显式配置 CompletionRatio，自动停用渠道并恢复新价格；确认三个旧 GPT-5 名称沿用系统默认 8 后复用同一 1640 和元数据恢复。期间其他操作修改的分组和分组间倍率保留，旧渠道摘要不变。

长请求另实测发现新一代默认缓存写入字段被程序丢弃，用户已选择修复并更新生产；见[计量修复](../bug-fix/2026-11-01-openai-cache-write-metering.md)。当前 16 个受影响名称暂从 1640 移出，29 个继续开放，45 个的完整恢复和缓存扣费须在新版本上另外验收。配置成功与完成上线不混同。

[配置说明](../../../../docs/channel/openrouter-openai.md)、[审计](../../../../docs/channel/openrouter-openai-audit-2026-11-01.json)保存全部范围、售价、报价和回执。

## Consequences

其中5个旧型号的价格决定已由[按官方价修正](2026-11-01-retail-official-correction.md)部分取代；新记录按官方USD×33重建输入、输出、缓存及适用长档，其他范围和本篇历史验收记录保留。

短请求不证明全上下文、所有工具、视觉或缓存写入能力。不同服务等级和报价动态变化仍需复核；保守低档限价会拒绝部分长上下文。配置时保留并发分组调整，不恢复外部操作；API 部分写入失败先停新渠道，再按预期值恢复本次项目。

后续[其余型号校准](2026-11-01-enterprise-price-alignment.md)按原厂分项修正29个精确名称及一个继承别名；部分取代本记录涉及名称的旧售价基准，原渠道、限价及历史验收记录保留。
