# 企业级其余型号价格校准（2026-11-01）

本次在线修正29个精确型号，`codex-auto-review` 按既有规则继承 `gpt-5.3-codex`，同步成为第30个受影响名称。生产仍为 v1.12.1，无程序部署、停服或重启。此前[八个型号](retail-official-correction.md)的修正保持。

美元原厂单价 ×6.6×5 = 能量售价；人民币原厂单价仅乘5。以下人民币价格按企业级倍率1、时段倍率1、每能量0.2元折算。其他分组沿用原倍率，因此这些名称的全局基础价变更也影响其他分组。官方人民币参考显示有独立汇率，与固定6.6换算会存在小幅差异。

用户明确排除 `k3`、`kimi-k2.8`、`kimi-k2.8-code`，三者配置和公开价格保持。Z-Image 经实际路径复核不需要加价：默认/关闭改写为0.1元/张，显式开启改写为0.2元/张，已有数量、重试防重复和两倍收费逻辑；上一轮仅按 ModelPrice 判断漏掉了该逻辑。[原厂默认值及参数](https://help.aliyun.com/zh/model-studio/z-image-api-reference)。

## 文本模型

单位：人民币/百万Token。除注明外展示标准短上下文。

| 型号 | 输入 | 输出 | 缓存读取 | 其他修正 |
|---|---:|---:|---:|---|
| gpt-5、gpt-5.1、gpt-5.1-codex-max | 8.25 | 66 | 0.825 | 取消旧站内价残留的0.64系数 |
| gpt-5.2、gpt-5.2-codex、gpt-5.3-codex | 11.55 | 92.4 | 1.155 | 同上；codex-auto-review继承后者 |
| claude-opus-5、claude-opus-4-6、claude-opus-4-7、claude-opus-4-8 | 33 | 165 | 3.3 | 写入5m/1h为41.25/66 |
| claude-sonnet-4-6 | 19.8 | 99 | 1.98 | 写入24.75/39.6 |
| claude-fable-5 | 66 | 330 | 6.6 | 写入82.5/132 |
| claude-fable-5-1 | 66 | 330 | 1.65 | 写入82.5/132 |
| claude-sonnet-5-5 | 13.2 | 66 | 0.66 | 输入输出原已正确，修正缓存 |
| grok-4.5 | 13.2 | 39.6 | 1.98 | 输入上下文超过200000时三项翻倍 |
| grok-4.6、grok-4.7 | 13.2 | 39.6 | 3.3 | 输入上下文超过200000时三项翻倍 |
| MiniMax-M3 | 1.98 | 7.92 | 0.396 | 超过512000时三项翻倍；当前渠道固定标准服务档 |
| kimi-k3 | 19.8 | 99 | 1.98 | 默认缓存写入19.8，原25；未声称1h用量已验收 |
| glm-5 | 4 / 6 | 18 / 22 | 1 / 1.5 | 国内价；低档 `<32000`，高档 `≥32000` |
| glm-5.1 | 6 / 8 | 24 / 28 | 1.3 / 2 | 国内价；同上，纠正旧缓存和边界 |
| qwen-plus、qwen-plus-latest | 0.8 / 2.4 / 4.8 | 非思考2 / 20 / 48；思考8 / 24 / 64 | 显式0.08 / 0.24 / 0.48；隐式0.16 / 0.48 / 0.96 | ≤128000、≤256000、其余长档；默认关闭思考；写入1 / 3 / 6 |
| qwen3.7-plus | 1.6 / 4.8 | 6.4 / 19.2 | 显式0.2 / 0.6；隐式0.4 / 1.2 | ≤256000与长档；输入输出按公开8折；缓存以标准输入价为基准，写入2.5 / 7.5 |

来源：[OpenAI](https://developers.openai.com/api/docs/pricing)、[Claude](https://platform.claude.com/docs/en/about-claude/pricing)、[Grok 4.5](https://docs.x.ai/developers/models/grok-4.5)、[4.6](https://docs.x.ai/developers/models/grok-4.6)、[4.7](https://docs.x.ai/developers/models/grok-4.7)、[MiniMax](https://platform.minimax.io/docs/pricing/overview)、[Kimi](https://platform.kimi.ai/docs/pricing/chat)、[智谱国内](https://docs.bigmodel.cn/cn/guide/start/pricing)、[阿里价格](https://help.aliyun.com/zh/model-studio/model-pricing)、[Qwen思考默认值](https://help.aliyun.com/zh/model-studio/deep-thinking)、[显式/隐式缓存](https://help.aliyun.com/zh/model-studio/context-cache)。精确型号来源与独立原价保存在审计JSON。

Qwen 通过实际传递的顶层 `enable_thinking` 判断，只有 true 使用思考价；当前渠道未覆盖此字段。缓存标记只识别 `messages[].content[].cache_control.type=ephemeral`，不会把提示词中的同名文字或上游忽略的工具标记当成显式缓存。先区分缓存模式、再分思考模式和上下文；输入、输出、缓存读写均保留完整字面价格，供既有前端解析。

## 图片和视频

| 型号 | 修正后企业级人民币价 | 原配置 |
|---|---|---|
| grok-imagine-image | 输出0.132元/张 | 0.4元/张 |
| qwen-image-2.0-pro | 0.5元/张 | 0.6元/张 |
| grok-imagine-video | 480P 0.33、720P 0.462元/秒 | 各分辨率0.6元/秒 |
| grok-imagine-video-1.5 | 480P 0.528、720P 0.924、1080P 1.65元/秒 | 各分辨率1.2元/秒 |
| wan3.0-video | 480P 0.21、720P 0.42、1080P 0.84元/秒 | 0.3 / 0.6 / 1.2元/秒 |

来源：[Grok图片](https://docs.x.ai/developers/models/grok-imagine-image)、[Grok视频](https://docs.x.ai/developers/models/grok-imagine-video)、[Grok视频1.5](https://docs.x.ai/developers/models/grok-imagine-video-1.5)、[阿里价格](https://help.aliyun.com/zh/model-studio/model-pricing)。Wan按公开7折价格；活动页面没有明确结束时间，不编造自动恢复日期。

图片保留实际输出张数结算与重试保护。视频沿用实际转换后请求快照，按有效分辨率和输出时长计价，未知档位拒绝；Grok为1–15整数秒，Wan保留原有效时长校验和样本数量规则。前端既有通用表达式展示按秒价格范围，不把未识别模板猜成分辨率标签。

这里校准输出单价，没有新增输入图片附加费或输入视频时长计量。不能把请求的输出时长当成输入媒体用量。Kimi当前OpenAI路径也没有独立1h用量验收。这些范围不等于完整多模态账单与所有缓存TTL均已验证。

## 生产操作与验证

通过既有管理员 options API 完成7步：完整新表达式原子承接 → ModelRatio / CompletionRatio / CacheRatio / CreateCacheRatio → ModelPrice两项 → 普通型号恢复原路径。每步核对目标旧值并读回完整写入表；私有日志保存尝试和目标级恢复信息。没有直接写SQL、改渠道/密钥/分组/充值/上游限价或改变应用版本。

- 实际Go价格助手、用量构造、表达式引擎59136项检查，覆盖过渡/最终状态、零用量、上下文边界、思考和缓存模式、Claude TTL、分组与时段；浮点边界独立复算允许最多1 quota取整误差，原始价格严格比较。
- 实际前端357项价格解析、人民币/K/M/分组与视频秒价检查；实际图片预扣路径96项数量、改写开关、重试、时段和分组检查；关联后端与前端回归通过。
- 26次低成本文本请求：19次成功，19条日志与令牌实际扣费均按新官方分项复算一致。三个Grok请求返回 `no_available_channel`，Qwen Plus两个名称各两种模式共4次返回 `bad_response_body`；7次均确认0消费，临时令牌已删除。Qwen该渠道变更前24小时已有14条失败记录。没有把这5个型号记为真实推理验收成功。
- 三个视频共8个分辨率档位通过生产本地预检，走实际适配、请求转换与预估路径，且上游请求数为0。未进行付费图片/视频生成，未发送付费长上下文或保证缓存命中的请求。
- 管理配置立即读回一致；公开目录有1分钟缓存，第一次读回旧值，刷新后外部域名与本地目录全部售价一致。原非目标哈希不一致仅来自 `codex-auto-review` 继承；排除该预期别名变化后，其余194个公开型号逐字段一致，包括用户排除三项和前次八项。
- 三条Claude OpenRouter Chat账单的初次验收错误地使用原生Messages截断；按实际Chat四舍五入重算原日志后全部一致，未重新请求，也未修改真实账单。

[完整审计](enterprise-price-alignment-audit-2026-11-01.json)保留原价、前后配置、来源、失败和修正后的验收结果。服务器 `/opt/meta-api/operations/enterprise-price-alignment-20261101` 保存私有快照、脚本及原始回执；仓库仅保存脱敏数据。

本次处理有证据的审计发现，未宣称此前206个企业级型号中缺少官方元数据的105个已经全部校准。价格不追溯历史账单，在途请求继续使用冻结快照。仅配置及审计文档更新，不创建新应用版本或Release。
