# 结构化输出

结构化输出适合分类、信息提取和需要程序消费的结果。开始前在[模型广场](https://meta-api.vip/pricing)确认实际模型支持的协议与 JSON 输出能力，将 `MODEL_ID` 替换为对应 ID。本站可以接收相关参数，但参数存在于接口结构中不代表所有上游模型都实现了同样的约束。

## JSON Schema 示例

下面使用 Chat Completions 的 `response_format`；仅适用于明确支持 `json_schema` 的模型。Key 来自环境变量。

```bash
curl --fail-with-body -sS 'https://meta-api.vip/v1/chat/completions' \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "MODEL_ID",
    "messages": [{"role": "user", "content": "将这段反馈分类：页面打开很快，使用方便。"}],
    "response_format": {
      "type": "json_schema",
      "json_schema": {
        "name": "feedback",
        "strict": true,
        "schema": {
          "type": "object",
          "properties": {
            "sentiment": {"type": "string", "enum": ["positive", "neutral", "negative"]},
            "summary": {"type": "string"}
          },
          "required": ["sentiment", "summary"],
          "additionalProperties": false
        }
      }
    }
  }'
```

## 选择合适的约束

`response_format.type` 选择输出模式，`json_schema.name` 标识结果结构，`schema` 定义字段，`strict` 请求严格遵循模式。模型通常只支持 JSON Schema 的一个子集，复杂联合类型、递归或其他高级规则需分别确认。若模型只支持 JSON 模式，可改用其支持的 `json_object` 形式，并在提示中明确要求 JSON；这种模式不能保证字段符合指定 schema。

Responses 的输出格式属于 `text` 配置，Gemini 使用自己的生成配置，Messages 的结构化能力依模型和接口而定。不能把同一份 Chat 请求参数原样搬到所有协议。需要模型发起业务动作时，应采用[工具调用](/docs/reference/tools)，函数仍由应用执行。

## 读取与验证

成功的 Chat 响应仍是外层 completion JSON，目标结果通常位于 `choices[0].message.content`，应用需要对其中的字符串再次进行 JSON 解析和 schema 校验。对拒绝回答、空内容、长度截断以及流式中断单独处理，不能把部分 JSON 当完整业务结果。即使结构正确，也应验证金额、日期等业务含义。

400 常见原因是当前模型不支持该模式、schema 超出支持范围或参数位置错误。先去掉输出约束确认基础调用，再用简单 schema 逐步定位。不要通过无上限重试弥补不兼容；需要调整结构或切换到确实支持该能力的模型。

关联：[Chat Completions](/docs/reference/chat-completions)、[Responses](/docs/reference/responses)、[Gemini 原生 API](/docs/reference/gemini)、[流式输出](/docs/reference/streaming)。
