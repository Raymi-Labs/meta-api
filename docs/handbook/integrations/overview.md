# 应用与开发集成总览

元衡提供模型接口，客户端负责对话界面、工作流和本地工具。接入前先确认客户端允许填写自己的服务地址、密钥和模型名称，再在[API 接入总览](/docs/reference/overview)核对该模型的协议。能够发送普通对话，不代表同时支持图像、工具调用、自动补全或实时语音。

## 按使用场景选择入口

下表覆盖常见开发工具和应用类别。它是接入条件与文档入口的对照，不表示所有客户端版本或付费功能都经过本站实测。

| 类别 | 工具与适配入口 | 先核对什么 |
| --- | --- | --- |
| 终端编程 | [Codex](/docs/clients/codex)、[Claude Code](/docs/clients/claude-code)、[Claude Code 使用 OpenAI / Codex 模型](/docs/clients/claude-code-openai) | Responses 或 Messages 协议 |
| 命令行与网关 | [Gemini CLI](/docs/clients/gemini)、[OpenCode](/docs/clients/opencode)、[OpenClaw](/docs/clients/openclaw)、[CC Switch](/docs/clients/ccswitch) | 按各篇填写地址，不混用协议 |
| 其他编程助手 | CodeBuddy、WorkBuddy、DeepSeek Harness、Crush、Gptme、Hermes Agent、Qwen Code、Kilo Code；见 [Agent 接入](/docs/integrations/agents-tools) | 是否有自定义提供商及所需工具能力 |
| 编辑器 | Cursor、Cline、Continue、Roo Code、Avante.nvim；见 [IDE 助手](/docs/integrations/ide-assistants) | 聊天、补全和 Agent 分别配置 |
| 工作流 | Dify、n8n、Coze；见 [工作流平台](/docs/integrations/workflows) | 凭据、联网地址、节点输入输出 |
| 代码框架 | OpenAI SDK、AI SDK、LangChain、LiteLLM；见 [SDK 与 HTTP](/docs/integrations/sdks) | SDK 使用的端点与模型协议一致 |
| 聊天与知识库 | Cherry Studio、Chatbox、RikkaHub、LobeChat、LobeHub、Open WebUI、NextChat、AnythingLLM、LibreChat、Chatx、ChatAll、ChatHub；见 [聊天客户端](/docs/integrations/chat-clients) | 自定义 URL；知识库另配嵌入模型 |
| 设计与桌面 Agent | Open Design、Claude Desktop | 先确认官方支持的模型配置方式；MCP 不等于替换模型服务 |
| 写作与学术 | Obsidian、NoteGen、Zotero、GPT-Academic、FeelFish；见 [应用配置](/docs/integrations/applications) | 插件提供方、发送范围与文档长度 |
| 翻译 | 沉浸式翻译、NextAI Translator、Pot、欧路翻译；见 [应用配置](/docs/integrations/applications) | 完整端点或 Base URL 的字段区别 |
| 业务与辅助工具 | cc-connect、懒人客服、uTools、All API Hub | 用独立令牌；通知、余额查询和导出功能另行核对 |
| MCP 与路由 | MCP 客户端、GCMP 等路由工具；见 [MCP 边界](/docs/integrations/mcp) | 工具服务与模型服务分开配置 |

## 通用配置字段

| 字段 | OpenAI 兼容对话的填写方式 |
| --- | --- |
| 提供商 | OpenAI Compatible 或允许自定义地址的等价选项 |
| Base URL | `https://meta-api.vip/v1` |
| 完整对话端点 | `https://meta-api.vip/v1/chat/completions`，仅用于明确要求完整端点的字段 |
| API Key | 本站令牌；命令行通过 `API_KEY` 环境变量读取 |
| Model | 将 `MODEL_ID` 替换为该令牌可用且支持对应协议的模型名 |

## 从最小验证开始

先创建一个用途明确、额度适当的令牌。下面只检查令牌模型目录，不发起模型生成：

```bash
curl --fail-with-body 'https://meta-api.vip/v1/models' \
  -H "Authorization: Bearer $API_KEY"
```

保存客户端配置后，新建对话发送一句短文本，再到使用日志核对模型和令牌。测试生成会按实际调用计费。不要一开始打开批量翻译、自动任务或多个 Agent，以免无法区分哪条配置产生请求。

## 接入失败时

401 优先检查密钥与环境变量；404 检查是否拼成了重复的 `/v1/v1`；模型不可用时检查分组、模型限制和协议；普通对话成功但 Agent 失败时检查工具调用支持。仅有厂商内置登录、未开放自定义模型服务的客户端，不应套用本页配置；客户端会员权益也不会被本站令牌替代。

## 官方参考

- [OpenAI SDK 文档](https://developers.openai.com/api/docs/libraries)
- [Cline 的 OpenAI Compatible 配置](https://docs.cline.bot/provider-config/openai-compatible)
- [MCP 架构说明](https://modelcontextprotocol.io/docs/learn/architecture)
