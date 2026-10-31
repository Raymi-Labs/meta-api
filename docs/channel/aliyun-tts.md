# 阿里云实时语音合成接入

Meta API 的阿里通义千问渠道支持 Qwen-Audio-TTS、CosyVoice 的实时语音合成。客户端使用 Meta API 令牌，上游百炼 API Key 只保存在服务端渠道中。用户可以通过本站 API 上传录音、创建复刻音色，再传入保存的音色 ID 合成新文本，无需登录阿里云或重复上传录音。

此功能从 v1.5.0 起提供。代码支持不表示线上已部署或模型已开放。管理员部署包含此功能的版本、配置渠道及价格后，用户才能调用。

## 支持范围

| 模型 | 上游结算单位 |
| --- | --- |
| `qwen-audio-3.1-tts-flash` | 输入文本 Token、输出音频 Token |
| `qwen-audio-3.1-tts-plus`、`qwen-audio-3.0-tts-plus`、`qwen-audio-3.0-tts-flash` | 计费字符 |
| `cosyvoice-v3.5-plus`、`cosyvoice-v3.5-flash`、`cosyvoice-v3-plus`、`cosyvoice-v3-flash`、`cosyvoice-v2` | 计费字符 |

只支持上述明确型号，不推断日期后缀或其他型号的协议。`qwen3-tts-flash-realtime`、语音对话 Realtime、Sambert、AOQ 和 WebRTC 不在本次适配范围。TTS 只负责朗读，完整语音聊天仍需客户端或应用编排语音识别、大模型回答和音频播放。

## 管理员配置

1. 在百炼对应地域创建 API Key。Key、业务空间、模型及音色必须属于可互相访问的地域和业务空间。
2. 在 Meta API「渠道」新增类型 **阿里通义千问**，密钥填写百炼 API Key。
3. 代理地址填写业务空间域名，例如 `https://{WorkspaceId}.cn-beijing.maas.aliyuncs.com`。新加坡使用 `https://{WorkspaceId}.ap-southeast-1.maas.aliyuncs.com`。不要追加 `/compatible-mode/v1`。网关自动转换为 `wss://` 并使用 `/api-ws/v1/inference`；也接受完整的该推理路径。优先使用百炼控制台显示的专属域名。
4. 手动选择需要开放的模型，如 `qwen-audio-3.1-tts-flash`，配置可用分组和令牌模型权限。渠道模型映射可用，但一个实时连接只能使用 URL 查询参数指定的同一个公开模型。
5. 在模型价格中配置倍率计费。Flash 分别配置文本输入和音频输出价格；字符模型将上游 `characters` 作为文本输入计费单位，输出不计。显示为输入单位的数值是计费字符，不是分词器 Token。不要直接照抄不同币种、地域或模型的价格。

首版的这两个阿里 TTS 入口仅支持倍率计费；按次价格、按时长价格、阶梯计费表达式会明确报错。默认不为新模型写入商业价格，管理员须按自己的售价配置。音频输出费用复用本站现有「模型倍率 × 音频倍率 × 音频补全倍率」计算关系，避免把音频输出价格填成普通文本补全倍率。

渠道通用「测试」没有音色输入，因此会提示使用下面的语音示例，不会发送默认聊天请求或替你创建复刻音色。真实语音调用可能计费。

## 反向代理的 WebSocket 配置

公网反向代理须为 `/v1/audio/speech/stream` 保留 HTTP/1.1 Upgrade。若直连应用返回 101、公网返回 400，先检查代理是否清空了 Connection。为此路径创建精确 location，复用现有 API 的可信入站、鉴权、维护窗口、真实 IP 和上游设置，在该路径设置：

```nginx
proxy_http_version 1.1;
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection "upgrade";
proxy_buffering off;
proxy_read_timeout 3600;
proxy_send_timeout 3600;
proxy_next_upstream off;
```

这段仅展示升级所需指令，不是完整 server 配置；不要同时保留该路径的 `proxy_set_header Connection ""`。保存备份后执行 `nginx -t`，通过后热重载，再从公网实际发送任务验证。普通 HTTP speech 不需要协议升级。

## 普通客户端：HTTP speech

使用已有 `POST /v1/audio/speech`。完整文本一次提交，网关收到音频块就立即返回，不等待整个文件完成。

```bash
export META_API_KEY='你的 Meta API 令牌'
curl --fail-with-body --no-buffer 'https://meta-api.com/v1/audio/speech' \
  -H "Authorization: Bearer $META_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "qwen-audio-3.1-tts-flash",
    "input": "你好，很高兴和你聊天。",
    "voice": "longanhuan_v3.1",
    "response_format": "mp3",
    "speed": 1.0,
    "metadata": {"sample_rate": 24000}
  }' --output speech.mp3
```

