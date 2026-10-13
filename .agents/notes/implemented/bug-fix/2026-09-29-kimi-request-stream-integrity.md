# Agent Note: K3 请求语义与跨协议流式完整性

Status: implemented

关联：[明确上游能力错误的安全切换](2026-10-12-upstream-capability-failover.md) 复用这里的输出后禁止重放边界，只扩展尚未输出时的特定上游 400 识别。

[Responses 空输出取消共享冷却](2026-10-13-responses-empty-output-no-shared-cooldown.md)保留失败退款和已输出不重放边界，仅取消完成但空输出的共享冷却并补充管理员诊断。

## Problem

相同模型经不同协议进入网关时，工具选择、历史思考、视频参数和输出预算必须保持用户要求。若转换忽略工具选择、只识别一种视频形状或取两个预算中的较大值，用户要求便被改变。网关额外缓存一个流式数据块会放大上游停顿；仅修复首块发送但漏掉并行工具或尾部 usage，同样不能完成用户流程。请求完整转发不等于上游一定执行工具或理解视频。

## Existing capabilities and impact

- 已有 Chat 消息级 tools 保留和 OpenAI SSE 立即发送；复用这些机制，不新增独立中转层。
- 入口为 `router/relay-router.go`、`controller/relay.go`，转换在 `service/convert.go`、`service/openaicompat/chat_to_responses.go`，上游处理在 `relay/channel/openai/` 与 `relay/channel/moonshot/`。
- `dto/openai_request.go` 承接媒体解析和有效预算；Claude 转 Chat 的 tool_choice 与历史思考归 `service/convert.go`；Messages 增量与结束状态归 `service/chat_to_claude_stream.go` 和现有 OpenAI 流处理。
- 已检索活跃笔记的 Kimi、工具、视频、stream、输出关键词，没有持有本决定的笔记。现有渠道用量与项目笔记流程两篇与本决定无行为重叠。
- 不修改数据库、价格、权限、线上渠道权重或已知失败正文识别规则。生产部署由管理员另外执行。

## Partial supersession

模型名驱动的 K3 输出别名自动改写由[复测范围纠偏](2026-09-29-kimi-retest-scope-correction.md)撤销；其余有本地回归证据的保真与流式修复保留。以下输出别名决定保留为原有取舍记录，不作为当前请求改写规则或客户验收结论。

## Decision

- 扩展现有转换：保留明确的工具选择、并行开关及 assistant 历史思考；不自动强制工具或关闭思考。
- 视频字符串和对象都进入媒体解析，保留原始结构与对象扩展字段；无法承接视频的协议返回明确转换错误，不能当图片处理或悄悄删掉。
- 输出预算按 `GetMaxTokens()` 的既有优先级统一。K3 Chat 兼容适配将显式 max_completion_tokens 映射为 max_tokens；不增加请求预算，不截断响应，不捏造 usage。
- Messages 路径即时发送正文、思考和工具增量，只延迟结束信号以吸收独立 usage 尾块。并行工具按实际索引保存状态，避免稀疏索引膨胀或首块漏工具。传输失败不能发送成功终态，也不能在已输出后重试拼接。
- 缺少工具 ID/名称时，只暂存身份到齐前的参数；单工具最多 64 KiB，单消息最多 1024 个工具状态。身份改变、非法索引或未完成身份的终态明确失败，不构造可执行的假工具。
- Chat 转 Responses/Claude 无法表达消息级动态工具时明确拒绝，不能把工具声明提升到全局或静默丢弃。视频在原生 Chat 及已有 Responses 映射中保留；转 Claude 的不支持情况返回 400。
- 流式失败沿用 controller 的预扣退款，不产生正常消费记录。取消或事件校验失败先关闭上游 body，再等待 scanner 退出，避免等待清理超时。

## Alternatives considered

- **只配置单一渠道**：可以减少供应商差异，但无法修复网关字段丢失和跨协议缓冲，且会集中流量。本改动不改变生产路由。
- **强制关闭思考、追加工具提示或截断到估算 Token 上限**：容易改善部分分数，但改变调用者意图，且没有上游 tokenizer 就不能可靠限制实际成本。只修请求约束传递，不承诺上游严格执行。
- **原样转发全部字节**：同协议保真最强，但不能覆盖 Messages/Responses 转换，也绕不开计费所需的媒体元数据。扩展现有结构化转换，并对不支持的转换明确报错。
- **立即发送包含 finish_reason 的所有块**：延迟最低，但 Messages 可能在独立 usage 到达前发送 message_stop。只保留结束信息，内容及时交付。

## Consequences

原生Claude缓存时长与计费组成的一致性由[缓存计费修复](2026-10-08-claude-cache-billing-consistency.md)补充；不改变本篇关于跨协议交付完整性及失败边界的决定。

网关保留调用方参数，避免静默退化与网关缓冲等待；费用仍以实际成功结果结算。代价是不能可靠转换的动态工具请求、非法流式数据和身份不完整的工具流会明确报错。HTTP 200 已提交时用协议内 error 表达失败，不能修改已经发送的 HTTP 状态。

已交付部分内容的失败请求也按当前失败退款策略返还预扣，不能因而宣称供应商没有成本。恶意反复取消仍需要现有并发、限流和运营审计约束；若要对部分输出单独收费，必须另行明确计费政策，不能通过把断流记为成功来实现。

K3 输出别名归一仅匹配 `kimi-k3` 和 `k3`，未知模型保持原字段。显式 `max_completion_tokens` 优先于旧 `max_tokens`，不进行 Token 截断，不强改推理等级；管理员参数覆盖与原样转发模式保留既有语义。

明示工具要求不代表上游一定执行；预算字段一致不代表供应商 tokenizer 与约束实现一致；视频参数完整不证明视频理解。跨协议转换影响非 K3 的同一入口，因此普通文本、工具多轮与失败退款均纳入回归。观察性慢首字机制保持原设置，不以心跳伪装首字或擅自增加并行重试成本。没有新增路由能力标签，也没有自动调权或生产配置变更。

## Verification

- 修复前定向用例复现工具选择/历史思考丢失、视频形状变化、较小输出预算被增大、Messages 首块等待、终态用量遗漏及断流误报成功。
- `go test -p 2 -timeout 120s ./...` 通过；使用 Go 1.25.1、GOMAXPROCS=2、GOPROXY=off。
- `go build -p 2 ./...`、`git diff --check` 和 `make verify-notes` 通过；笔记校验使用与 CI 相同的 Bun 1.3.14。
- 对 controller、dto、service、service/openaicompat、relay/channel/openai、relay/helper 的新增契约、首块、工具身份和资源清理用例执行 `go test -race -p 2 -timeout 90s ... -count=1` 通过。
- 控制器通过替换 HTTP transport 检查最终出站视频、工具选择、输出预算及不改变原样转发；没有使用真实供应商或客户密钥。
- 隔离 SQLite 覆盖 Chat/Messages 的钱包、订阅、独立令牌断流退款，验证有实际预扣、只请求上游一次、没有成功终态和正常消费记录；既有 Responses 全资金来源退款回归通过。
- 没有数据库结构或 SQL 变更；未进行真实 MySQL/PostgreSQL 部署验收、真实 K3 能力测试或性能压测。未验证第三方上游遵守预算、工具选择、视频理解和实际首字 P95。
