# 模型广场全目录只读检查（2026-11-16）

## 范围与结论

- 匿名 GET `https://meta-api.com/api/pricing`：240 个公开模型；36 个缺少公开官方参考价。
- 已登录浏览器模型广场以100/100/40分页逐卡检查，240个唯一模型均有数值售价（包含合法零价）。这不是全部分组、全部档位与真实结算的审计。
- 208张卡片展示首字/输出速度指标；52张首字显示暂无数据，53张输出速度显示暂无数据。32张卡片没有这块指标，未算作缺失。
- 对有数值的性能卡片，本次没有逐一核实其来源、采样窗口、可靠性或所有分组历史曲线。不能据此称全站统计正常。
- 目录随线上变动：上轮239个，本轮240个；本次浏览器与接口总数一致。页面统计也会自动刷新，数值为读取时快照。
- 本轮只读线上与源码，未改计费、渠道、模型资料、应用代码或生产服务。只新增本报告。

## 重点问题

### P1：Hydrogen 输出比例与本站所列官方参考价差10倍

`gemini-4-hydrogen` 当前 `model_ratio=24.75`、`completion_ratio=0.5`，没有分档表达式覆盖。
本站公开参考表为输入1.5、输出7.5 USD/M，输出/输入比为5；本站配置却为0.5。
登录企业级分组实见输入9.9、输出4.95人民币/M，这与配置一致。若基础比例应对齐该参考表，输出应为49.5人民币/M。
这是一项基础定价配置疑点，不能通过仅改前端文案解决；也不能未经核实就把所有差价都当成错误。未读取账单或执行付费请求，未量化历史扣费影响。
价格读取与计算入口：`web/src/helpers/utils.jsx:calculateModelPrice`；服务端配置读取：`relay/helper/price.go`；结算：`service/quota.go`。

### P1：Terra 历史速度异常，并可能被补值延续

`gpt-6.1-terra` 官方参考价、分组售价及当前性能均存在。页面历史明细实见福利分组7841.1、5383.3、7258.4 tok/s，多个分组有5000/10000 tok/s；24小时汇总约51–72 tok/s。
窗口汇总按总输出Token/总生成耗时加权，数值低于部分节点不能单独证明汇总错误。
源码确认：成功流式请求在生成耗时>0毫秒且有有效Token使用量时入样；没有最短持续时间或内容分片数量质量门槛。极短、缓冲后集中返回等样本可能产生极高数值，但本轮未取得对应原始请求计时，不能断言就是线上根因。
`getHealthMeasurement(..., true)`、`chartDisplayValue` 还会将过去有效值补入空节点；本地实际函数探针输入空当前值和历史10000，输出仍为10000。故异常会被沿用，曲线不等同于每半小时都有实测。
历史补值是已有产品决定（见2026-11-03模型性能补值来源说明笔记），不应未经评估直接删掉或加任意速度上限。
建议先查原始采样的首输出/最后内容/usage时间及流式分片，明确可用测速条件；区分当前实测与历史参考，避免异常样本持续延展。

### P2：DeepSeek Vision 缺性能而非价格

`deepseek-v4-flash-vision-exp` 现场详情有峰/谷官方参考价与本站售价（企业级输入2/1、输出8/4人民币/M）。首字与输出速度为空，可用率100%。展开小时记录能看到上游测试补充。
测试可使用同渠道其他模型，所以可用率存在并不能证明这个精确模型有可用的首字、Token和生成耗时样本。
不能借用 `deepseek-flash` 或其它别名的性能来填满；需要核实精确模型统计归属与真实调用覆盖。

### P2：Hydrogen 空指标本轮未持续复现

企业级本轮实见首字2.60s、速度163.3tok/s、可用率84.27%；其他三个分组也有性能值。用户此前截图为空不能否定，但当前不能把它认定为永久漏采。
分组价格行使用72小时健康窗口；下方性能图默认24小时/半小时节点。比较两处数字时必须统一窗口。

### P2：非聊天模型性能适用性

缺指标清单含嵌入、重排、语音、结构化决策等类型。部分不输出聊天Token或不支持流式返回，统一展示首字/Token速度会制造“缺数据”观感。
建议复用现有模态与能力字段确定展示哪些指标；不适用与缺样本应区分，不用0或其它模型速度填空。

## 基础输出比例差异筛查

仅筛查非表达式计费、具有唯一且输入非零的 million_tokens 官方参考档位的模型。比较本站completion_ratio与参考表output/input；列出差异严格大于5%的项（容忍浮点误差）。这是候选问题，不是全部价格核对或官方现价认证；分档83项未用这个简化比值判断。

