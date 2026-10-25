# 桌面与 Web 聊天客户端

Cherry Studio、Chatbox、RikkaHub、LobeChat、LobeHub、Open WebUI、NextChat、LibreChat 等客户端通常把模型连接与聊天界面分开管理。请在当前版本中寻找“模型服务”“提供商”或“连接”设置，选择允许自定义 URL 的 OpenAI 兼容连接。只能连接其他平台的专属插件不适用于本站令牌。

## 添加连接

| 字段 | 本站 OpenAI 兼容连接 |
| --- | --- |
| 名称 | 自定义为便于识别的“元衡” |
| 协议 / Provider | OpenAI Compatible，具体标签以当前版本为准 |
| Base URL | `https://meta-api.vip/v1` |
| API Key | 本站创建的独立令牌；脚本中对应 `API_KEY` |
| 模型 ID | `MODEL_ID`，替换为该令牌可用的模型名称 |
| 显示名称 | 可自定义，不能替代发给接口的模型 ID |

保存连接后，启用它并添加模型；在新建对话中明确选中这个连接下的模型。部分客户端不会自动列出第三方模型，此时应手动添加精确 ID。自动获取列表失败并不一定代表生成接口不可用，可先检查模型目录：

```bash
curl --fail-with-body 'https://meta-api.vip/v1/models' \
  -H "Authorization: Bearer $API_KEY"
```

## 各类客户端的差异

| 场景 | 配置与验收重点 |
| --- | --- |
| Cherry Studio、Chatbox、RikkaHub | 连接保存后，还需在对话中选择新增模型 |
| Open WebUI、LibreChat、NextChat、LobeChat / LobeHub | 由管理员配置服务器连接或个人连接；确认请求实际从哪个服务端发出 |
| AnythingLLM 等知识库应用 | 对话与[嵌入](/docs/reference/embeddings)分别配置；更换嵌入模型后通常需重新建立索引 |
| ChatAll、ChatHub 等多模型客户端 | 先只启用一个模型，避免并发比较产生多笔调用 |
| Open Design、Claude Desktop 等 Agent 应用 | 先查产品是否开放自定义模型连接；挂载 MCP 工具不等于替换内置模型 |

Web 客户端如果由服务端转发，请确认部署机器能访问本站。浏览器直接请求的客户端还受其安全策略与跨域设计影响；不要通过公开页面暴露共享管理密钥。本站 API Key 与客户端自己的订阅是两套独立权限。

## 最小验收与排查

先关闭知识库、联网、插件和图片附件，发送一条短文本。在本站使用日志中核对模型、时间和令牌，再逐项开启附加能力。文本成功而图片失败时，按[图片与视频理解](/docs/reference/vision)确认模型视觉能力及客户端发送的图片格式；知识库无结果时，分别检查嵌入请求和文档索引。404 常见于重复追加 `/v1` 或填入完整端点；401 则检查密钥是否粘贴完整及令牌是否停用。更多见[常见报错排查](/docs/troubleshooting)。

## 官方参考

- [Chatbox 自带 API Key 指南](https://chatboxai.app/en/guide/byok)
- [Open WebUI 连接提供商](https://docs.openwebui.com/getting-started/quick-start/connect-a-provider)
- [Cherry Studio 官方文档](https://docs.cherryai.com.cn/)
