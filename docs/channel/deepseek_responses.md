# DeepSeek 原生 Responses

DeepSeek 专用渠道支持客户端 `POST /v1/responses`，转发到渠道基址的 `/responses`，复用现有鉴权、分组/令牌权限、模型映射、用量及失败退款流程。该能力从 `v1.0.0-compat.45` 起提供。

## 渠道设置

- 渠道类型：DeepSeek。
- 官方基址：`https://api.deepseek.com`；上游出站为 `https://api.deepseek.com/responses`。
- 模型：在渠道和对应分组启用 `deepseek-flash`，配置其价格。旧模型名称仍保留，不自动改写已有渠道。
- 公司自定义模型名可通过原有模型映射映射到 `deepseek-flash`。映射和参数覆盖沿用既有规则；开启请求体透传时不会自动应用模型映射。
- 第三方基址必须提供原生 Responses。同一 Key 的 Chat 成功不证明它支持 Responses；仅支持 Chat 的上游需要另外的转换实现。

## 请求与推理

```json
{
  "model": "deepseek-flash",
  "input": "Reply OK.",
  "max_output_tokens": 256,
  "reasoning": {"effort": "none"},
  "stream": false
}
```

`input` 支持字符串或完整输入项列表。`reasoning.effort` 原样保留，官方支持 `none/low/high/max`；省略时采用上游默认行为。`max_output_tokens` 同时包含推理和正文预算，开启推理后较小预算可能产生截断，网关不自动提高预算。

`deepseek-flash-none`、`deepseek-flash-max` 是网关后缀别名，实际发送 `deepseek-flash` 并分别设置 `effort=none/max`；后缀与显式 effort 冲突时沿用后缀优先。已有 `deepseek-v4-*` 后缀语义保持不变。

`stream:true` 使用 Responses SSE，保留推理文本、正文和工具参数事件，接收末尾 `response.completed` 的用量，不依赖 `[DONE]`。DeepSeek 不支持 Responses `stream_options`，在最终出站体中移除，覆盖参数覆盖及请求体透传。失败或不完整终态沿用既有明确失败、输出后不重放及退款保护。

## 工具续接

DeepSeek Responses 是无状态 API。客户端应保留前一轮的完整 `output`，包括 `reasoning.content` 和 `function_call`，将其追加到原始 `input`，再追加对应的工具执行结果。例如：

```json
{
  "model": "deepseek-flash",
  "input": [
    {"role": "user", "content": "调用 echo 后回复结果。"},
    {"type": "reasoning", "content": [{"type": "reasoning_text", "text": "此处为上一轮原样返回的推理内容"}]},
    {"type": "function_call", "call_id": "call_example", "name": "echo", "arguments": "{\"value\":\"OK\"}"},
    {"type": "function_call_output", "call_id": "call_example", "output": "OK"}
  ],
  "tools": [{"type": "function", "name": "echo", "parameters": {"type": "object", "properties": {"value": {"type": "string"}}}}],
  "reasoning": {"effort": "high"}
}
```

示例中的推理和工具调用由上一轮实际输出替换，`call_id` 必须对应。网关只转发工具协议，工具执行由客户端完成。非空 `previous_response_id` 或 `conversation` 返回明确的 400，提示回传完整历史，不发送到上游，也不切换渠道重试。Chat 转 Responses 的既有会话优化在 DeepSeek 上不缓存或缩减历史。

官方内置工具支持范围不是完整 OpenAI 工具集合；不能以函数工具回归通过推断任意内置工具、服务端会话或 `/responses/compact` 可用。

## 验证与诊断边界

本地回归使用真实 HTTP 协议桩通过完整 Relay 处理器，覆盖原始非流式示例、流式、四档推理、映射、两轮工具续接、透传/参数覆盖、无状态边界和原始上游 400/502。共享回归覆盖失败退款，实际供应商账号与生产请求仍需上线后独立验证。

HTTP 上游错误继续保留具体状态和错误信息，管理员诊断沿用上游地址、请求 ID 和尝试渠道；不增加统一 502 包装。非流式 HTTP 200 中的明确错误/failed 对象按失败处理并保留原因，禁止重试，不记为成功。本地通过不代表已定位外部网关给出的 UUID，也不代表已部署生产。

官方文档（2026-10-18 核对）：[Responses 指南](https://api-docs.deepseek.com/guides/responses_api/)、[请求与参数](https://api-docs.deepseek.com/api/create-response/)。
