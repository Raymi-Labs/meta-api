# 阿里云实时语音合成接入

Meta API 的阿里通义千问渠道支持 Qwen-Audio-TTS、CosyVoice 的实时语音合成。客户端使用 Meta API 令牌，上游百炼 API Key 只保存在服务端渠道中。支持预先创建的复刻音色；每次合成传入保存的音色 ID，无需重新上传录音。

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

## 实时聊天：双向 WebSocket

连接：

```text
wss://meta-api.com/v1/audio/speech/stream?model=qwen-audio-3.1-tts-flash
Authorization: Bearer <META_API_KEY>
```

该入口使用百炼任务事件和二进制帧，不是 OpenAI `/v1/realtime` 协议，也不能只替换官方 SDK 的 Base URL（本站还要求 `model` 查询参数）。服务端应用可设置 Authorization；浏览器可沿用本站 `realtime` 与 `openai-insecure-api-key.<META_API_KEY>` 子协议鉴权，使用受限的本站令牌，不能把百炼 Key 发给浏览器。

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
url = "wss://meta-api.com/v1/audio/speech/stream?" + urlencode({"model": model})
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
