# Responses API

Responses 使用 `POST https://meta-api.vip/v1/responses`，以 `input` 而非 Chat 的 `messages` 作为输入。它适合支持该协议的文本、多模态和工具流程。请先在[模型广场](https://meta-api.vip/pricing)确认协议及令牌分组，再将 `MODEL_ID` 替换为实际模型；不要因模型支持 Chat 就默认支持 Responses。

## 最小请求

```bash
curl --fail-with-body -sS 'https://meta-api.vip/v1/responses' \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "MODEL_ID",
    "instructions": "用简明中文回答。",
    "input": "给一个 API 错误排查的起点。",
    "stream": false
  }'
```

`API_KEY` 只从终端环境读取。原始 HTTP 响应的内容在 `output` 项目及其内容块中；Python SDK 的 `output_text` 是便利聚合属性，不能假设原始 JSON 一定有同名字段。输出也可能包含工具请求、推理摘要或其他类型，应用要按 type 分支处理。

## 主要参数与状态

| 字段 | 用法 |
| --- | --- |
| input | 文本或符合 Responses 格式的输入项目；必填 |
| instructions | 本次请求的指令，不等同于 Chat 的 system 消息结构 |
| max_output_tokens | 输出预算；具体可用范围由模型决定 |
| reasoning | 模型支持时的推理配置，不能用于所有模型 |
| tools / tool_choice | 工具声明和选择；函数工具仍由应用执行 |
| previous_response_id | 续接依赖上游保存的响应状态，可能受渠道与保存设置影响 |
| stream | 使用 Responses 事件流，不能照搬 Chat delta 解析器 |

如果使用完整历史自行管理上下文，应保留工具调用与结果的对应关系。使用 previous_response_id 时，要处理响应状态不存在或已失效的错误；不能假设任意备用渠道都持有同一状态。`store` 由渠道设置及上游能力共同决定，不是本站全站数据保留承诺。

## 支持边界与错误

本站提供 `POST /v1/responses/compact` 路由，但是否可用取决于对应模型与渠道；压缩结果应按原类型保存，不擅自删掉结构字段。当前没有通用 Responses 后台运行、GET 查询、DELETE 删除接口，不要设置 background 后再等待不存在的轮询入口。

400 常见于缺少 input、混用了 messages、工具结果对应关系错误或不支持的参数。流式时需识别 completed、failed、incomplete 等终态；拿到 HTTP 200 或部分文本不等于完整成功。错误排查应保留请求 ID 和事件类型，避免提交完整密钥或私密正文。

关联：[流式输出](/docs/reference/streaming)、[工具调用](/docs/reference/tools)、[API 总览](/docs/reference/overview)、[Codex 接入](/docs/clients/codex)。
