# OpenRouter 按厂商生产接入记录（2026-10-31）

初次接入是生产配置更新，复用已有 OpenRouter 适配器、渠道管理、模型元数据和计费设置；当时没有修改应用程序或部署版本。一个模型厂商一个 OpenRouter 渠道，渠道 ID 为 1592–1637，统一使用 `OpenRouter` 分组（倍率 1），并加入现有自动分组顺序末尾。所有新渠道 `auto_ban=0`，避免单一模型的限流导致同厂商全部模型被自动停用。

## 后续采购限价

2026-11-01 的当前采购路由已按原厂公开价审核：本轮 133 个配置模型保留 50 个、暂停 83 个，采用低价优先和限价内回退。详见[限价记录](openrouter-price-routing.md)与[逐模型清单](openrouter-price-audit-2026-11-01.json)。下文 145 个启用模型及 OpenRouter 分组等数量、设置描述属于初次接入快照。

## 厂商 Logo

图标映射见 [45 条厂商记录](openrouter-vendor-icons.json)，包含 43 个原先为空的 icon 和 Inception、Z.AI 两个旧库无法解析的名称。主站共用图标注册表同时覆盖供应商筛选、卡片、详情及管理页。先部署含本地图标的应用，再通过管理员供应商 API 读取完整记录并仅补齐 icon；保留其他字段与人工配置。资源来源、许可和字节校验见 [图标说明](../../web/src/assets/provider-icons/README.md)。

## 范围与结果

