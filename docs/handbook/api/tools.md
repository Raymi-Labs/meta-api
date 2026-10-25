# 工具调用

工具调用用于让模型提出结构化的操作请求，例如查询业务数据或计算结果。先在[模型广场](https://meta-api.vip/pricing)选择支持工具调用及对应协议的模型，将 `MODEL_ID` 替换为实际 ID。本站负责转发和协议适配；**函数工具由你的应用执行**，提交函数声明不会让网关自动访问数据库、发邮件或运行命令。

## 声明一个函数

下面的 Chat Completions 请求声明只读温度查询函数。`API_KEY` 通过环境变量提供，模型可以返回文本，也可以请求调用函数。

```bash
curl --fail-with-body -sS 'https://meta-api.vip/v1/chat/completions' \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "MODEL_ID",
    "messages": [{"role": "user", "content": "查询上海当前温度。"}],
    "tools": [{
      "type": "function",
      "function": {
        "name": "get_temperature",
        "description": "查询指定城市的当前温度",
        "parameters": {
          "type": "object",
          "properties": {"city": {"type": "string"}},
          "required": ["city"],
          "additionalProperties": false
        }
      }
    }],
    "tool_choice": "auto"
  }'
```

## 完成调用循环

读取 `choices[0].message.tool_calls`，保存整个 assistant 消息。每项包含调用 `id`、函数名称及 JSON 字符串形式的 `arguments`。应用解析参数，检查函数白名单、参数范围和当前用户权限，再执行实际函数。随后将原 assistant 消息和 `role: "tool"` 的结果加入历史；结果消息的 `tool_call_id` 必须对应原调用 ID，`content` 可用序列化后的结果文本。再次发送消息，模型才可根据真实结果继续回答。

工具名称和参数来自模型，不能直接拼接为 SQL 或 shell 命令。涉及付款、删除或对外发送等业务动作，应由应用实施自己的确认和权限流程。为循环设置最大轮次，并给执行动作配置去重标识，防止网络重试造成重复操作。

## 协议差异与常见错误

Responses 使用自己的 function call 与结果项，Messages 使用 `tool_use` / `tool_result` 内容块；不要把 Chat 的消息结构直接发送到其他协议。流式参数应拼接完成后执行。400 常见于模型不支持工具、schema 无效或结果缺少对应调用 ID；JSON 解析失败时不要执行不完整参数。`tool_choice`、并行调用和严格 schema 的支持范围取决于模型及渠道；内置联网搜索等托管工具不能由普通函数声明推导为可用。

关联：[Chat Completions](/docs/reference/chat-completions)、[Responses](/docs/reference/responses)、[Messages](/docs/reference/messages)、[结构化输出](/docs/reference/structured-output)。
