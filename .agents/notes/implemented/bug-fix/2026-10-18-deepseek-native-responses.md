# Agent Note: DeepSeek 原生 Responses 路由与无状态工具续接

Status: implemented

## Problem

DeepSeek 专用渠道声明支持 Responses，但转换函数返回 not implemented，地址选择也把 Responses 落到 Chat Completions。官方当前已支持 deepseek-flash 的原生 Responses；同 Key 的 Chat 成功不能证明这条路径可用。外部提供的 UUID 和 502 尚未对应运行记录，不能把代码缺口当作该次请求的已证实根因。

## Existing capabilities and notes audit

复用 router/relay-router.go → controller.Relay → relay.ResponsesHelper 的鉴权、选渠、映射、重试和结算，以及 openai 的原生 Responses JSON/SSE 处理器。Input 与 Tools 使用 RawMessage，可保留 reasoning、function_call 和 function_call_output 的完整历史，无须转 Chat 或新增数据库。

检索活跃笔记后，[上游能力切换](../../implemented/bug-fix/2026-10-12-upstream-capability-failover.md)和[空输出诊断](../../implemented/bug-fix/2026-10-13-responses-empty-output-no-shared-cooldown.md)部分重叠，继续持有已输出不重放、失败退款和日志隔离边界；Kimi 参数、用量及超时笔记不持有 DeepSeek 原生协议决定，无需替换。

## Decision

- DeepSeek 渠道 Responses 路由使用配置基址追加 /responses，保留显式 /v1 前缀，去除尾斜杠。Chat、Messages、FIM 地址保持原行为。
- 实现原生请求转换，保留 input/tools/reasoning/max_output_tokens；模型后缀 none/max 与现有 DeepSeek 语义一致，新增 deepseek-flash 元数据并保留旧名称。
- 复用现有 JSON/SSE 处理器，覆盖文本、推理和工具事件及末尾用量；Responses stream_options 声明改为不支持，并在最终出站体移除，覆盖透传与参数覆盖。
- 官方 Responses 无状态：完整历史续接正常转发，非空 previous_response_id/conversation 明确返回 400 且禁止重试，防止被上游静默忽略。最终出站校验同时覆盖透传与参数覆盖；不增加会话存储。既有 Chat 转 Responses 会话优化对 DeepSeek 跳过存储和匹配，防止网关自行缩减完整历史。
- 保留上游 HTTP 状态、错误消息与诊断；不新增统一 502 包装。共享非流式 Responses 处理器识别只有 code/message 的错误及 failed 状态，不将 HTTP 200 的失败对象写为成功；沿用 SSE 错误映射并禁止重放，避免重复执行工具。非流式 incomplete 的既有语义不改变。修复不改变生产渠道、价格、权限或数据库。

## Alternatives considered

- Responses 转 Chat 可覆盖仅支持 Chat 的第三方，但官方已有原生接口，转换会增加工具、推理和流式状态的损失风险，故本次接原生协议。
- 切换通用 OpenAI 渠道配置可绕开未实现函数，但无法修复专用渠道，也不能统一无状态边界和地址规则，因此仅作为独立运维选项。
- 网关持久化历史可支持 previous_response_id，但引入租户隔离、TTL、存储与跨渠道绑定；当前完整历史已能续接，本次明确拒绝无法恢复的引用。

## Verification

- 真实本地 HTTP 桩通过完整 controller.Relay → ResponsesHelper → DeepSeek → HTTP → 原生 JSON/SSE 路径验证原始非流式示例、模型映射与后缀、四档 reasoning.effort、工具首轮纯工具/推理输出、两轮完整历史与 call_id 原样保留。分别覆盖流式/非流式及透传；末尾缓存和推理用量在客户端响应保持原样。
- 最终出站体回归覆盖 stream_options 清理、参数覆盖和透传、未知大整数与 opaque input 保真。非空 previous_response_id / conversation 本地400零出站且禁止重试；Chat 会话优化不丢历史。HTTP 400/502 保留上游状态与具体错误，流式失败/不完整在推理输出后不重放、不产生成功终态。
- `GOMAXPROCS=2 go test -p 2 -timeout 180s ./relay/... ./controller ./service/... ./middleware ./dto ./setting/... ./types ./router` 通过；后续会话保护由 service/controller 定向竞态回归覆盖。
- `GOMAXPROCS=2 go test -race -parallel 1 -p 2 -timeout 180s ./relay/channel/deepseek ./relay/common ./controller ./service -run 'TestDeepSeek|TestRelayDeepSeek|TestResolveChannelCapabilities|TestResponsesSessionBridge|TestRelayResponsesInterruptedOutputRefundsAllFundingSources' -count=1` 通过，包含现有钱包/订阅/令牌失败退款回归。
- 最终补充的 `go test ./relay/channel/openai ./controller ./service` 与上述非流式失败/DeepSeek/退款定向竞态检查通过。Go 全包构建通过，带版本二进制自报 v1.0.0-compat.45；Bun 图片工作台/主站构建与系统 Chrome 五个公开页预渲染通过，make verify-notes 校验 47 篇通过，git diff --check 通过。
- 未执行供应商付费调用或生产部署。GitHub Release/镜像与本地验证分开核对，不以触发工作流代替发布完成。

## Consequences

DeepSeek 原生 Responses 可复用既有转发与计费保护，不再需要协议转换；代价是最终出站体增加一次保留 RawMessage 的校验/重编码，且无状态续接要求客户端回传完整历史。新增模型名不自动修改已配置渠道与价格，旧名称保留。

DeepSeek 官方不支持服务端会话和完整 OpenAI 内置工具集合。第三方基址必须提供相同原生协议；不把任意代理域名当作支持证明。真实账号调用及用户给出的请求编号仍未验证。只有明确需要跨请求服务端历史引用时，才重新评估独立的租户隔离、存储和过期机制。

官方依据（2026-10-18 核对）：https://api-docs.deepseek.com/guides/responses_api/ 、 https://api-docs.deepseek.com/api/create-response/ 。
