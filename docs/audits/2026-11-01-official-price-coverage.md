# 官方参考价格覆盖审计（2026-11-01）

## 范围与结果

只读审计 `https://meta-api.com/api/pricing` 的 224 个公开模型。快照中已有厂家参考价 103 个，缺失 121 个；修复后对缺失项逐一抓取厂家资料，89 个取得完整可解析价格，32 个仍缺少可核实的报价或精确型号映射。

89 是多轮有界抓取中成功结果的并集，不代表单轮请求全部成功，也不代表生产数据库已写入。全目录重抓时部分旧文档返回 404、模型身份不匹配或超时；同步失败保留已有有效价格。已有 103 加新增可取得的 89 合计为 192 个潜在可覆盖条目，须部署后按同步开关、人工字段所有权及抓取结果实际落库。本轮没有部署生产或修改生产模型数据。

## 根因与修复

- Haiku 5.5 价格包含 ≤100K / >100K 输入及 5m / 1h 缓存，旧解析只接受纯单价。本次保留完整四档，不压平条件。
- HTML 单元格已解码后再次按 HTML 解析，会丢失 `0<Token≤1M` 边界；本次仅做文本规范化。
- 增补已有同步入口缺少的厂家格式，官方域名、精确 ID、币种、单位和适用条件共同校验。
- AWS 页面占位符仅连接固定官方价格 JSON；OpenAI 图片表只解码数据，不执行网页代码。
- 文本、图片、音频、视频 Token 及字符价格使用不同单位，不能互算折扣。
- 复用逐字段所有权：人工有效值（含零价）保持，失败不清空已有价；自动同步不改变本站实际结算价格。

## 仍不能填入数值的 32 个模型

这些是证据缺口，不是已修复并有价的条目。取得精确 API 名称或厂家重新发布报价后再接入；不使用代理报价、相近版本或搜索摘要冒充官方价。

