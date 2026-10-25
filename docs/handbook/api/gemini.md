# Gemini 原生 API

Gemini 原生接口使用 `contents/parts` 表达输入，路径中包含模型名与动作。先在[模型广场](https://meta-api.vip/pricing)确认模型支持 Gemini 原生协议，并检查令牌分组；将 URL 中的 `MODEL_ID` 替换为实际模型 ID。使用 OpenAI 兼容调用时则应按 Chat Completions 文档构造请求，两种格式不能混用。

## 普通生成请求

```bash
curl --fail-with-body -sS \
  'https://meta-api.vip/v1beta/models/MODEL_ID:generateContent' \
  -H "x-goog-api-key: $API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "contents": [{
      "role": "user",
      "parts": [{"text": "用两句话介绍多模态模型。"}]
    }]
  }'
```

`API_KEY` 从环境变量读取。通常从 `candidates[].content.parts[]` 中提取文本或其他内容，结合 `finishReason`、安全相关字段和 `usageMetadata` 判断结果。没有候选文本时，先检查结束原因和错误信息，不要直接将空数组视为网络失败。

## 常见字段

| 字段 | 说明 |
| --- | --- |
| contents | 按顺序提供 user/model 对话内容 |
| parts | 文本、图片等内容块，类型需与模型能力匹配 |
| systemInstruction | 系统指令，与 contents 分开填写 |
| generationConfig | 输出预算、采样及支持的输出格式设置 |
| tools / toolConfig | 函数声明与调用策略；函数由应用执行 |
| safetySettings | 模型支持的安全设置，不能绕过平台或上游政策 |

流式生成可使用同一路径的 `:streamGenerateContent?alt=sse`。客户端要按 SSE 消费多段候选内容，而不是等连接关闭后再一次性解析 JSON。多模态输入中，应按协议正确标记 MIME 类型；远程资源还需上游能够访问，不能假设本机文件路径会自动上传。

## 当前边界

本站未提供 Gemini `countTokens`，也没有完整的 Gemini 文件上传或缓存资源管理 API。请求中存在 cachedContent 字段不等于平台能替你创建该资源；依赖外部资源标识时还受上游账号与渠道状态限制。不要把另一个服务生成的文件、缓存或操作 ID 直接假设为可移植。

400 常见于 parts 格式、模型动作或生成参数不匹配；404 先检查 `/v1beta/models/` 与动作名；401 检查密钥；429 需要控制并发并退避。遇到安全拦截时，应阅读返回原因并调整合法输入，不能依靠更换协议规避限制。

关联：[模型选择](/docs/reference/models)、[流式输出](/docs/reference/streaming)、[图片理解](/docs/reference/vision)、[Gemini CLI](/docs/clients/gemini)。
