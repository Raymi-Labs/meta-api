# Gemini 4 Carbon 与 Hydrogen 审核规则恢复（2026-11-16）

渠道 1638 的模型列表和映射已有 `gemini-4-carbon` → `google/gemini-4-carbon`，但参数覆盖仍使用旧的 23 型号审核允许列表。请求命中本站 `503 / openrouter_price_unverified`，未发送到 OpenRouter。模型目录同步和官方价格展示不自动更新采购准入规则。

## 核价与修复

[Google 当前官方价格](https://aistudio.google.com/docs/pricing)明确 Standard 输入、输出（含思考）、缓存读取分别为 **$2 / $10 / $0.10 每百万 Token**，有效至 2026-11-30；2026-12-01 起分别为 $4 / $20 / $0.20。显式缓存不受支持。

[OpenRouter 端点报价](https://openrouter.ai/api/v1/models/google/gemini-4-carbon/endpoints)中 `google-ai-studio`、`google-vertex/global` 标准端点的输入、输出、图像输入、音频输入、缓存读取和思考价格符合当前原厂报价。区域加价、priority 和 flex 不纳入本次标准服务允许列表。端点还列有搜索与缓存写入价格；搜索沿用原收费工具禁用规则，原厂不支持的显式缓存未验收，不把基础输入输出上限宣传为所有费用的动态上限。

通过现有管理员渠道 PUT，仅在旧拒绝条件中增加 Carbon 豁免，并增加其服务商允许列表及 `provider.max_price.prompt=2`、`completion=10`、`request=0`。总操作数由 107 变为 111。保留最低价排序、限价内回退、标准服务等级、收费插件和托管工具拒绝；其余未审核模型继续拒绝。图像是输入 Token 单价，沿用现有 Gemini 规则，不增加零美元每张图片过滤。

更新前重新比较当前渠道配置；更新后逐字段读回，并核对全站价格配置、密钥和其他渠道配置哈希不变。未改零售价、模型名、映射、分组、渠道状态或程序版本；未重启服务。无密钥恢复快照及验收记录位于生产私有目录 `/opt/meta-api/operations/carbon-price-20261116`。

## 验证与边界

- 现有实际参数覆盖引擎通过 14 项检查：Carbon 上限、严格客户端上限、普通函数工具、隐私条件保留，未知模型、Hydrogen、收费工具及跨模型回退拒绝。
- 创建仅允许 Carbon 和渠道 1638 的短期验收令牌，关闭跨组回退。一笔请求返回 HTTP 200、正文 `OK`、正常终态及完整 usage：输入 36、输出 5 Token，无缓存读写。
- 响应及独立 generation 查询均确认 Google 服务商和上游费用 $0.000122，等于 `36×2/1e6 + 5×10/1e6`；本站仅一条消费记录，渠道 1638、quota 2013，Request ID `20261116105053126782664sMcIX4dV`。临时令牌已删除。
- 仅验收普通文本；未验证流式、多模态、缓存、长上下文或真实故障回退。没有用重试绕过定价失败。

当前采购上限是本次审核快照，不会自动跟随 12 月涨价；届时若端点超过上限，将保留拒绝，需重新核价。未发布新应用版本或 Release。


## Hydrogen 与后台查看入口

用户随后反馈 Hydrogen 同样失败。复核 [Google Standard 报价](https://aistudio.google.com/docs/pricing#gemini-4-hydrogen)输入、输出、缓存读取为 $1.50/$7.50/$0.075 每百万 Token；[OpenRouter 端点](https://openrouter.ai/api/v1/models/google/gemini-4-hydrogen/endpoints)中 `google-ai-studio` 与 `google-vertex/global` 标准端点各分项符合原厂价。复用相同的增量更新方法，保留 Carbon 及其他现有规则，操作数由 111 变为 115；Hydrogen 输入/输出上限为 1.5/7.5。

“原厂价格审核”是本站采购准入配置，并非 Google/OpenRouter 的审批流程。后台路径为 **渠道管理（`/console/channel`）→ 1638「OpenRouter · gemini」→ 编辑 → 参数覆盖**。数据库字段为 `channels.param_override`；规则代码 `openrouter_price_unverified`，历史来源为 `scripts/openrouter-price-policy.py`。模型列表和映射中有名称，不代表覆盖规则也已允许；新增模型必须同时核对规则。

本次将渠道 1638 的拒绝文字改为“当前渠道尚未配置该模型的采购价格上限，请联系管理员更新渠道参数覆盖。”，保留错误码与 503 状态、未知模型拒绝和其他安全条件；没有删除整套覆盖规则，没有改写其他渠道的历史提示。

最终配置通过 27 项实际覆盖引擎检查，覆盖 Carbon、Hydrogen 放行及旧 preview 型号继续拒绝。Hydrogen 限定渠道的一次真实请求返回 200 和 `OK`：输入36、输出20 Token（其中思考15），上游响应费用 $0.000204，符合 `36×1.5/1e6 + 20×7.5/1e6`。独立 generation 查询确认 Google 服务商及相同 Token/成本；本站仅一条消费记录，quota 1139，Request ID `20261116105434976076905rVeT1IyW`。短期令牌已删除，价格与其他渠道配置哈希保持一致。恢复及验证材料保存在生产 `/opt/meta-api/operations/hydrogen-price-20261116`。

生产保持 `main-0fc874c`，本次仅配置与运维文档更新，不代表做过应用升级。