`voice` 可以换成已创建成功且与目标模型一致的复刻音色 ID。`cosyvoice-v3.5-*` 需要使用其支持的自定义音色，不能套用示例中的系统音色。

支持 `response_format=mp3/pcm/wav/opus`，默认 MP3；最终可用范围以模型为准。`instructions` 映射为百炼 `instruction`，`speed` 映射为 `rate`（0.5–2，省略使用上游默认）。`metadata` 可传 `sample_rate`、`volume`、`pitch`、`bit_rate`、`language_hints`、`hot_fix`、`seed`、`enable_ssml`、`word_timestamp_enabled`，它只补充合成参数，不能覆盖模型、任务或文本。

默认或 `stream_format=audio` 返回音频二进制。设为 `stream_format=sse` 时返回：

```text
data: {"type":"speech.audio.delta","audio":"BASE64_AUDIO_CHUNK"}

data: {"type":"speech.audio.done","usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}

```

SSE 的 `audio` 是该次音频块的 Base64。按块解码后交给播放器；只有收到 `speech.audio.done` 才算完整结束。错误使用 SSE 错误事件。二进制响应开始后无法改写 HTTP 状态，异常会截断音频并写入失败日志，不会把错误 JSON 拼到音频末尾；HTTP 200 本身不证明完整成功。

## 用户音色：上传、查询与删除

此组接口从 **v1.10.0** 起提供，使用 **Meta API 令牌**，不是百炼 Key。管理员须先部署包含音色管理的版本；现有 TTS 版本并不自动拥有这些接口。复用令牌的模型权限、分组、渠道白名单和限流。首版只接入 API，没有控制台上传页面。

| 操作 | 接口 | 返回 |
| --- | --- | --- |
| 上传并创建 | `POST /v1/audio/voices` | 音色对象，首次创建 HTTP 201，幂等重放 HTTP 200 |
| 列出我的音色 | `GET /v1/audio/voices?limit=20&offset=0` | `data`、`has_more`、`limit`、`offset` |
| 刷新状态 | `GET /v1/audio/voices/{id}` | 音色对象及最新 `status` |
| 删除 | `DELETE /v1/audio/voices/{id}` | `{"id":"meta-voice-…","deleted":true}` |

创建使用 multipart 字段：`model` 为之后合成时使用的公开模型名；`name` 为1–64字符的音色名称；`file` 为本地录音。支持 WAV、MP3、M4A，最大10 MiB，时长5–60秒，推荐10–20秒。推荐24 kHz及以上清晰人声；采样率等供应商音质要求仍由阿里校验；仅改文件扩展名不能转换音频。本站不接收录音 URL，不长期保存录音，文件会编码后发送到管理员配置的阿里渠道。

```bash
export META_API_KEY='本站令牌'
# 每次有意创建新音色使用新键；网络错误后的重试必须保留原键。
export VOICE_REQUEST_KEY='myvoice-20261031-001'
curl --fail-with-body -sS 'https://meta-api.com/v1/audio/voices' \
  -H "Authorization: Bearer $META_API_KEY" \
  -H "Idempotency-Key: $VOICE_REQUEST_KEY" \
  -F 'model=qwen-audio-3.1-tts-flash' \
  -F 'name=我的声音' \
  -F 'file=@sample.wav'
```

`Idempotency-Key` 必填，为8–128位字母、数字、点、下划线、冒号或连字符。同一归属范围下，相同键和相同内容返回原记录，不再次创建；相同键配不同录音、模型或名称返回409。删除后旧键仍返回已删除记录，重新创建需换新键。**不要在超时后换键重试**。

返回对象示例（ID仅为占位符）：

```json
{"id":"meta-voice-0123456789abcdef0123456789abcdef","name":"我的声音","model":"qwen-audio-3.1-tts-flash","status":"processing","created_at":1793404800,"updated_at":1793404800}
```

保存 `id`，查询到 `ready` 后才可合成：

```bash
export VOICE_ID='替换为返回的meta-voice-ID'
curl --fail-with-body -sS "https://meta-api.com/v1/audio/voices/$VOICE_ID" \
  -H "Authorization: Bearer $META_API_KEY"

# model 必须与创建时相同；响应为音频，先检查命令退出状态再播放。
curl --fail --no-buffer -sS 'https://meta-api.com/v1/audio/speech' \
  -H "Authorization: Bearer $META_API_KEY" \
  -H 'Content-Type: application/json' \
  -d "{\"model\":\"qwen-audio-3.1-tts-flash\",\"input\":\"你好，这是我的复刻声音。\",\"voice\":\"$VOICE_ID\",\"response_format\":\"mp3\"}" \
  --output speech.mp3

curl --fail-with-body -sS -X DELETE "https://meta-api.com/v1/audio/voices/$VOICE_ID" \
  -H "Authorization: Bearer $META_API_KEY"
```

