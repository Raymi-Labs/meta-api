# SDK、HTTP 与应用框架

使用支持 OpenAI Chat Completions 的模型时，可以保留现有 SDK，把服务地址改为 `https://meta-api.vip/v1`，并使用本站令牌。本页选择最小对话请求，便于确认网络、鉴权和模型是否匹配；需要 [Responses](/docs/reference/responses)、[Claude Messages](/docs/reference/messages) 或 [Gemini 原生 API](/docs/reference/gemini) 时，请按相应文档选择端点，不能只换一个模型名称。

## 配置与安装

| 配置 | 含义 |
| --- | --- |
| `API_KEY` | 在运行环境中设置的本站 API Key，不提交到代码仓库 |
| `base_url` / `baseURL` | Python / JavaScript SDK 的服务地址字段 |
| `MODEL_ID` | 替换为令牌可用的 Chat 模型 ID |
| `timeout` / `maxRetries` | 客户端等待和重试策略，不代表服务端撤销请求 |

```bash
python3 -m pip install openai
# JavaScript 项目使用自己的包管理器安装：
npm install openai
```

## Python 示例

保存为 `example.py`，确认启动它的终端已设置 `API_KEY` 后运行 `python3 example.py`。

```python
import os
from openai import OpenAI

client = OpenAI(
    api_key=os.environ["API_KEY"],
    base_url="https://meta-api.vip/v1",
    timeout=60.0,
    max_retries=0,
)
result = client.chat.completions.create(
    model="MODEL_ID",
    messages=[{"role": "user", "content": "Hello"}],
)
print(result.choices[0].message.content)
```

## 服务端 JavaScript 示例

保存为 `example.mjs`，使用 Node.js 运行。不要把这段含密钥的调用直接放进浏览器页面。

```javascript
import OpenAI from 'openai';

const client = new OpenAI({
  apiKey: process.env.API_KEY,
  baseURL: 'https://meta-api.vip/v1',
  timeout: 60_000,
  maxRetries: 0,
});
const result = await client.chat.completions.create({
  model: 'MODEL_ID',
  messages: [{ role: 'user', content: 'Hello' }],
});
console.log(result.choices[0].message.content);
```

## 不依赖 SDK 的 HTTP 请求

下面请求可用于区分“平台调用失败”和“框架转换有误”。成功结果的正文通常位于 `choices[0].message.content`；其他字段及结束原因见 [Chat Completions](/docs/reference/chat-completions)。

```bash
curl --fail-with-body 'https://meta-api.vip/v1/chat/completions' \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"model":"MODEL_ID","messages":[{"role":"user","content":"Hello"}]}'
```

## 接入框架

AI SDK、LangChain 或 LiteLLM 会在业务代码和模型接口之间增加一层适配。先用上述 HTTP 请求验证，再在框架的 OpenAI 兼容提供方中设置同一地址、密钥和模型。不同版本可能默认使用 Responses、Chat 或旧 Completions，必须核对实际请求路径。嵌入、重排、图像和音频分别使用对应组件，不把聊天模型自动用于所有任务。

首次排查关闭自动重试；修复后再按错误类型设置有限次数和退避。超时不证明服务端未处理，重新提交可能产生另一条请求。记录时间、HTTP 状态和请求 ID，不记录完整密钥。收到流式响应时应持续消费事件并检查结束状态，不能只凭 HTTP 200 判断整个调用成功。

## 官方参考

- [OpenAI SDK 安装与环境变量](https://developers.openai.com/api/docs/libraries)
- [OpenAI 文本生成接口说明](https://developers.openai.com/api/docs/guides/text)
- [Continue 的 OpenAI 兼容地址配置示例](https://docs.continue.dev/customize/model-providers/top-level/openai)
