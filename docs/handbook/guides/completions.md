# 文本补全与 FIM

传统 Completions 使用 `prompt`，Chat Completions 使用 `messages`。FIM 用前缀和后缀约束中间生成内容，常用于代码补全；它是特定模型的能力，不是所有聊天模型通用功能。

## 最小补全请求

选择明确支持 `/v1/completions` 的模型，将 `MODEL_ID` 替换为实际 ID。密钥通过环境变量提供。

```bash
curl --fail-with-body https://meta-api.vip/v1/completions \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"model":"MODEL_ID","prompt":"def add(a, b):\n    ","max_tokens":64}'
```

| 字段         | 说明                          |
| ------------ | ----------------------------- |
| `model`      | 支持补全协议的模型 ID         |
| `prompt`     | 前缀或待续写内容              |
| `suffix`     | 仅在模型支持 FIM 时传入的后缀 |
| `max_tokens` | 生成长度上限，仍受模型限制    |
| `stream`     | 是否按受支持的流式格式返回    |

响应通常从 `choices[].text` 读取文本，同时检查 `finish_reason` 和 `usage`。不要按聊天响应的 `message.content` 解析。

## 使用中间填充

当模型明确支持 `suffix` 时，把插入点之前的内容放入 `prompt`，之后的内容放入 `suffix`。先用短的、语法完整的上下文验证；保留缩进和换行，避免客户端预处理破坏代码结构。

例如需要补齐函数主体时，可将以下请求体用于同一个补全端点；必须先将模型占位符替换为已确认支持 FIM 的模型。

```json
{
  "model": "MODEL_ID",
  "prompt": "def add(a, b):\n    ",
  "suffix": "\n\nassert add(1, 2) == 3\n",
  "max_tokens": 64
}
```

将 `choices[0].text` 作为中间片段拼到前缀与后缀之间，再检查语法与测试结果。模型可能输出多余缩进、后缀重复内容或未完成片段，应用需要检测这些情况，不能把任意返回文本直接保存执行。`finish_reason` 表示长度终止时，应先检查补全是否完整。

部分渠道会把补全请求适配到特定上游路径，调用方仍使用本站地址。不要自行把参考平台的 beta 地址或厂商密钥混入配置。

## 常见问题

400 或不支持接口时，确认模型是否为补全模型，删除不支持的采样参数。只有聊天能力的模型可以通过对话形式请求补全，但输出格式、停止规则和成本并不等同于 FIM。

生成代码必须经过语法检查、测试与审查。不要让 IDE 无条件执行补全文本中的 shell 命令。已有 IDE 接入方式见[IDE 与编程助手](/docs/integrations/ide-assistants)。
