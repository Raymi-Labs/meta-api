# Claude Messages

`POST https://meta-api.vip/v1/messages` 接收 Claude Messages 格式。适用于支持该协议的模型和客户端。模型名称、分组和可选能力需在[模型广场](https://meta-api.vip/pricing)核对；把 `MODEL_ID` 替换为实际 ID，不要把 Chat、Responses 的请求体直接提交到此端点。

## 最小请求

在终端环境中设置 `API_KEY`。下面的 max_tokens 是示例输出预算，可按任务与模型限制调整；不是本站规定的统一额度。

```bash
curl --fail-with-body -sS 'https://meta-api.vip/v1/messages' \
  -H "x-api-key: $API_KEY" \
  -H 'anthropic-version: 2023-06-01' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "MODEL_ID",
    "max_tokens": 512,
    "system": "请用简洁中文回答。",
    "messages": [{"role": "user", "content": "解释流式输出。"}]
  }'
```

响应的 `content` 是内容块数组，文本块通常使用 `type: "text"` 与 `text`。不要只读取 Chat 格式的 `choices`。`stop_reason` 表示完成、达到输出限制或工具调用等情况；`usage` 可能包括输入、输出及缓存相关字段。

## 参数与内容块

| 字段 | 说明 |
| --- | --- |
| messages | user/assistant 历史；工具结果也在消息内容中表达 |
| system | 顶层系统提示，不能随意改为 system 角色消息 |
| max_tokens | 本次输出预算；思考模型还可能有额外约束 |
| stream | 开启 Messages SSE 事件流 |
| tools | 名称、说明和 input_schema；工具由应用执行 |
| tool_choice | auto、指定工具等策略，按模型支持使用 |
| thinking | 仅在对应模型与渠道明确支持时添加 |

多轮调用要保留必要历史和工具调用 ID。模型请求工具时，将执行结果与原 tool_use_id 对应后返回；不要把工具输出伪装成新的普通问题。对于包含思考块或签名的历史，不要擅自重写结构；跨协议转换并不保证所有扩展语义都能保留。

## 常见问题

400 应先核对 model/messages、max_tokens、内容块类型和工具结果顺序。工具选择和思考预算也可能受到上游限制。若客户端需要特定 beta 功能，先确认渠道支持，不应通过随意拼接头部“开启”未提供的能力。

流式响应需要按事件消费，正确处理内容块开始、增量、结束及 message_stop；中断或错误事件不能伪装为成功完成。若调用失败，保留 HTTP 状态、错误类型与请求 ID，结合使用日志定位。

关联：[工具调用](/docs/reference/tools)、[流式输出](/docs/reference/streaming)、[Claude Code](/docs/clients/claude-code)、[错误排查](/docs/troubleshooting)。
