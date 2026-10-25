# API 接入总览

元衡 API 将已配置的模型渠道接入同一服务入口。先在[模型广场](https://meta-api.vip/pricing)查看目标模型的 ID、可用分组与支持协议，再创建对应分组的 API Key。页面出现某个厂商名称，不代表该厂商的每种接口、工具或模型都已开放；同一模型也可能只支持其中一种协议。

## 地址与请求方式

API 服务根地址为 `https://meta-api.vip`。OpenAI SDK 通常填写 `https://meta-api.vip/v1`；原生 HTTP 请求需带下面的完整路径。`docs.meta-api.vip` 只提供文档，不接收模型生成请求。

| 用途 | 方法与路径 | 核心输入 |
| --- | --- | --- |
| 模型列表 | `GET /v1/models` | API Key |
| Chat | `POST /v1/chat/completions` | model、messages |
| Responses | `POST /v1/responses` | model、input |
| Claude Messages | `POST /v1/messages` | model、messages |
| Gemini | `POST /v1beta/models/MODEL_ID:generateContent` | contents |
| 图片生成、编辑 | `POST /v1/images/generations`、`/v1/images/edits` | model、提示词或图片 |
| 音频 | `POST /v1/audio/speech`、`/v1/audio/transcriptions`、`/v1/audio/translations` | 文本或音频 |
| 向量、重排 | `POST /v1/embeddings`、`/v1/rerank` | input 或 query/documents |
| 视频任务 | `POST /v1/videos` | model、prompt 等 |

## 先验证认证

在当前终端设置好 `API_KEY` 环境变量后执行。不要将真实密钥放进截图、公开仓库或网页前端。

```bash
curl --fail-with-body -sS 'https://meta-api.vip/v1/models' \
  -H "Authorization: Bearer $API_KEY"
```

成功响应通常包含 `data` 模型数组。生成示例中的 `MODEL_ID` 是占位符，必须替换为模型广场中支持相应协议的模型名；不能把它原样提交。模型列表成功只能证明认证与目录读取正常，不能替代真实生成测试。

## 兼容范围

目前不提供 Files、Fine-tuning、Images Variations、Assistants、Threads、Vector Stores 或通用 Batch 管理接口。Responses 后台运行及其查询/删除配套接口未提供；Gemini `countTokens` 也不支持。视频轮询是特定任务能力，不能推导为所有接口都可异步运行。参数能进入网关，不等于上游一定接受。

遇到失败时保留 HTTP 状态、错误正文和 `X-Oneapi-Request-Id`。401 先查密钥，403 查权限或分组，404 查路径，400 查协议和字段；不要通过反复更换模型名掩盖配置问题。

继续阅读：[认证与密钥](/docs/reference/authentication)、[模型与分组](/docs/reference/models)、[Chat Completions](/docs/reference/chat-completions)、[常见报错](/docs/troubleshooting)。