| 模型 | 原因 | 已核对入口 |
| --- | --- | --- |
| `mimo-v2.5-tts`、`mimo-v2.5-tts-voiceclone`、`mimo-v2.5-tts-voicedesign` | 厂家当前标注限时免费，未提供可用于该参考价结构的常规数值单价和计量单位；不能猜成按 Token 零价。 | [MiMo](https://mimo.mi.com/static/docs/price/pay-as-you-go.md) |
| `kimi-k2.5` | 当前厂家模型表标记已停用；未取得该历史版本当前有效单价。 | [Kimi 模型](https://platform.kimi.ai/docs/models.md) |
| `kimi-k2.8`、`kimi-k2.8-code` | 当前厂家价格表没有这些精确 API ID，不用 K3 价格替代。 | [Kimi 定价](https://platform.kimi.ai/docs/pricing/chat) |
| `MiniMax-M2.5` | 当前定价页未列该版本；旧发布文章不能证明当前服务价格。 | [MiniMax](https://platform.minimax.io/docs/pricing/overview.md) |
| `HappyOyster-1.0-Adventure`、`qwen-image-max`、`qwen-plus-2025-09-11`、`qwen-turbo`、`qwen-vl-max`、`qwen3-235b-a22b`、`qwen3-coder-plus`、`qwen3-max`、`qwen3-max-preview` | 当前下载的厂家定价正文没有对应精确 ID 的可解析报价；搜索索引的旧内容不当作当前报价。 | [百炼](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `gemini-3.1-flash-image-preview` | 型号文档返回 404，当前价格表未找到对应完整条件报价。 | [Gemini](https://ai.google.dev/gemini-api/docs/pricing) |
| `grok-imagine-image-edit`、`grok-imagine-image-lite` | 型号地址返回内容没有对应模型身份，不能套用 image 或 image-2.0。 | [xAI](https://docs.x.ai/developers/models) |
| `deepseek-v4.1-flash`、`deepseek-v4.1-flash-1017` | 厂家当前 API ID 是 deepseek-flash；仅明确接受的两个旧别名映射已实现，这两个版本名没有同等证据。 | [DeepSeek](https://api-docs.deepseek.com/quick_start/pricing/) |
| `meta/muse-wonder` | 本地无版本后缀，当前厂家报价为 muse-wonder-1.0，尚无明确 API 别名证明。 | [Meta](https://dev.meta.ai/docs/pricing-rate-limits) |
| `rekaai/reka-edge` | 本地无版本后缀，当前厂家价格表使用 reka-edge-2603，尚无明确 API 别名证明。 | [Reka](https://developer.reka.ai/pricing) |
| `unbiased/pareto-26.9` | 厂家当前更新记录只证实 26.10 的价格，不能回填给 26.9。 | [Unbiased](https://unbiased.ai/changelog/) |
| `Tripo/Tripo-H3.1`、`codex-auto-review`、`decision-model-preview`、`flow-video-gemini-omni-flash-1.1`、`flow-video-gemini-omni-flash-1.1-1080p`、`k3`、`muse-video`、`snow-grok-imagine-video` | 平台自定义名、组合服务或缺少已核实的厂家 API 映射；需模型维护者提供精确厂家型号及官方定价入口。 | 待核实映射 |

## 复查方法

正常单元测试离线运行。重新审计时先下载公开目录，再运行显式选择的只读网络测试：

```sh
TEST_OFFICIAL_PRICING_CATALOG=/path/to/public-pricing.json \
TEST_OFFICIAL_PRICING_REPORT=/path/to/report.json \
go test ./controller -run '^TestOfficialPricingCatalogCoverage$' -count=1 -v
```

默认只检查目录中缺价项；额外设置 `TEST_OFFICIAL_PRICING_ALL=1` 核查全目录。此入口不执行 `ApplyModelCapabilityMetadata`，不访问或写入生产数据库。结果随厂家页面、区域和时间变化，不能作为冻结价格种子。

## 取得厂家价格的 89 个原缺失模型

下表为真实下载资料经本次解析器校验的结果；只记录覆盖信息，不把审计快照当作运行时价格。

| 模型 | 币种 | 档位数 | 单位 | 来源 |
| --- | --- | --- | --- | --- |
| `MiniMax-H3` | USD | 3 | image, second | [source](https://platform.minimax.io/docs/pricing/overview.md) |
| `MiniMax-M3` | USD | 4 | million_tokens | [source](https://platform.minimax.io/docs/pricing/overview.md) |
| `MiniMax-M3.1-Flash` | USD | 1 | million_tokens | [source](https://platform.minimax.io/docs/pricing/overview.md) |
| `aion-labs/aion-2.0` | USD | 1 | million_tokens | [source](https://api.aionlabs.ai/v1/models) |
| `aion-labs/aion-3.0` | USD | 1 | million_tokens | [source](https://api.aionlabs.ai/v1/models) |
| `aion-labs/aion-3.0-mini` | USD | 1 | million_tokens | [source](https://api.aionlabs.ai/v1/models) |
| `aion-labs/aion-3.5` | USD | 1 | million_tokens | [source](https://api.aionlabs.ai/v1/models) |
| `aion-labs/aion-3.5-mini` | USD | 1 | million_tokens | [source](https://api.aionlabs.ai/v1/models) |
| `aion-labs/aion-rp-llama-3.1-8b` | USD | 1 | million_tokens | [source](https://api.aionlabs.ai/v1/models) |
| `amazon/nova-2-lite-v1` | USD | 1 | million_tokens | [source](https://aws.amazon.com/nova/pricing/) |
| `amazon/nova-lite-v1` | USD | 1 | million_tokens | [source](https://aws.amazon.com/nova/pricing/) |
| `amazon/nova-micro-v1` | USD | 1 | million_tokens | [source](https://aws.amazon.com/nova/pricing/) |
| `amazon/nova-pro-v1` | USD | 1 | million_tokens | [source](https://aws.amazon.com/nova/pricing/) |
| `claude-haiku-5-5` | USD | 4 | million_tokens | [source](https://platform.claude.com/docs/en/models/haiku-5-5/overview) |
| `cohere/command-a` | USD | 1 | million_tokens | [source](https://docs.cohere.com/v2/docs/command-a) |
| `cohere/command-r-08-2024` | USD | 1 | million_tokens | [source](https://docs.cohere.com/v2/docs/command-r) |
| `cohere/command-r-plus-08-2024` | USD | 1 | million_tokens | [source](https://docs.cohere.com/v2/docs/command-r-plus) |
| `cohere/command-r7b-12-2024` | USD | 1 | million_tokens | [source](https://docs.cohere.com/v2/docs/command-r7b) |
| `deepseek-v4-flash` | USD | 2 | million_tokens | [source](https://api-docs.deepseek.com/quick_start/pricing/) |
| `deepseek-v4-flash-vision-exp` | USD | 2 | million_tokens | [source](https://api-docs.deepseek.com/quick_start/pricing/) |
| `fireworks/ember-1` | USD | 1 | million_tokens | [source](https://fireworks.ai/models/fireworks/ember-1) |
| `glm-5` | USD | 1 | million_tokens | [source](https://docs.z.ai/guides/overview/pricing.md) |
| `glm-5.1` | USD | 1 | million_tokens | [source](https://docs.z.ai/guides/overview/pricing.md) |
| `gpt-image-2` | USD | 2 | million_image_tokens, million_tokens | [source](https://developers.openai.com/api/docs/pricing) |
| `gpt-image-2.5-flare` | USD | 2 | million_image_tokens, million_tokens | [source](https://developers.openai.com/api/docs/pricing) |
| `gpt-image-2.5-sunburst` | USD | 2 | million_image_tokens, million_tokens | [source](https://developers.openai.com/api/docs/pricing) |
| `happyhorse-1.1-i2v` | CNY | 3 | second | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `happyhorse-1.1-r2v` | CNY | 3 | second | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `happyhorse-1.1-t2v` | CNY | 3 | second | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `inception/mercury-2` | USD | 1 | million_tokens | [source](https://docs.inceptionlabs.ai/get-started/models.md) |
| `inception/mercury-2.5` | USD | 1 | million_tokens | [source](https://docs.inceptionlabs.ai/get-started/models.md) |
| `inception/mercury-3` | USD | 1 | million_tokens | [source](https://docs.inceptionlabs.ai/get-started/models.md) |
| `inference-net/schematron-v2-small` | USD | 1 | million_tokens | [source](https://inference.net/blog/schematron-v2/) |
| `inference-net/schematron-v2-turbo` | USD | 1 | million_tokens | [source](https://inference.net/blog/schematron-v2/) |
| `liquid/lfm-2.5-24b-a2b` | USD | 1 | million_tokens | [source](https://www.liquid.ai/pricing) |
| `meituan/longcat-2.0` | USD | 1 | million_tokens | [source](https://longcat.chat/platform/docs/Pricing/LongCat-2.0.html) |
| `meta/muse-spark-1.1` | USD | 1 | million_tokens | [source](https://dev.meta.ai/docs/pricing-rate-limits) |
| `meta/muse-spark-1.2` | USD | 1 | million_tokens | [source](https://dev.meta.ai/docs/pricing-rate-limits) |
| `meta/muse-spark-1.3` | USD | 1 | million_tokens | [source](https://dev.meta.ai/docs/pricing-rate-limits) |
| `mistralai/codestral-2508` | USD | 1 | million_tokens | [source](https://docs.mistral.ai/inference/pricing) |
| `mistralai/ministral-14b-2512` | USD | 1 | million_tokens | [source](https://docs.mistral.ai/inference/pricing) |
| `mistralai/ministral-3b-2512` | USD | 1 | million_tokens | [source](https://docs.mistral.ai/inference/pricing) |
| `mistralai/ministral-8b-2512` | USD | 1 | million_tokens | [source](https://docs.mistral.ai/inference/pricing) |
| `mistralai/mistral-large-4-0` | USD | 1 | million_tokens | [source](https://docs.mistral.ai/inference/pricing) |
| `mistralai/mistral-small-2603` | USD | 1 | million_tokens | [source](https://docs.mistral.ai/inference/pricing) |
| `moondream/moondream3-preview` | USD | 2 | million_tokens | [source](https://moondream.ai/pricing) |
| `morph/morph-v3-fast` | USD | 1 | million_tokens | [source](https://www.morphllm.com/api/models/json?prices=list) |
| `morph/morph-v3-large` | USD | 1 | million_tokens | [source](https://www.morphllm.com/api/models/json?prices=list) |
| `particle-ai/particle-1` | USD | 1 | million_tokens | [source](https://particle.ai/blog/introducing-particle-1) |
| `perceptron/perceptron-mk1` | USD | 1 | million_tokens | [source](https://www.perceptron.inc/blog/introducing-perceptron-mk1) |
| `perceptron/perceptron-mk1.5` | USD | 1 | million_tokens | [source](https://www.perceptron.inc/blog/introducing-perceptron-mk1-5) |
| `qwen-audio-3.1-asr-flash` | CNY | 1 | million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen-audio-3.1-asr-flash-message` | CNY | 1 | million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen-audio-3.1-realtime-flash` | CNY | 2 | million_audio_tokens, million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen-audio-3.1-tts-flash` | CNY | 1 | million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen-audio-3.1-tts-next` | CNY | 1 | million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen-audio-3.1-tts-plus` | CNY | 1 | million_characters | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen-image-2.0` | CNY | 1 | image | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen-image-2.0-pro` | CNY | 1 | image | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen-image-3.0-pro` | CNY | 2 | image | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen-mt-flash` | CNY | 1 | million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen-mt-lite` | CNY | 1 | million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen-mt-plus` | CNY | 1 | million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen-plus` | CNY | 6 | million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen-plus-latest` | CNY | 6 | million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen3-livetranslate-flash` | CNY | 3 | million_audio_tokens, million_image_tokens, million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen3-vl-embedding` | CNY | 2 | million_image_tokens, million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen3-vl-plus` | CNY | 3 | million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen3-vl-rerank` | CNY | 2 | million_image_tokens, million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen3.5-ocr` | CNY | 1 | million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen3.7-plus` | CNY | 4 | million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen3.7-text-embedding` | CNY | 1 | million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen3.7-text-embedding-flash` | CNY | 1 | million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen3.7-text-rerank` | CNY | 1 | million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen3.8-flash` | CNY | 1 | million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen3.8-livetranslate-flash-realtime` | CNY | 3 | million_audio_tokens, million_image_tokens, million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen3.8-max` | CNY | 1 | million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen3.8-max-0902` | CNY | 1 | million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `qwen3.8-omni-flash` | CNY | 1 | million_tokens | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `rekaai/reka-flash-3` | USD | 1 | million_tokens | [source](https://developer.reka.ai/pricing) |
| `thinkingmachines/inkling` | USD | 1 | million_tokens | [source](https://tinker-docs.thinkingmachines.ai/tinker/models/models_and_pricing/) |
| `unbiased/pareto-26.10` | USD | 1 | million_tokens | [source](https://unbiased.ai/changelog/) |
| `upstage/solar-mini4` | USD | 1 | million_tokens | [source](https://console.upstage.ai/docs/models/solar-mini-4.md) |
| `upstage/solar-pro4` | USD | 1 | million_tokens | [source](https://console.upstage.ai/docs/models/solar-pro-4.md) |
| `wan2.7-image-pro` | CNY | 1 | image | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `wan3.0-video` | CNY | 3 | second | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `wan3.0-video-prime` | CNY | 3 | second | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
| `writer/palmyra-x5` | USD | 1 | million_tokens | [source](https://dev.writer.com/home/pricing.md) |
| `z-image-turbo` | CNY | 2 | image | [source](https://help.aliyun.com/zh/model-studio/model-pricing) |