| 模型 | 本站输出倍率 | 参考表输出/输入 | 判断 |
| --- | ---: | ---: | --- |
| `kimi-k2.6` | 5 | 4.21053 | 核实自主定价或旧配置 |
| `glm-5.3` | 3.5 | 3.14286 | 核实自主定价或旧配置 |
| `kimi-k2.7-code` | 5 | 4.21053 | 核实自主定价或旧配置 |
| `gemini-4-hydrogen` | 0.5 | 5 | 优先核实10倍差异 |
| `schematron-v2-small` | 4.6 | 2.5 | 核实自主定价或旧配置 |
| `particle-1` | 9.78365 | 4 | 核实自主定价或旧配置 |
| `schematron-v2-turbo` | 5 | 2.5 | 核实自主定价或旧配置 |
| `glm-5.2` | 3.5 | 3.14286 | 核实自主定价或旧配置 |

## 官方参考价缺失清单

以下为接口已存的核实状态，本次未重新逐一访问全部厂商来源。13项历史适用范围、12项名称映射、9项平台变体、1项有来源待同步、1项未核实；不等于厂商没有价格。

| 模型 | 核实状态 |
| --- | --- |
| `MiniMax-M2.5` | legacy_review |
| `TTseadanco2.5-720` | identity_review |
| `claude-opus-4-20250514` | legacy_review |
| `codex-auto-review` | identity_review |
| `deepseek-v4-flash-0731` | identity_review |
| `deepseek-v4.1-flash` | identity_review |
| `dots-3-note-preview:free` | platform_variant |
| `flint-v1.0:free` | platform_variant |
| `flow-video-gemini-omni-flash-1.1` | identity_review |
| `flow-video-gemini-omni-flash-1.1-1080p` | identity_review |
| `gemini-3-pro-image-preview` | legacy_review |
| `gemini-3.1-flash-image-preview` | legacy_review |
| `gemini-nano-banana-2.1-c` | identity_review |
| `gemini-omni-1.2-flash` | source_available |
| `gpt-6.2-astra` | identity_review |
| `gpt-6.2-sol` | identity_review |
| `gpt-image-2.6` | identity_review |
| `grok-4.7-build-fast` | identity_review |
| `kimi-k2.5` | legacy_review |
| `lfm-2.5-2.6b:free` | platform_variant |
| `muse-video` | unverified |
| `nemotron-3-nano-omni-30b-a3b-reasoning:free` | platform_variant |
| `nemotron-3-ultra-550b-a55b:free` | platform_variant |
| `nemotron-3.5-lightning:free` | platform_variant |
| `nemotron-3.5-super-vl:free` | platform_variant |
| `north-mini-code:free` | platform_variant |
| `qwen-image-max` | legacy_review |
| `qwen-plus-2025-09-11` | legacy_review |
| `qwen-turbo` | legacy_review |
| `qwen-vl-max` | legacy_review |
| `qwen3-235b-a22b` | legacy_review |
| `qwen3-coder-plus` | legacy_review |
| `qwen3-max` | legacy_review |
| `qwen3-max-preview` | legacy_review |
| `snow-grok-imagine-video` | identity_review |
| `xing4.0-29b-a4b:free` | platform_variant |

## 性能空白卡片清单

观察范围为当前登录账户、全部分组目录卡片；卡片可采用本站当前/历史指标及外部参考，不代表所有空白都属于采集故障。