| status | 含义与操作 |
| --- | --- |
| `creating` | 创建中，保留 ID，稍后查询 |
| `processing` | 上游已接受，等待处理/审核；稍后查询 |
| `ready` | 可以合成 |
| `rejected` | 上游审核未通过，可删除后更换样本 |
| `failed` | 上游明确拒绝创建；可删除记录 |
| `unknown` | 上游结果未确认；查询会按该请求的唯一前缀寻找已创建音色，不自动重复创建 |
| `deleting` / `delete_failed` | 删除中或删除未确认，不能再合成；稍后查询或重试 DELETE |
| `deleted` | 已删除，不能再合成 |

创建失败响应可能含 `voice` 对象，务必保留其中的 ID；如果连接中断未收到 ID，用同一个幂等键重试或从列表找回。长时间 `unknown` 且无法找回时联系管理员核对上游，不能把查询暂时为空当作创建未发生。删除失败不释放配额，重试删除不会重新创建。阿里明确返回音色不存在时视为已删除，其他错误不能当作删除成功。

普通令牌按所属用户共享音色；可售令牌按令牌独立隔离，不能读取发行者或其他买家的音色。每个归属范围最多20条未删除记录（包括失败/未知记录），删除后释放名额；阿里账号自身的共享配额仍适用。列表不包含删除记录，`limit` 为1–100，`offset` 为0–100000，`has_more` 指示是否还有下一页。

音色创建和管理不另扣本站额度；合成按当前模型及实际路由分组计费。音色绑定创建时的公开模型、映射后模型、渠道及凭据摘要，后续调用仍须有该模型与渠道的访问权限。首版要求单 Key 阿里渠道，不支持多 Key 轮询池。禁用渠道、撤销分组权限、修改模型映射、变更渠道 Key 或业务空间会使合成失败；不会自动跨账号转发。凭据轮换需管理员核实音色归属，不能直接替换成另一个账号继续使用。

本站 ID 以 `meta-voice-` 开头；上游创建使用保留的 `mv` 加8位十六进制前缀，禁止下游直接使用这类上游音色 ID。原有系统音色与其他外部预创建音色的用法保持不变。

