# 官方模型资料覆盖复查（2026-11-13）

本次检查全部 240 个公开模型：40 个无官方参考价，44 个缺少输入或输出模态。沿用原有厂家来源与人工保护规则，不以平台零价或相近型号填空。

## 实现与真实文档验证

- Seedance 从单个 2.5 型号扩展到官方能力表中的六个精确型号：2.5、2.0、2.0 Fast、2.0 Mini、1.0 Pro、1.0 Pro Fast。每个版本单独读取能力和价格；1.0 的离线价格、2.0 的 4K 档位不混入其他版本。尚未上架的型号只是获得同步支持，并未新增渠道或公开条目。
- 百炼增加官方独立详情页的精确标题、两列/四列能力矩阵解析。下载 24 个缺模态候选页面，19 个解析成功；旧型号/别名标题不匹配的 5 个拒绝套用。向量输出若不属于现有模态枚举，仍保持未知。
- 同一轮共享官方页面只抓取一次；复用现有六并发上限，失败保留旧数据。
- 以下是只读文档解析覆盖，不等同于线上已写入；实际落库受同步开关、人工所有权、网络和缓存影响。生产验证以发布后公开 API 为准。

| 百炼模型 | 输入模态 | 输出模态 |
| --- | --- | --- |
| `qwen-audio-3.1-asr-flash` | audio | text |
| `qwen-audio-3.1-asr-flash-message` | audio | text |
| `qwen-audio-3.1-realtime-flash` | text, audio | text, audio |
| `qwen-audio-3.1-tts-flash` | text | audio |
| `qwen-audio-3.1-tts-next` | text, audio | audio |
| `qwen-audio-3.1-tts-plus` | text | audio |
| `qwen-image-3.0-pro` | image, text | image |
| `qwen-mt-flash` | text | text |
| `qwen-mt-lite` | text | text |
| `qwen3-vl-embedding` | text, image | 未知 |
| `qwen3-vl-rerank` | text, image, video | text |
| `qwen3.5-ocr` | image | text |
| `qwen3.7-text-embedding` | text | text |
| `qwen3.7-text-embedding-flash` | text | text |
| `qwen3.7-text-rerank` | text | text |
| `qwen3.8-livetranslate-flash-realtime` | audio, image | text, audio |
| `wan3.0-video` | text, image, video, audio, file | video |
| `wan3.0-video-prime` | text, image, video, audio, file | video |
| `z-image-turbo` | text | text, image |

## 仍缺价格的条目

本轮沿用现有解析器复查 40 个缺价项，未取得新的完整价格。不能因此断言厂商没有报价；以下区分来源与映射问题。早前逐项核对见 [11 月 1 日审计](2026-11-01-official-price-coverage.md)。

- `:free` 是平台服务变体，公开层按既有政策不冒充厂家官方免费价格。
- Google 文档从 ai.google.dev 跳转至 aistudio.google.com；新页面使用不同的客户端结构，旧表格解析及同域重定向限制不能直接使用。需独立适配该迁移，不能仅放宽跳转后猜价。
- 平台别名缺少厂家精确 ID 映射；历史型号、版本名或未公开型号不套用新版价格。
- 百炼详情的简化价格表未充分携带区域/上下文/输入输出等完整条件，本次仅扩展属性读取，继续由原价格解析器处理价格。

| 模型 | 本轮状态 |
| --- | --- |
| `qwen-turbo` | 未取得精确型号与完整条件价格；保留未知 |
| `gpt-image-2.6` | 未取得精确型号与完整条件价格；保留未知 |
| `MiniMax-M2.5` | 未取得精确型号与完整条件价格；保留未知 |
| `flint-v1.0:free` | 平台免费变体，不作为厂家价 |
| `nemotron-3.5-lightning:free` | 平台免费变体，不作为厂家价 |
| `gemini-3-pro-image-preview` | Google 文档迁移，未解析到完整价格 |
| `qwen-image-max` | 未取得精确型号与完整条件价格；保留未知 |
| `deepseek-v4.1-flash` | 未取得精确型号与完整条件价格；保留未知 |
| `nemotron-3-nano-omni-30b-a3b-reasoning:free` | 平台免费变体，不作为厂家价 |
| `gemini-4-argon-high` | Google 文档迁移，未解析到完整价格 |
| `grok-imagine-image-edit` | 未取得精确型号与完整条件价格；保留未知 |
| `nemotron-3.5-super-vl:free` | 平台免费变体，不作为厂家价 |
| `qwen-vl-max` | 未取得精确型号与完整条件价格；保留未知 |
| `qwen3-max` | 未取得精确型号与完整条件价格；保留未知 |
| `deepseek-v4.1-flash-1017` | 未取得精确型号与完整条件价格；保留未知 |
| `qwen-plus-2025-09-11` | 未取得精确型号与完整条件价格；保留未知 |
| `lfm-2.5-2.6b:free` | 平台免费变体，不作为厂家价 |
| `north-mini-code:free` | 平台免费变体，不作为厂家价 |
| `grok-4.7-build-fast` | 未取得精确型号与完整条件价格；保留未知 |
| `codex-auto-review` | 未取得精确型号与完整条件价格；保留未知 |
| `flow-video-gemini-omni-flash-1.1` | 未取得精确型号与完整条件价格；保留未知 |
| `xing4.0-29b-a4b:free` | 平台免费变体，不作为厂家价 |
| `glimpse` | 未取得精确型号与完整条件价格；保留未知 |
| `gemini-4-carbon` | Google 文档迁移，未解析到完整价格 |
| `gemini-omni-1.2-flash` | Google 文档迁移，未解析到完整价格 |
| `dots-3-note-preview:free` | 平台免费变体，不作为厂家价 |
| `gemini-4-argon-medium` | Google 文档迁移，未解析到完整价格 |
| `gemini-nano-banana-2.1-c` | Google 文档迁移，未解析到完整价格 |
| `gpt-6.2-sol` | 未取得精确型号与完整条件价格；保留未知 |
| `flow-video-gemini-omni-flash-1.1-1080p` | 未取得精确型号与完整条件价格；保留未知 |
| `gpt-6.2-astra` | 未取得精确型号与完整条件价格；保留未知 |
| `snow-grok-imagine-video` | 未取得精确型号与完整条件价格；保留未知 |
| `nemotron-3-ultra-550b-a55b:free` | 平台免费变体，不作为厂家价 |
| `qwen3-coder-plus` | 未取得精确型号与完整条件价格；保留未知 |
| `kimi-k2.5` | 未取得精确型号与完整条件价格；保留未知 |
| `qwen3-235b-a22b` | 未取得精确型号与完整条件价格；保留未知 |
| `qwen3-max-preview` | 未取得精确型号与完整条件价格；保留未知 |
| `TTseadanco2.5-720` | 未取得精确型号与完整条件价格；保留未知 |
| `gemini-3.1-flash-image-preview` | Google 文档迁移，未解析到完整价格 |
| `muse-video` | 未取得精确型号与完整条件价格；保留未知 |