| 模型 | 首字 | 输出速度 |
| --- | --- | --- |
| `deepseek-v4-flash-vision-exp` | 暂无数据 | 暂无数据 |
| `mercury-3` | 暂无数据 | 暂无数据 |
| `jev-1.14` | 暂无数据 | 暂无数据 |
| `glm-5.3-flashx` | 暂无数据 | 暂无数据 |
| `jev-1.13` | 暂无数据 | 暂无数据 |
| `qwen3.8-omni-flash` | 暂无数据 | 暂无数据 |
| `mercury-2.5` | 暂无数据 | 暂无数据 |
| `qwen3.8-max` | 暂无数据 | 暂无数据 |
| `kimi-k2.7-code-highspeed` | 暂无数据 | 暂无数据 |
| `qwen3.7-plus` | 暂无数据 | 暂无数据 |
| `gemini-embedding-2` | 暂无数据 | 暂无数据 |
| `gemini-3.1-flash-tts-preview` | 暂无数据 | 暂无数据 |
| `glm-5.1` | 暂无数据 | 暂无数据 |
| `gpt-5.4-pro` | 暂无数据 | 暂无数据 |
| `mercury-2` | 暂无数据 | 暂无数据 |
| `MiniMax-M2.5` | 暂无数据 | 暂无数据 |
| `claude-haiku-4-5` | 有值 | 暂无数据 |
| `qwen3-max` | 暂无数据 | 暂无数据 |
| `qwen3-vl-plus` | 暂无数据 | 暂无数据 |
| `qwen3-coder-plus` | 暂无数据 | 暂无数据 |
| `gemini-embedding-001` | 暂无数据 | 暂无数据 |
| `qwen3-235b-a22b` | 暂无数据 | 暂无数据 |
| `qwen-mt-plus` | 暂无数据 | 暂无数据 |
| `qwen-turbo` | 暂无数据 | 暂无数据 |
| `qwen-vl-max` | 暂无数据 | 暂无数据 |
| `qwen-plus` | 暂无数据 | 暂无数据 |
| `claude-opus-4-20250514` | 暂无数据 | 暂无数据 |
| `deepseek-v4-flash-0731` | 暂无数据 | 暂无数据 |
| `gemini-3.5-transcribe` | 暂无数据 | 暂无数据 |
| `gemini-3.8-flash-lite-tts` | 暂无数据 | 暂无数据 |
| `gemini-3.8-flash-tts` | 暂无数据 | 暂无数据 |
| `lfm-2.5-2.6b:free` | 暂无数据 | 暂无数据 |
| `qwen-audio-3.1-asr-flash` | 暂无数据 | 暂无数据 |
| `qwen-audio-3.1-asr-flash-message` | 暂无数据 | 暂无数据 |
| `qwen-audio-3.1-realtime-flash` | 暂无数据 | 暂无数据 |
| `qwen-audio-3.1-tts-flash` | 暂无数据 | 暂无数据 |
| `qwen-audio-3.1-tts-next` | 暂无数据 | 暂无数据 |
| `qwen-audio-3.1-tts-plus` | 暂无数据 | 暂无数据 |
| `qwen-mt-flash` | 暂无数据 | 暂无数据 |
| `qwen-mt-lite` | 暂无数据 | 暂无数据 |
| `qwen-plus-2025-09-11` | 暂无数据 | 暂无数据 |
| `qwen-plus-latest` | 暂无数据 | 暂无数据 |
| `qwen3-livetranslate-flash` | 暂无数据 | 暂无数据 |
| `qwen3-max-preview` | 暂无数据 | 暂无数据 |
| `qwen3-vl-embedding` | 暂无数据 | 暂无数据 |
| `qwen3-vl-rerank` | 暂无数据 | 暂无数据 |
| `qwen3.5-ocr` | 暂无数据 | 暂无数据 |
| `qwen3.7-text-embedding` | 暂无数据 | 暂无数据 |
| `qwen3.7-text-embedding-flash` | 暂无数据 | 暂无数据 |
| `qwen3.7-text-rerank` | 暂无数据 | 暂无数据 |
| `qwen3.8-livetranslate-flash-realtime` | 暂无数据 | 暂无数据 |
| `schematron-v2-small` | 暂无数据 | 暂无数据 |
| `xing4.0-29b-a4b:free` | 暂无数据 | 暂无数据 |

## 验证与限制

- 全量字段扫描：157项普通计价、83项表达式计价；对应基础字段或表达式均存在且基础数值有效。9项基础零价均为 `:free` 名称，未将零价计为缺价。
- 浏览器：完成三页、240张唯一卡片检查；每批等加载结束后读取；没有数值价格空白卡片。重点详情检查Terra、DeepSeek Vision、Hydrogen。
- `bun test scripts/model-chart-display.test.jsx scripts/model-group-discount.test.mjs scripts/tiered-billing-display.test.mjs`：价格比较7项通过，档位脚本断言通过；图表套件因缺少 `react/jsx-dev-runtime` 未启动（整条命令失败）。没有为只读调查安装依赖，也不声称完整测试通过。
- `bun -e` 调用真实 `chartDisplayValue`：当前缺值/零样本、历史参考10000时显示10000；当前有效值10000也显示10000。探针为机制复现，不证明线上特定请求产生异常的原因。
- 全量认证健康接口未导出；直接导航因缺少应用用户请求头被拒绝，未绕过鉴权或提取登录凭据。通过正常已登录网页读取可见信息。
- 未进行付费模型调用；未启动开发服务器、监听器或后台测试。所有本轮命令均结束，临时浏览器页已关闭，遗留本轮测试进程为0。

## 推荐推进顺序

1. 核实Hydrogen基础输出倍率是否误填0.5；同步核查上表其它比例差异，保留自主分组优惠。
2. 为极短/缓冲型流式样本建立有证据的测速规则，检查历史补值和异常样本传播；同时验证小时与半小时统计。
3. 按能力区分不适用指标、缺样本、请求失败；保持无有效证据时未知，不借用别名统计。
4. 沿用已有官方同步链路处理36项参考价缺口，优先 `source_available`；其它项保留精确型号与单位约束。

完成代码修复后再验收全目录、代表性分组详情、历史曲线与实际结算；本报告不代表修复、发布或生产部署完成。