按 [OpenRouter 模型目录](https://openrouter.ai/api/v1/models) 的 452 个文本/聊天模型 ID 依次筛选：268 个属于用户排除厂商；剩余中 15 个已在启用渠道配置；再跳过 16 个 batch、自动路由或 latest 特殊入口，得到 153 个候选。排除厂商为 Tencent、ByteDance、StepFun、Qwen、GLM（z-ai）、OpenAI、Google、xAI、Anthropic。Meta 和 meta-llama 合并为一个厂商。

最终新增 **46 个厂商渠道，启用 145 个模型，其中免费版本 9 个**。153 个候选均保留价格和元数据，8 个受上游限制的模型不在启用渠道能力中，并关闭其展示状态。公开目录由 116 个模型增加至 261 个。已有渠道未被本次更新，原有价格映射条目全部保留。

完整逐模型 ID、厂商渠道、价格和验收状态见 [JSON 清单](openrouter-models-2026-10-31.json)。清单不含凭据；它是本次快照，不是持续自动同步任务。

## 定价口径

沿用「接入阿里模型」约定的**后台价格栏数字**：OpenRouter 美元单价 × 6.8 × 5，即 ×34。输入、输出、缓存读取和音频 Token 分别换算，免费版本填 0。`ModelRatio = 美元输入价/百万 Token ×34÷2`，输出倍率为输出价/输入价；新分组倍率固定为 1。

生产采用自定义能量单位，充值价格为 ¥0.2/能量；这里没有修改全站充值或兑换规则。因此后台 34 能量对应前台 ¥6.8，不能把后台数值直接称为人民币销售价。例如 Mistral Small 4 输入 $0.15/百万 Token → 后台 5.1 能量 → 前台 ¥1.02/百万 Token。

`sakana/fugu-ultra-v2`、`sakana/fugu-ultra` 保留 272,000 输入 Token 的长上下文价格；`deepseek/deepseek-v4-pro-0813` 保留 UTC 工作日 01:00–04:00、06:00–10:00 的峰时价格，使用现有阶梯表达式。目录价格可能与 OpenRouter 具体路由实际成本不同，本次销售配置依据目录快照。

**附加计费边界：**15 个候选条目有联网搜索报价，其中已启用 12 个；搜索单价已按 ×34 写入工具价格配置，但当前 OpenRouter 转发未完整映射实际搜索次数，不能保证这部分附加费被完整结算。`perplexity/sonar-deep-research` 的独立推理 Token 价格也不能通过现有普通输出倍率单独结算。这些不是“全部计费能力已验收”；完整列表保存在 JSON 的 `billing_limits`。音频费率已配置，但没有进行真实音频输入测试。

## 未启用的 8 个模型

没有修改账号隐私设置，也没有伪装已获认可的代理应用。

| 模型 | 原因 |
| --- | --- |
| `meta/muse-spark-1.3-contributor` | 账号隐私设置拒绝用于训练的付费路由 |
| `meta/muse-spark-1.2-contributor` | 账号隐私设置拒绝用于训练的付费路由 |
| `liquid/lfm-2.5-2.6b` | 账号隐私设置拒绝用于训练的付费路由 |
| `sakana/sakana-namazu` | 账号隐私设置拒绝用于训练的付费路由 |
| `thinkingmachines/inkling-small:free` | 上游仅允许受认可的 agentic harness 应用 |
| `thinkingmachines/inkling:free` | 上游仅允许受认可的 agentic harness 应用 |
| `nvidia/nemotron-3.5-content-safety:free` | 上游 NVIDIA 模型函数不存在，重复返回 404 |
| `amazon/nova-premier-v1` | 上游确认该模型版本已结束生命周期 |

## 已启用但本轮未验证成功的 5 个模型

这些模型保留已配置入口，状态不等同于当前可调用成功。

| 模型 | 观察结果 |
| --- | --- |
| `liquid/lfm-2.5-2.6b:free` | 上游连续返回 429 |
| `perplexity/sonar-deep-research` | 一次非流式测试超过 100 秒，尚未验证完成结果 |
| `recursive/flint-v1.0:free` | 上游连续返回 503 |
| `thedrummer/cydonia-24b-v4.1` | 上游连续返回 429 |
| `xingchen-agi/xing4.0-29b-a4b:free` | 上游连续返回 429 |

## 验收

- 153 个候选均进行渠道短请求测试，结合有针对性的复测，140 个已启用模型得到成功响应。短请求通常限制 16 个输出 Token，不能据此推断完整推理、工具或多模态能力。
- ScaleDown Extract 使用 `response_format.type=json_schema` 描述提取实体；Relace Apply 3 使用 `<instruction>`、`<code>`、`<update>` 格式。二者经真实公网请求确认输出结果，普通 `hi` 测试不适用于它们。
- 公网 `/api/pricing`、令牌选择器、受限临时令牌的 `/v1/models` 均得到同一组 145 个模型，8 个未启用模型不在公开列表中。
- 经 `https://meta-api.com/v1/chat/completions` 完成 7 次调用：Mistral 普通/流式、NVIDIA 免费、DeepSeek 分时、Sakana 阶梯、ScaleDown 提取和 Relace 修改。7 次均有有效结果；流式响应有正文、usage 和 `[DONE]`。
- 对应 7 条消费日志的路由分组和倍率均正确；按目录单价 ×34 和实际 Token 数逐条重算，与生产扣费的整数舍入完全一致，免费记录为 0。Sakana 命中 base，DeepSeek 命中 off_peak；峰时和长上下文未做昂贵真实请求。
- 临时测试令牌限定新渠道、固定 OpenRouter 分组、有限额度和有效期，测试后删除。浏览器确认模型详情的官方参考价、公开人民币价格、OpenRouter 分组与上下文信息。
- 最后核验已有 ModelRatio 215 项、CompletionRatio 166 项、CacheRatio 166 项、AudioRatio 11 项及工具价格 10 项均未改变。新增价格与 145 条启用能力一致。

## 厂商渠道

| 厂商 | 渠道 ID | 启用模型 | 未启用 |
| --- | ---: | ---: | ---: |
| Particle AI | 1592 | 1 | 0 |
| Recursive | 1593 | 1 | 0 |
| Elorian | 1594 | 1 | 0 |
| XingChen | 1595 | 1 | 0 |
| Reflection | 1596 | 1 | 0 |
| Inception | 1597 | 3 | 0 |
| PrismML | 1598 | 2 | 0 |
| Moondream | 1599 | 2 | 0 |
| Meta | 1600 | 14 | 2 |
| NVIDIA | 1601 | 10 | 1 |
| unbiased | 1602 | 2 | 0 |
| LiquidAI | 1603 | 2 | 1 |
| ScaleDown | 1604 | 2 | 0 |
| Mistral | 1605 | 20 | 0 |
| inclusionAI | 1606 | 3 | 0 |
| Perceptron | 1607 | 2 | 0 |
| Fireworks | 1608 | 1 | 0 |
| AionLabs | 1609 | 6 | 0 |
| Upstage | 1610 | 2 | 0 |
| Cohere | 1611 | 6 | 0 |
| Inference.net | 1612 | 2 | 0 |
| Sakana | 1613 | 3 | 1 |
| Nex AGI | 1614 | 2 | 0 |
| IBM | 1615 | 1 | 0 |
| Dots Studio | 1616 | 1 | 0 |
| DeepSeek | 1617 | 8 | 0 |
| Thinking Machines | 1618 | 2 | 2 |
| Meituan | 1619 | 1 | 0 |
| MiniMax | 1620 | 8 | 0 |
| Xiaomi | 1621 | 2 | 0 |
| rekaai | 1622 | 2 | 0 |
| Writer | 1623 | 1 | 0 |
| Relace | 1624 | 2 | 0 |
| Amazon | 1625 | 4 | 1 |
| MoonshotAI | 1626 | 3 | 0 |
| Perplexity | 1627 | 5 | 0 |
| TheDrummer | 1628 | 3 | 0 |
| Nous | 1629 | 3 | 0 |
| Venice | 1630 | 1 | 0 |
| Morph | 1631 | 2 | 0 |
| Microsoft | 1632 | 1 | 0 |
| anthracite-org | 1633 | 1 | 0 |
| Sao10K | 1634 | 2 | 0 |
| Mancer | 1635 | 1 | 0 |
| undi95 | 1636 | 1 | 0 |
| gryphe | 1637 | 1 | 0 |

## 运维边界

变更均通过现有管理员 API 进行，数据库仅作只读核验；密钥和管理员凭据只在服务器内存中使用。变更前配置快照、计划、执行状态和详细验收记录保存在生产 `/opt/meta-api/operations/openrouter-20261031`（目录 0700、文件 0600）。首次元数据请求因 `sync_official` 应为整数而失败，改为 0 后按已记录状态继续，无重复渠道或模型。一次 SSH 连接在认证前超时，重连后完成。

需要回退时，通过渠道管理停用这 46 个明确 ID，并从自动分组序列移除本次添加的 OpenRouter 项；不要覆盖整份旧 option 快照，以免抹掉后续合法改动。价格和元数据可保留为停用状态；删除本次新增键前逐项确认没有被后续渠道复用。本次没有停止或重启生产服务。
