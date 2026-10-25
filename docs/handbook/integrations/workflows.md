# Dify、n8n 与工作流平台

工作流平台可以使用自定义模型提供方，也可以通过通用 HTTP 节点调用本站。先用一个输入、一次模型请求、一个输出节点完成最小流程，再加入知识库、循环、定时器和业务动作。本站提供模型接口；流程编排、凭据保存、触发频率及失败重跑由工作流平台负责。

## 选择接入方式

| 平台或方式 | 配置重点 |
| --- | --- |
| Dify 模型提供方 | 选择允许自定义地址的 OpenAI 兼容提供方，核对插件版本和模型类型 |
| n8n 模型节点 | 检查当前节点或凭据是否支持自定义 Base URL；不支持时用 HTTP Request 节点 |
| Coze 等托管流程 | 仅在该产品当前版本允许自定义 HTTP 请求、鉴权与目标域名时接入 |
| 通用 HTTP 节点 | 手动配置 URL、Header、JSON Body 与返回字段 |
| 知识库 | 嵌入、重排和对话分别选择支持对应能力的模型 |

模型提供方填写 Base URL `https://meta-api.vip/v1`；HTTP 节点则填写完整端点。使用平台的凭据或受保护变量机制保存密钥，不把真实 Key 写入可共享的流程 JSON、节点名称或运行截图。没有该配置能力的版本不应通过不受支持的方式强行接入。

## HTTP 节点字段

| 字段 | 值 |
| --- | --- |
| Method | `POST` |
| URL | `https://meta-api.vip/v1/chat/completions` |
| Authorization | `Bearer` 加本站 API Key；从安全凭据读取 |
| Content-Type | `application/json` |
| Body | 下方 JSON；把 `MODEL_ID` 替换为可用模型 |
| 输出 | 普通文本响应读取 `choices[0].message.content` |

```json
{
  "model": "MODEL_ID",
  "stream": false,
  "messages": [
    {"role": "user", "content": "Summarize the supplied text in one sentence."}
  ]
}
```

先固定这段请求验证，再将用户输入作为 JSON 字符串变量传入。让节点的 JSON 序列化处理换行和引号，不用字符串拼接生成原始请求体。上游输入为空时在请求前分支处理，避免触发无意义调用。完整请求与返回结构见 [Chat Completions](/docs/reference/chat-completions)。

## 上线前验证

使用一条公开测试文本运行流程，在本站日志核对令牌和模型，再确认流程输出传给了正确节点。云端托管和本地部署的网络环境不同，浏览器能打开本站不代表工作流执行器能访问。超时后先查日志，不立即重跑整条含发送消息或写数据库的流程。

为定时任务设置明确批次、并发上限和有限重试。将模型生成与不可重复的业务动作分开，给业务记录设置自己的去重标识；本站不会因为两个提示词相同就自动合并收费请求。400 查模型与参数，401 查凭据，429 降低并发并退避，5xx 保留请求 ID 后再排查。详见[常见报错](/docs/troubleshooting)。

## 官方参考

- [Dify 模型提供方](https://docs.dify.ai/en/cloud/use-dify/workspace/model-providers)
- [n8n OpenAI 凭据](https://docs.n8n.io/integrations/builtin/credentials/openai/)
- [OpenAI 文本生成格式](https://developers.openai.com/api/docs/guides/text)
