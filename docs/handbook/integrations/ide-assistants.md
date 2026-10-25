# IDE 与编程助手接入

编辑器中的聊天、自动补全、代码修改和 Agent 可能使用不同模型配置。先让普通聊天工作，再开启读取文件、执行命令或修改代码。Cursor、Cline、Continue、Roo Code、Kilo Code、Avante.nvim 等工具的界面会随版本变化；本页统一说明连接字段，具体功能仍由客户端和所选模型共同决定。

## 配置字段

| 项目 | 填写与核对方式 |
| --- | --- |
| API Provider | 选择 OpenAI Compatible 或当前版本的自定义提供商 |
| Base URL / `apiBase` | `https://meta-api.vip/v1` |
| API Key | 独立开发令牌；脚本读取 `API_KEY` |
| Model ID | `MODEL_ID`，必须支持客户端所用协议 |
| 上下文长度 | 按模型广场填写，不因客户端允许输入大数而扩大模型限制 |
| 工具调用 / 图片 | 仅在模型与渠道支持时启用 |

## Cline、Continue 与其他编辑器

在 Cline 设置中选择 OpenAI Compatible，依次填写 Base URL、API Key 和 Model ID，保存后从新任务开始。Continue 的 OpenAI 兼容提供方使用 `apiBase` 指定服务地址；配置中区分聊天、编辑和补全的模型角色。Roo Code、Kilo Code 与 Avante.nvim 请先查看当前版本支持的自定义提供方，不能直接粘贴另一客户端的配置文件。

Cursor 的自带 Key 使用范围和功能限制以官方当前说明为准。即使连接可用，客户端专属模型、自动补全或 Agent 功能也可能需要独立权限。本站令牌不用于绕过客户端订阅、登录或访问限制。

## 把客户端问题与接口问题分开

先在终端发出同协议的最小请求，将 `MODEL_ID` 替换为真实模型：

```bash
curl --fail-with-body 'https://meta-api.vip/v1/chat/completions' \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"model":"MODEL_ID","messages":[{"role":"user","content":"Return OK"}]}'
```

若命令成功而编辑器失败，检查编辑器是否仍使用旧连接、是否自动改用了 Responses、是否额外发送工具或推理参数。不要把不支持的参数批量改成空字符串；先恢复最小配置，再按模型文档逐项添加。终端修改环境变量后，应重新打开编辑器或其终端进程。

## Agent 的验收范围

在临时测试目录中要求助手只读取一个无敏感数据的文件并解释内容。确认普通响应、工具请求、工具结果回传都正常后，再授权写文件或执行命令。工具调用失败不等于 API Key 失效；检查模型工具能力和客户端工具格式。连续自动重试时停止任务并查使用日志，避免同一失败循环继续消耗额度。终端工具的独立教程见[客户端接入](/docs/clients)。

## 官方参考

- [Cline OpenAI Compatible 配置](https://docs.cline.bot/provider-config/openai-compatible)
- [Continue OpenAI 与自定义 apiBase](https://docs.continue.dev/customize/model-providers/top-level/openai)
- [Cursor 官方文档](https://cursor.com/docs)
