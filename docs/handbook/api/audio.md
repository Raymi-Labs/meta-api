# 语音合成与识别

音频接口包括文字转语音、语音转文字和音频翻译。先在[模型广场](https://meta-api.vip/pricing)确认模型支持哪一种任务与协议，将 `MODEL_ID` 替换为实际 ID。音频输入模型、语音合成模型和实时对话模型并不通用，不能仅凭名称含有 audio 就互换使用。

## 文字转语音

`POST /v1/audio/speech` 接收 `model`、`input` 和 `voice`。示例的 `VOICE_ID` 也要替换为该模型支持的音色；这里的 MP3 格式仅适用于支持该输出格式的模型。`API_KEY` 由环境变量提供。

```bash
curl --fail-with-body -sS 'https://meta-api.vip/v1/audio/speech' \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "MODEL_ID",
    "input": "欢迎使用元衡 API。",
    "voice": "VOICE_ID",
    "response_format": "mp3"
  }' --output speech.mp3
```

语音结果通常是音频二进制，不能按 JSON 解析。保存文件前应检查 HTTP 状态和 Content-Type，避免把错误 JSON 当成音频播放。`speed`、`instructions`、流式格式与特定音色的支持由模型决定，不应同时套用不同服务商的参数。

## 阿里云实时语音

已配置的阿里渠道支持 Qwen-Audio-TTS 和 CosyVoice。普通合成使用 `POST /v1/audio/speech`；分段输入使用 `GET /v1/audio/speech/stream?model=MODEL_ID` 建立 WebSocket，接收二进制音频。该入口不使用 OpenAI Realtime 协议。

复刻音色创建成功后复用音色 ID；取消当前任务后可复用连接。配置、计费限制及完整示例见[阿里云 TTS 接入指南](https://github.com/Raymi-Labs/meta-api/blob/main/docs/channel/aliyun-tts.md)。实际可用性取决于部署版本和渠道配置。

## 语音转文字

准备本地 `sample.wav`，使用 multipart 上传。cURL 自动设置文件上传的 Content-Type 和 boundary。

```bash
curl --fail-with-body -sS 'https://meta-api.vip/v1/audio/transcriptions' \
  -H "Authorization: Bearer $API_KEY" \
  -F 'model=MODEL_ID' \
  -F 'file=@sample.wav'
```

兼容 JSON 响应的识别文字通常在 `text`。支持详细格式的模型还可能返回语言、时长、分段或时间戳；需先确认其 `response_format` 和时间戳参数。音频翻译使用 `/v1/audio/translations`，它的目标语言及支持范围取决于模型，不能假设可以指定任意语言对。

## 文件与错误排查

文件类型、体积、时长、采样率、输出速度及分段能力没有全站统一保证。先用清晰、较短的文件测试，再扩展到长音频；大量静音、噪声或多人重叠可能影响识别。400 常见于任务选错模型、音色不存在或格式不支持；413 检查上传大小。超时后先查使用日志，避免盲目重复提交。需要双向持续交互时另看 Realtime，普通 speech 的流式输出不等于完整实时会话。

关联：[MiMo 音频](/docs/mimo-audio)、[Realtime](/docs/reference/realtime)、[流式输出](/docs/reference/streaming)、[故障排查](/docs/troubleshooting)。
