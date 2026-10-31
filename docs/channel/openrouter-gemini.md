# OpenRouter Gemini：渠道 1638

本配置只统一渠道 1638 的 Gemini 模型名：对外 `gemini-…`，内部映射至 `google/gemini-…`。当前目录 39 个条目中，29 个具体型号纳入配置，10 个 batch 特殊入口按用户要求跳过。精确清单及原厂来源见 [审核记录](openrouter-gemini-audit-2026-11-01.json)。

## 当前阻碍

当前密钥对 4 个语音型号以及文本、向量、图像代表型号均返回 OpenRouter `403 provider Terms Of Service`。渠道保持暂停，并设置请求拒绝规则。**发布了适配代码不代表上游已经可用**；解除账号限制后仍须重新核价、验证实际 usage 和消费日志，再按审核结果逐个恢复，不能一键启用所有配置型号。

24 个型号有公开原厂价；Gemini 3.5 Flash 的 OpenRouter 音频输入高于当前原厂标准价，保持暂停；另 5 个旧 preview 型号仍需确认型号身份与当前原厂报价。所有未核实型号均不能调用。

## 语音协议和费用

- `/v1/audio/speech` 支持普通文本、voice、instructions 和原始 PCM；不支持 MP3、SSE 或非 1 倍语速。保留请求的数据隐私偏好。
- `/v1/audio/transcriptions` 接受 JSON base64 或单个上传文件，转为上游 JSON；支持 json/verbose_json，文件不超过 25 MiB。
- OpenRouter 原生语音接口不执行聊天的 `sort/only/max_price`。经用户确认，本网关在**每次调用前**查询该模型全部 endpoint。允许列表外的服务商、超价、非标准服务档位、新增非零费用或核价失败都会阻止发送。不会缓存“允许调用”的核价结果。
- 这是调用前核价，不是上游硬限价，不能消除报价与实际收费之间的调价窗口，也不能保证语音选择最低价服务商。
- TTS 根据 `X-Generation-Id` 查询原生输入/输出 Token，输出归为音频；STT 使用上游明确提供的 input/output tokens。缺少实际 usage 返回错误，不用音频文件大小或其他模型的时长系数估算。语音已经发出后不自动重试。
- TTS 为核实完整响应与 usage 会缓存音频（上限 64 MiB）；generation 查询有界等待 8 秒，HTTP 元数据禁止重定向。无法核实 usage 时上游可能已产生费用，但网关不会猜测向用户收费。

原厂价格来源：[Google API](https://ai.google.dev/gemini-api/docs/pricing)、[Google Cloud](https://cloud.google.com/model-garden/pricing)。协议来源：[OpenRouter TTS](https://openrouter.ai/docs/guides/overview/multimodal/tts)、[STT](https://openrouter.ai/docs/guides/overview/multimodal/stt)、[generation usage](https://openrouter.ai/docs/api/api-reference/generations/get-generation)。

## 约 2% 价差

原售价按固定 USD/CNY 6.8 换算；官方对比使用 2026-10-30 ECB 公开汇率 6.652882205513785。`6.8 / 6.652882205513785 - 1 = 2.211%`，差异来自汇率口径，不是 Token 多扣 2%。

价格计划复用同一公开汇率，能量售价 = 原厂美元价 × 6.652882205513785 ÷ 0.2，即 ×33.264411027568926。分别比较输入、输出和缓存，保留已有较低价格；已有按次收费不擅自改为按 Token。图像输出复用 `img_o`，不与文本重复计价。此处是公开汇率快照，不是每日自动调价服务；不修改充值价、全局分组倍率或官方对比显示。

`scripts/openrouter-retail-plan.py` 只生成无凭据计划；生产写入必须读取最新值、校验预期旧值、合并本次涉及键并保留备份。应用先经受保护更新入口升级，配置随后写入，渠道保持暂停。恢复调用须另有上游账号可用和真实计量证据。
