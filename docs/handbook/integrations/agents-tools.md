# Agent、工具调用与 Skills

Agent 在模型回复和工具执行之间循环：模型提出工具调用，客户端验证参数并执行工具，再把结果交回模型。本站转发模型协议，不会替客户端执行本地命令、访问文件或安装 Skill。只有所选模型及渠道支持工具调用时，下面的 Chat Completions 示例才适用；协议字段及其他格式见[工具调用](/docs/reference/tools)。

## 配置与职责

| 部分 | 谁负责 |
| --- | --- |
| 模型地址与密钥 | `https://meta-api.vip/v1` 与环境变量 `API_KEY` |
| 模型选择 | 将 `MODEL_ID` 替换为支持当前协议及工具调用的模型 |
| `tools` 定义 | 客户端声明名称、参数类型和用途 |
| 工具执行 | 应用校验名称、参数和权限后执行 |
| Skill / 提示词文件 | 由本地 Agent 加载，不是网关端点或权限授权 |

## 一个受限的工具循环

安装 `openai` 后运行此 Python 示例。演示只允许加法工具，不执行模型生成的代码；最多三轮，防止无限循环。每次模型请求都可能产生用量。

```python
import json
import math
import os
from openai import OpenAI

client = OpenAI(api_key=os.environ["API_KEY"],
                base_url="https://meta-api.vip/v1", timeout=60.0, max_retries=0)
messages = [{"role": "user", "content": "Use add to calculate 2 + 3."}]
tools = [{"type": "function", "function": {
    "name": "add", "description": "Add two finite numbers.",
    "parameters": {"type": "object", "properties": {
        "a": {"type": "number"}, "b": {"type": "number"}},
        "required": ["a", "b"], "additionalProperties": False}
}}]

for _ in range(3):
    result = client.chat.completions.create(
        model="MODEL_ID", messages=messages, tools=tools)
    message = result.choices[0].message
    if not message.tool_calls:
        print(message.content)
        break
    messages.append(message.model_dump(exclude_none=True))
    for call in message.tool_calls:
        if call.type != "function" or call.function.name != "add":
            raise ValueError("Unexpected tool")
        args = json.loads(call.function.arguments)
        if not isinstance(args, dict) or set(args) != {"a", "b"} or any(
            type(args[k]) not in (int, float)
            or abs(args[k]) > 1_000_000 or not math.isfinite(args[k])
            for k in ("a", "b")
        ):
            raise ValueError("Invalid arguments")
        messages.append({"role": "tool", "tool_call_id": call.id,
                         "content": json.dumps({"sum": args["a"] + args["b"]})})
else:
    raise RuntimeError("Tool round limit reached")
```

## 接入现成 Agent

Codex、Claude Code、Gemini CLI、OpenCode、OpenClaw 的步骤见[客户端总览](/docs/clients)。CodeBuddy、WorkBuddy、DeepSeek Harness、Crush、Gptme、Hermes Agent、Qwen Code、Kilo Code 等工具，应先核对其当前版本的自定义提供商和协议；只支持某厂商登录的模式不能直接换成本站 Key。为其他平台编写的专属插件，也需要明确支持自定义地址后才能使用。

Skill 是流程和说明的集合，安装前应检查来源、命令及其将读取的数据。需要图片生成时可查看[本站生图指南](/docs/image-generation)，不能据此推断任意 Skill 都受本站维护。涉及发送消息、修改数据或执行系统命令的工具，应由应用在执行前落实相应确认。

## 排查工具循环

普通对话成功但工具失败时，先检查工具能力、参数 JSON、调用 ID 和结果角色。不要丢弃模型返回的工具调用消息，也不要把一个工具结果配给另一调用。流式参数需完整拼接后才能解析执行；断流或缺失结束状态时不要执行残缺参数。工具多轮重复失败时停止并保存请求 ID，分别检查模型请求与工具本身。

## 官方参考

- [OpenAI Function Calling 流程](https://developers.openai.com/api/docs/guides/function-calling)
- [MCP 架构与客户端职责](https://modelcontextprotocol.io/docs/learn/architecture)
