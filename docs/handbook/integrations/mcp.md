# 模型 API 与 MCP 工具连接

模型 API 负责生成回复，MCP 负责让应用发现和调用工具、资源等能力。两者不是同一个接口：把 `https://meta-api.vip/v1` 填进 MCP Server URL 不会变成工具服务；配置一个 MCP 服务器，也不会自动把客户端的模型提供方切换到元衡。

## 分别配置两条连接

| 连接 | 配置位置 | 凭据与地址 |
| --- | --- | --- |
| 应用 → 元衡模型 API | 客户端模型提供商设置 | Base URL `https://meta-api.vip/v1`、本站 Key、`MODEL_ID` |
| 应用 → 本地 MCP 服务 | 客户端 MCP 设置 | 本地启动命令与受控环境变量，按工具官方说明填写 |
| 应用 → 远端 MCP 服务 | 客户端 MCP 设置 | 工具提供方的 MCP 地址及其独立鉴权方式 |
| 工具结果 → 模型 | 客户端工具循环 | 客户端将必要结果作为上下文交回模型 |

本站没有统一托管 MCP 服务或任意工具代理的承诺。使用第三方图片 MCP、GCMP 或其他路由扩展前，应核对它们是否支持自定义模型地址和所需协议；仅填写本站 Key 无法改变扩展的实际连接目标。本站的图片功能见[生图指南](/docs/image-generation)，模型调用循环见[Agent 与工具](/docs/integrations/agents-tools)。

## 验证模型连接

先在不启用 MCP 的情况下确认模型配置。此请求查询本站模型目录，使用模型 API Key 环境变量：

```bash
curl --fail-with-body 'https://meta-api.vip/v1/models' \
  -H "Authorization: Bearer $API_KEY"
```

再单独启用一个来源明确、只读的 MCP 工具，检查客户端是否完成初始化并显示工具列表。下方只是初始化完成后的 JSON-RPC 消息示例，须由符合 MCP 规范的客户端通过正确传输发送，不能 POST 到本站模型端点：

```json
{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}
```

## 工具权限与数据

本地工具进程会继承你配置的权限和环境。只授予所需目录和独立凭据，不把模型 Key 作为所有远端工具的通用密码。远端工具有自己的数据处理方；发送查询、上传文件或触发外部动作前，确认数据范围及接收方。模型建议调用工具不等于用户已经授权不可逆操作，最终执行控制应放在客户端。

## 分层排查

工具列表为空时检查 MCP 进程、初始化、传输方式与工具鉴权，不先更换模型 Key；工具列表存在但模型不调用时检查模型工具支持和客户端是否把工具声明发给模型；工具已执行却没有答案时检查工具结果是否完整回传、是否超出上下文，以及调用 ID 是否对应。文件路径、工具端点和 API Base URL 分开记录，便于定位是哪条连接出错。排查日志不要包含密钥或完整敏感文件。

## 官方参考

- [MCP 架构：Host、Client 与 Server](https://modelcontextprotocol.io/docs/learn/architecture)
- [OpenAI 工具调用流程](https://developers.openai.com/api/docs/guides/function-calling)
