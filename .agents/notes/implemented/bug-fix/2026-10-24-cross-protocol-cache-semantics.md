# Agent Note: 跨协议输入缓存语义与历史统计

Status: implemented

## Problem

Messages 转 OpenAI 上游的输入总量包含缓存，结算入口却把消费日志统一标为 Anthropic。统计和界面因此重复累加缓存，全局缓存率偏低；普通倍率结算也可能重复计算输入缓存。生产核对的 516 条阶梯记录使用正确的完整输入长度结算，本次不重算历史账单。

## Decision

- 按明确 usage 标记和最终上游协议判断实际输入语义，在普通倍率计算中将缓存只计算一次；阶梯参数沿用原有正确规范化。
- 新消费日志增加 `prompt_tokens_semantic`，以 `inclusive` 或 `non_cache` 明确描述落盘 prompt 是否包含缓存，保存原始上游 prompt，不以客户端入口或模型名称推断。
- 历史数据仅在 Messages→OpenAI 转换链、阶梯模式、保存的 `tiered_params.len` 与 prompt 精确相等时识别误标；OpenRouter 旧记录已提前扣除缓存，不能仅凭转换链翻转语义。缺少充分证据的旧记录保持原分类，不修改日志或余额。
- 日志统计、公开令牌分布、前端摘要及计费说明统一使用该语义；分组健康和渠道亲和性也复用同一上游用量判定。原生 Anthropic 缓存时长分项及取整规则不变。公开令牌调用明细继续展示原始prompt计数及缓存明细，不重定义该既有API字段。

## Existing capabilities and note audit

复用 `PostClaudeConsumeQuota`、兼容入口结算、日志累计器及前端缓存组成辅助函数。[缓存时长一致性](../../implemented/bug-fix/2026-10-08-claude-cache-billing-consistency.md)部分重叠，保留原生时长分项决策并补充跨协议落盘契约；本次不更改渠道模型映射、价格配置、路由或生产服务。

## Alternatives considered

- 直接依转换链认定 OpenAI 输入包含缓存，能覆盖更多历史记录，但 OpenRouter 历史日志已经减去缓存，会破坏原本正确的展示。
- 批量改写历史日志标签可简化读取，但修改审计数据且难以证明所有旧记录的输入形态；采用明确新标记和有证据的只读兼容。
- 只修缓存率分母改动最小，却让日志摘要、令牌分布和普通结算继续分歧，因此统一输入口径。

## Verification

- `controller/relay_messages_cache_semantics_test.go`：Messages→Chat/Responses × JSON/SSE × 普通/阶梯共8组合，实际转换与usage解析、隔离SQLite钱包/令牌结算、落盘及统计均通过；输入1000、缓存800、输出20的样本只扣300内部quota，缓存率80%。
- `service/claude_usage_settlement_test.go`：Chat、Responses、OpenRouter两种格式、原生及明确Anthropic标记的12个普通/阶梯组合；包含创建分项，原始prompt不变、输入长度1000、非缓存300，金额535。验证日志标记和分组健康分母，另有缓存校验及亲和性模式回归。
- `model/log_prompt_semantics_test.go`：新字段优先、旧Chat/Responses误标、旧OpenRouter已规范化prompt、缺失/不匹配长度及非阶梯回退；统计和公开分布使用同一分母。生产聚合数字的合成fixture重现83.78%并断言查询不改日志或额度。
- 前端 `web/scripts/cache-semantics.test.mjs` 与既有缓存时长回归通过；真实页面壳的浏览器验收覆盖新旧跨协议、原生、旧OpenRouter、窄屏与普通账号视图。前端对有缓存创建的inclusive日志先规范化非缓存输入，再展示完整读写分项。
- 所有上述后端专项及新增controller/service/model回归的 `-race` 检查通过，`make verify-notes` 与diff检查通过。新统计处理在Go内完成，不新增SQL方言或schema迁移；未单独运行MySQL/PostgreSQL或真实收费上游。

## Consequences

已核实的516条阶梯记录只是日志误标导致显示分母重复，阶梯结算并未重复收费；普通倍率是另一个经代码及隔离结算回归确认的缺陷。旧普通倍率日志没有可靠长度快照时无法无损还原语义，保持旧解释。未来普通倍率账单消除缓存重复计价，既有价格与历史账单不变。测试使用隔离数据，不代表生产已升级。
