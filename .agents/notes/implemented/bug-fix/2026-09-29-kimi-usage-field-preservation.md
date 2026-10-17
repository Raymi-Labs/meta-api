# Agent Note: K3 缓存与思考用量字段保真

Status: implemented

## Problem

调用成功但测试器没有读到 cached_tokens 或 reasoning_tokens，不足以证明缓存未命中或思考未开启。原生Chat响应存在嵌套字段与供应商别名；typed重序列化及流末尾元数据可能丢失别名或让内部结算与下游可见值不一致。

## Existing capabilities and impact

复用 `relay/channel/openai/relay-openai.go` 的原生Chat流式/非流式处理、usage解析及既有供应商后处理。已有缓存提取支持部分Moonshot choices用量，但依赖末块原文；没有reasoning平铺别名兼容。`dto.Usage` 跨协议共享，避免为截图问题全局修改所有供应商结构。

活跃的[官方KVV对齐](../testing/2026-09-29-official-kvv-alignment.md)约束不能伪造通过结果；[流式完整性](2026-09-29-kimi-request-stream-integrity.md)约束usage与终态分开。此笔记补充字段来源与优先级，不改变这些决定。

## Decision

- 仅K3/kimi-k3、OpenAI兼容或Moonshot原生Chat路径启用兼容；其他模型、图片、Responses原生协议不改变。
- 标准嵌套值优先，明确0仍为权威。仅从真实、非负整数来源补齐缺失的标准字段及平铺兼容别名，不推算命中/思考，不从reasoning字符数估计。
- 兼容usage平铺、Responses风格details和单choice缓存位置，`cache_read_tokens`作为最低优先级后备来源；不把多choice用量擅自相加。数值大于其明确输入/输出总数时不作为补齐来源，`cache_write_tokens`不映射成读取缓存。
- 解析前统一原始usage，保留其未知字段；强制格式化也不把缺失的缓存/思考量制造成0。usage存在的块与尾部元数据分开保存，计费继续使用同一真实来源。
- [流式保留与来源诊断](2026-09-29-kimi-stream-usage-evidence.md)进一步处理相同输入/输出总量的后续usage快照覆盖细项，扩展单choice思考位置，并在admin_info记录固定来源元数据；未报告数据仍不制造计数。
- 缺失、0、大于0分别表示未知、报告为零、有用量，验收文档要求查嵌套路径及原始响应，不以字段存在直接判命中。
- [冲突一致性修复](2026-09-29-kimi-usage-conflict-consistency.md)统一JSON/SSE内部计数选择，unknown/invalid不再沿用通用解析器复制的低优先级别名；K3非流式复用既有流式总量归一，负数总量不进入结算。

## Alternatives considered

- **所有返回都补0**：让字段稳定但把未知伪造成未命中/无思考，并可能使错误的“字段存在即命中”断言变绿，拒绝。
- **仅让客户改测试器**：是修正断言所必需，但无法修复网关确实存在的流尾字段丢失与重序列化问题；两部分分开处理。
- **全局修改Usage反序列化**：所有供应商同时受影响、风险较大；选择有范围限制的原生Chat边界兼容。

## Consequences

兼容真实缓存信息可能纠正此前遗漏的缓存计费，但不调整历史余额。上游根本未报数据时仍无法证明命中率或推理量；截图缺原始JSON、渠道与请求ID，不能宣称客户两项已线上解决。供应商用量冲突仍需诊断，不能修改官方KVV或硬补命中值。

平铺别名给只读平铺字段的客户端提供兼容，但标准嵌套字段仍优先。保留冲突原文，不编造第三个折中值；缺失与明确0可在最终Chat线缆响应中区分。共享内部Usage仍以整数表示未知，不能将内部默认0当作供应商证据。这个边界处理不改变其他模型的用量结构。

## Verification

- 修复前，K3平铺reasoning_tokens=18的handler用例复现内部计数为0。
- handler回归覆盖JSON/SSE与强制格式化，平铺/嵌套映射、usage后的元数据尾块、单choice计数、明确0/缺失/null、冲突、非法及超界候选值、多choice不合并、usage隐藏、其他模型及Responses不启用本规则。
- 隔离SQLite控制器验证流式/非流式下标准、平铺、重复字段、标准0冲突、cache_read别名及其标准0冲突六种形式；在测试倍率下报告80缓存的形式扣48内部额度，标准0扣120；钱包/令牌/消费日志一致，缓存不重复扣算，思考不额外叠加输出Token。
- 全仓 `go test -p 2 -timeout 120s ./...` 通过；controller和OpenAI handler的用量、流式、失败退款定向 `go test -race -p 2 -timeout 90s ... -count=2` 通过。重复执行夹具关闭SQLite连接，避免测试自身残留状态。
- [授权的元衡用户体验实测](../testing/2026-09-29-kimi-user-experience-usage-probe.md)保留14条真实请求证据，实际usage切片的16个handler回放子用例通过。真实输入5720首次cache_read=0、重复时与标准缓存计数同为5720，补齐此前漏识别的cache_read字段；完全没有计数的样本仍保持未知。
- 未修改门户测试器、官方KVV、生产配置或历史余额；线上实测使用旧版网关，本地补丁尚未部署。仅对明确相同总量的快照保留细项，供应商扩展usage增量分散在多个无总量数据块的任意形式仍需原始SSE样本单独核对。
