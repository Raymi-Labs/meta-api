# Chat Completions

`POST https://meta-api.vip/v1/chat/completions` 适合基于消息列表的对话、摘要、文本生成和支持该协议的工具调用。先在[模型广场](https://meta-api.vip/pricing)确认模型支持 Chat Completions，把示例的 `MODEL_ID` 替换为实际 ID，并使用有权访问对应分组的 Key。

## 最小请求

`API_KEY` 由终端环境提供。非流式响应成功后，主要读取 `choices[].message`；返回的 `usage` 用于理解本次模型用量，最终费用以本站使用日志为准。

```bash
curl --fail-with-body -sS 'https://meta-api.vip/v1/chat/completions' \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "MODEL_ID",
    "messages": [
      {"role": "system", "content": "请准确、简洁地回答。"},
      {"role": "user", "content": "解释什么是 API。"}
    ]
  }'
```

## 关键字段

| 字段 | 含义与注意事项 |
| --- | --- |
| model | 公开模型 ID，不能省略或用展示名称代替 |
| messages | 按时间顺序排列的消息，至少包含实际输入 |
| stream | 开启后响应变成 SSE，不能按普通 JSON 一次性解析 |
| max_tokens / max_completion_tokens | 输出预算字段，按模型要求选择，避免同时填写冲突值 |
| temperature / top_p | 采样控制，仅在模型支持时设置 |
| tools / tool_choice | 声明可调用工具及选择策略，工具由应用执行 |
| response_format | JSON 或结构化输出约束，依模型能力生效 |

多轮对话通常需要由应用保存并再次提交必要历史。不能假设相同 Key 会让服务器自动记住上一轮。历史、系统提示、图片和工具结果都会影响上下文占用；需要控制长度，避免不断追加无关内容。

## 如何判断完成

纯文本通常在 `choices[0].message.content`；工具调用可能出现在 `tool_calls`，此时 content 为空不一定失败。结合 `finish_reason` 判断正常结束、长度截断或等待工具。应用应处理空 choices、错误对象及非预期返回结构，不能直接把整段响应当展示文本。

400 常见于参数不支持、消息格式错误或上下文超限；改用最小请求确认后逐项增加参数。401 查 Key，429 降低并发并退避；连接中断后先看使用日志，不要无限重放可能已经完成的请求。

关联：[流式输出](/docs/reference/streaming)、[工具调用](/docs/reference/tools)、[结构化输出](/docs/reference/structured-output)、[图片理解](/docs/reference/vision)。