供应商依据：[声音复刻 HTTP API](https://help.aliyun.com/zh/model-studio/voice-clone-design-http-api)、[录音要求和绑定规则](https://help.aliyun.com/zh/model-studio/voice-cloning-user-guide)。

## 实时聊天：双向 WebSocket

连接：

```text
wss://meta-api.com/v1/audio/speech/stream?model=qwen-audio-3.1-tts-flash
Authorization: Bearer <META_API_KEY>
```

该入口使用百炼任务事件和二进制帧，不是 OpenAI `/v1/realtime` 协议，也不能只替换官方 SDK 的 Base URL（本站还要求 `model` 查询参数）。服务端应用可设置 Authorization；浏览器可沿用本站 `realtime` 与 `openai-insecure-api-key.<META_API_KEY>` 子协议鉴权，使用受限的本站令牌，不能把百炼 Key 发给浏览器。

使用本站创建的 `meta-voice-…` 音色时，连接URL还须添加 `&voice=音色ID`，用于在握手前固定到创建音色的渠道。每轮 `run-task.parameters.voice` 必须与查询参数一致；想换另一个音色需建立新连接。删除音色后，原连接的新任务也会被拒绝。下面代码自动为本站音色添加此参数。

依次发送 `run-task`，等待 `task-started`，再把大模型生成的文本分段送入 `continue-task`。文本结束时发送 `finish-task`。同时持续接收消息：文本帧是任务状态/用量，二进制帧是音频。

下面的 Python 示例只使用本站令牌。安装 `websocket-client` 后运行。接收线程与发送线程同时工作；写文件处可以替换为实时播放器。

```python
import json
import os
import queue
import threading
import uuid
from urllib.parse import urlencode

import websocket

model = "qwen-audio-3.1-tts-flash"
voice = os.environ.get("TTS_VOICE", "longanhuan_v3.1")
query = {"model": model}
if voice.startswith("meta-voice-"):
    query["voice"] = voice
url = "wss://meta-api.com/v1/audio/speech/stream?" + urlencode(query)
ws = websocket.create_connection(
    url, header=["Authorization: Bearer " + os.environ["META_API_KEY"]], timeout=30
)
events = queue.Queue()

def receive():
    try:
        with open("speech.mp3", "wb") as output:
            while True:
                message = ws.recv()
                if isinstance(message, bytes):
                    output.write(message)  # 可在这里逐块播放
                elif message:
                    events.put(json.loads(message))
                else:
                    raise RuntimeError("connection closed")
    except Exception as error:
        events.put(error)

def wait_event(expected):
    while True:
        event = events.get(timeout=35)
        if isinstance(event, Exception):
            raise event
        if "error" in event or event.get("header", {}).get("event") == "task-failed":
            raise RuntimeError(event)
        if event.get("header", {}).get("event") == expected:
            return event

def send(action, task_id, payload):
    ws.send(json.dumps({
        "header": {"action": action, "task_id": task_id, "streaming": "duplex"},
        "payload": payload,
    }, ensure_ascii=False))

reader = threading.Thread(target=receive, daemon=True)
reader.start()
try:
    task_id = str(uuid.uuid4())
    send("run-task", task_id, {
        "task_group": "audio", "task": "tts", "function": "SpeechSynthesizer",
        "model": model,
        "parameters": {"text_type": "PlainText", "voice": voice,
                       "format": "mp3", "sample_rate": 24000},
        "input": {},
    })
    wait_event("task-started")
    for text in ["你好。", "我们可以一边生成回答，一边播放语音。"]:
        send("continue-task", task_id, {"input": {"text": text}})
    send("finish-task", task_id, {"input": {}})
    wait_event("task-finished")
finally:
    ws.close()
    reader.join(timeout=3)
```

需要打断当前回答时，立即停止/清空客户端播放队列，并发送：

```json
{"header":{"action":"finish-task","task_id":"当前任务ID","streaming":"duplex"},"payload":{"input":{"directive":"cancel"}}}
```

等到 `task-finished` 后，可以在原连接上用全新的 `task_id` 开始下一轮。不能并行启动两个任务，不能改变本连接的公开模型，不能重用旧 task ID。跨轮复用的是连接与音色 ID，合成每段新文本仍会产生用量。

## 延迟、限制与计费

- 阿里服务会缓冲不完整语句，优先按短句/标点提交，结束时必须发送 `finish-task`。不要等大模型完整回答后才送入 TTS，也不要把文件下载完成当作流式播放。
- 单条客户端消息最多 128 KiB；文本每段最多 20,000 字符、每任务最多 200,000 字符。一个连接最多 100 个顺序任务、最长 10 分钟；网关上游空闲超时 60 秒。百炼自身可能更早超时，例如文本发送间隔要求小于 23 秒。
- 收到音频即转发；每次写入最多等待 10 秒，慢客户端会断开。客户端断开或额度不足时尝试取消，最多额外等 2 秒收取上游终态和用量。
- 按连接记录一次使用日志。各任务累计用量去重后求和，`sentence-end` 和 `task-finished` 不能重复相加。取消或失败时保留已确认的消耗；没有报告的上游用量不猜测，日志会说明未完整结束。上游在断线后仍可能产生本站来不及确认的费用，应通过百炼账单核对。
- 实时与阿里 HTTP speech 禁用信任预扣旁路。按已提交文本保守预留额度（每字符预留两个输入单位，Flash 再预留 32 个音频输出 Token），叠加正常请求窗口；**预留不是最终收费**，结束后按上游实际用量结算并返还差额。余额很低或长文本可能提前被拒绝。
- 本地协议测试覆盖提前播放、取消复用、用量去重和失败扣费，但不代表真实百炼首包延迟。应在实际部署地区分别测直连与经网关的首段音频时间、连续播放和打断停声时间。

## 排查

握手 401/403：核对渠道百炼 Key、地域和业务空间权限。模型价格错误：配置倍率及对应音频输出价格。音色不存在或无声音：核对 `voice` 的状态及其 `target_model`。调用 `/v1/realtime` 报错：改用本文独立入口和任务协议。缺少 `task-started`、`task-finished` 或用量：检查使用日志及上游错误，已提交文本不自动重试，避免重复发声和计费。

协议依据：[实时语音合成](https://docs.bailian.console.aliyun.com/zh/model-studio/realtime-tts-user-guide)、[Qwen-Audio-TTS WebSocket](https://help.aliyun.com/zh/model-studio/qwen-audio-tts-websocket-api)、[客户端事件](https://help.aliyun.com/zh/model-studio/qwen-audio-tts-client-events)、[服务端用量](https://help.aliyun.com/zh/model-studio/qwen-audio-tts-server-events)、[CosyVoice WebSocket](https://help.aliyun.com/zh/model-studio/cosyvoice-websocket-api)。
