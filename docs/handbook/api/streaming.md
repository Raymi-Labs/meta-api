# 流式输出

流式响应让应用在生成过程中逐步接收结果，适合聊天和长文本展示。先在[模型广场](https://meta-api.vip/pricing)确认模型及所选协议支持流式调用，再把 `MODEL_ID` 替换为实际 ID。流式主要改变传输和解析方式，不代表更低价格、更长上下文或无限连接时长。

## 最小请求

下面使用 Chat Completions。`API_KEY` 从环境变量读取，`-N` 关闭 cURL 的输出缓冲，便于观察服务端事件。

```bash
curl --fail-with-body -N 'https://meta-api.vip/v1/chat/completions' \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "MODEL_ID",
    "messages": [{"role": "user", "content": "用三句话介绍流式响应。"}],
    "stream": true
  }'
```

## 按协议解析

| 协议 | 请求方式 | 常见读取位置 |
| --- | --- | --- |
| Chat Completions | JSON 中 `stream: true` | SSE 的 `data`，正文增量通常在 `choices[].delta.content` |
| Responses | JSON 中 `stream: true` | 按事件类型处理文本、工具与终止状态 |
| Messages | JSON 中 `stream: true` | 按事件类型处理内容块、增量和结束事件 |
| Gemini | `:streamGenerateContent?alt=sse` | 逐事件读取 candidates 中的内容 |

SSE 事件之间以空行分隔。一次网络读取可能包含半个事件或多个事件，不能按网络分块直接执行 JSON 解析。Chat 的 `data: [DONE]` 不是 JSON；也不能拿它作为所有协议的通用结束标记。工具参数可能被拆成多个增量，应先按调用标识拼接完整，再校验和执行。

部分模型支持 `stream_options` 返回用量，另一些模型会拒绝该字段。没有收到用量事件不等于没有消耗，结算应查询本站使用日志。收到第一段文字也不代表最终成功，应用仍需处理错误事件、长度截断、连接中断及协议规定的终止状态。

## 连接与错误处理

客户端、反向代理和部署平台都应允许持续读取响应，避免额外缓冲。取消请求应关闭连接，但已经发生的上游计算不一定被撤销。首次响应前的 401、400、429 按普通 HTTP 错误处理；开始输出后的故障应保留已显示内容并提示生成未完成。重试前核对日志，尤其不要自动重复已经触发业务动作的工具调用。

关联：[Chat Completions](/docs/reference/chat-completions)、[Responses](/docs/reference/responses)、[Messages](/docs/reference/messages)、[工具调用](/docs/reference/tools)。
