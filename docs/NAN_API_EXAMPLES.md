# NAN 各厂商模型调用请求样例

> 通过本站统一调用模型。在线文档中的地址跟随后台服务器地址；模型、分组和样例跟随当前模型广场，进入页面、返回标签页及页面可见时每分钟自动刷新，也可点击刷新。

## 1. 客户端地址怎么填

| 客户端协议              | Base URL                  | 请求路径                                 |
| ----------------------- | ------------------------- | ---------------------------------------- |
| OpenAI Chat / Responses | `https://meta-api.vip/v1` | `/chat/completions` 或 `/responses`      |
| Claude / Anthropic      | `https://meta-api.vip`    | `/v1/messages`                           |
| Google Gemini           | `https://meta-api.vip`    | `/v1beta/models/{model}:generateContent` |
| OpenAI Images           | `https://meta-api.vip/v1` | `/images/generations`                    |

Base URL 是否带 `/v1` 取决于客户端，避免重复拼接。API Key 在 `控制台 -> 令牌管理` 创建。模型和协议必须匹配，不能仅凭厂商名称推断兼容性。

## 2. 设置密钥并查询令牌模型

```bash
export NAN_BASE_URL="https://meta-api.vip"
export NAN_API_KEY="sk-替换为你的API密钥"

curl -sS "$NAN_BASE_URL/v1/models" \
  -H "Authorization: Bearer $NAN_API_KEY"
```

模型广场展示当前身份可见的模型，令牌还可能有独立的分组和模型限制。正式调用前，用上面的请求核对该令牌返回的模型名。不要把真实 API Key 写入仓库、截图或工单。

## 3. 当前分组

<!-- docs:groups -->

请打开[在线文档](https://meta-api.vip/docs/api-examples)或[模型广场](https://meta-api.vip/pricing)查看当前分组。

<!-- /docs:groups -->

## 4. 当前模型与协议

<!-- docs:catalog -->

在线文档自动展示当前模型、账号可选分组与支持协议。源码不保存易过期的模型快照。

<!-- /docs:catalog -->

## 5. 按当前模型生成的请求样例

以下样例按当前目录中声明的协议生成，使用最少的通用参数。选择顺序不代表质量、价格或稳定性推荐，也不证明当前令牌已具备调用权限。可按上表替换成支持相同协议的模型，并选择该模型对应的分组。

<!-- docs:examples -->

请打开[在线文档](https://meta-api.vip/docs/api-examples)查看按现网配置生成的 Chat、Responses、Claude、Gemini 和图片请求。

<!-- /docs:examples -->

## 6. 流式调用

OpenAI Chat、Responses 和 Anthropic Messages 可在请求体中加入 `"stream": true`。使用 `curl -N` 关闭输出缓冲：

<!-- docs:stream -->

在线文档会选择当前支持 Chat Completions 的文本模型生成流式样例。

<!-- /docs:stream -->

HTTP 200 不代表整个流式请求成功，请继续检查流中的错误和结束事件。模型可能先输出思考内容；正文为空或截断时，核对生成预算和使用日志。

## 7. Python OpenAI SDK

```bash
python3 -m pip install openai
```

模型同样由在线文档从支持 Chat Completions 的 OpenAI 模型中选取。若仍显示 MODEL\_\*，先确认模型目录可用。

```python
import os
from openai import OpenAI

client = OpenAI(
    api_key=os.environ["NAN_API_KEY"],
    base_url="https://meta-api.vip/v1",
)
response = client.chat.completions.create(
    model="MODEL_OPENAI",
    messages=[{"role": "user", "content": "你好，请简短介绍自己。"}],
)
print(response.choices[0].message.content)
```

## 8. 生图、语音与排查

- [AI 生图](./NAN_IMAGE_GENERATION.md)：平台工作台、专用技能包、图片编辑及 Responses 生图工具。
- [MiMo 语音](./NAN_MIMO_AUDIO.md)：TTS、音色设计、音色克隆和 ASR 的专用请求体。
- [常见报错](./NAN_TROUBLESHOOTING.md)：认证、分组、模型、网络和扣费排查。

语音、图像工具和第三方客户端的专用参数不能只换模型名套用；这些说明仍需随协议变化维护。
