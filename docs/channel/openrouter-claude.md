# OpenRouter Claude 企业级接入（2026-11-01）

新增 **1639：OpenRouter · Claude**，类型 20，分组为 **企业级**，倍率 1。使用当前 OpenRouter 账号，15 个普通 Claude 型号单渠道管理。原有其他 Claude/OpenRouter 渠道、14 个已配置型号售价、分组倍率和用户令牌保持原值。

## 模型与价格

[当前目录](https://openrouter.ai/api/v1/models)的 30 个 Anthropic 条目中，15 个普通型号全部通过当前账号的受限价短请求。按既有用户决定跳过 15 个 batch；不自动同步新增型号。对外使用原厂风格 ID，已有型号沿用站内名称，内部通过精确映射转为 OpenRouter ID。

| 站内名称 | OpenRouter ID | 销售价 |
| --- | --- | --- |
| `claude-haiku-5-5` | `anthropic/claude-haiku-5.5` | 沿用现价 |
| `claude-sonnet-5-5` | `anthropic/claude-sonnet-5.5` | 沿用现价 |
| `claude-opus-5-5` | `anthropic/claude-opus-5.5` | 沿用现价 |
| `claude-fable-5-1` | `anthropic/claude-fable-5.1` | 沿用现价 |
| `claude-opus-5` | `anthropic/claude-opus-5` | 沿用现价 |
| `claude-sonnet-5` | `anthropic/claude-sonnet-5` | 沿用现价 |
| `claude-fable-5` | `anthropic/claude-fable-5` | 沿用现价 |
| `claude-opus-4-8` | `anthropic/claude-opus-4.8` | 沿用现价 |
| `claude-opus-4-7` | `anthropic/claude-opus-4.7` | 沿用现价 |
| `claude-sonnet-4-6` | `anthropic/claude-sonnet-4.6` | 沿用现价 |
| `claude-opus-4-6` | `anthropic/claude-opus-4.6` | 沿用现价 |
| `claude-opus-4-5` | `anthropic/claude-opus-4.5` | 沿用现价 |
| `claude-haiku-4-5` | `anthropic/claude-haiku-4.5` | 沿用现价 |
| `claude-sonnet-4-5` | `anthropic/claude-sonnet-4.5` | 沿用现价 |
| `claude-opus-4-1-20250805` | `anthropic/claude-opus-4.1` | 新增：原厂美元价 ×33 |

唯一新增售价的 `claude-opus-4-1-20250805` 依据 [Anthropic 官方价表](https://platform.claude.com/docs/en/about-claude/pricing)，以官方美元价 ×6.6×5，即 ×33 写入后台能量价格：

| 每百万 Token 分项 | 官方美元价 | 本站能量价（分组前） |
| --- | ---: | ---: |
| 输入 | 15 | 495 |
| 输出 | 75 | 2475 |
| 缓存读取 | 1.5 | 49.5 |
| 缓存写入 5m | 18.75 | 618.75 |
| 缓存写入 1h | 30 | 990 |

对应 ModelRatio 247.5、CompletionRatio 5，原有历史 CacheRatio 0.1 与 CreateCacheRatio 1.25 保持，1h 按现有 1.6 倍派生。官方参考价元数据单独保存美元价；全站充值单价仍为 ¥0.2/能量。

Opus 4.1 在原厂直接 API 已退役，但[官方生命周期说明](https://platform.claude.com/docs/en/about-claude/model-deprecations)明确合作云平台独立安排退役。此次仅接入已实测可用且价格合规的 OpenRouter Amazon Bedrock 端点，不声称原厂直连恢复。

## 采购与协议边界

复用[原厂价限价规则](openrouter-price-routing.md)：`provider.sort="price"`、`allow_fallbacks=true`、按原厂标准价限制输入/输出、审核后的服务商名单、默认服务等级。地区加价端点排除；未知模型、跨模型回退、收费搜索及托管工具在转换后的请求上被拒绝，客户端执行的 function tools 保留。公网 Chat DTO 不接收 `plugins` 字段，渠道还会将出站 plugins 固定为空；因此传入该未知字段可能作为普通 Chat 请求完成，不能宣称所有原始插件请求都会返回 400。渠道/全局请求体透传关闭。

Haiku 5.5 的 100k 和 Sonnet 4.5 的 200k 档采用保守标准档上游限价；长上下文请求可能因没有满足该上限的端点而失败，不自动放宽预算。缓存报价按本次服务商快照审核，OpenRouter 没有独立缓存硬限价，因此不能承诺将来缓存涨价会实时拦截。本次不增加定时核价或自动追价任务，也未更改账号隐私与服务条款设置。

完整端点报价、允许/排除名单、官方来源、15 个 batch 名称与唯一新增价格见[审核 JSON](openrouter-claude-audit-2026-11-01.json)。

## 执行与验证

只通过管理 API 配置，数据库只读核验；服务全程在线，运行版本 v1.11.12。先生成计划、核价、真实上游探测与实际覆盖引擎检查，再以停用状态创建渠道并配置模型及价格，读回后启用。

预检的名称查询遇到 MySQL 字符串排序规则差异，未写入前改为精确二进制比较。第一次保存后的校验使用字符串比较，后台将设置规范化为默认对象并省略 false，导致校验中止；自动保持 1639 停用并恢复两个价格字段。确认语义相同后按已创建渠道与模型继续，未重复创建；明确验证透传仍关闭、代理为空、其他字段符合计划，重新配置并启用成功。

15 次直接 OpenRouter 请求均返回 200 和正文，合计推理成本约 $0.0035215。上游实际价格未超审核上限。实际 `ApplyParamOverride` 引擎完成 195 项检查；`relay/common` 参数覆盖测试通过。新增 Opus 4.1 输入/输出/缓存及三种分组倍率的 18 项计费核验通过。

15 个型号逐个通过站内 Chat 请求，加测 Sonnet 5.5 流式（正文、usage、`[DONE]`）和新 Opus 4.1 Messages，共 17 笔成功请求。每笔消费日志均落到 1639/企业级，倍率 1，仅一次消费，价格复算与令牌 used_quota 一致。公开 pricing 与企业级令牌目录均覆盖 15 项；15 个 batch 未接入。新 Opus 4.1 的 Chat/ Messages 同为 11 输入、4 输出 Token，分别收 7673/7672 quota，差异来自既有 Chat 四舍五入与 Messages 截断规则，没有为了此次接入改变程序。

收费 `web_search_options` 请求返回 400，消费记录为零且令牌扣费为零。最初以不被 DTO 接收的 `plugins` 字段测试拒绝时，返回了普通 Chat 结果并按普通用量扣费；源码及出站规则确认该字段未用于启用插件，随后使用实际支持的搜索字段完成拒绝验证，未改规则来迎合测试。

测试第 11 次令牌搜索触发每用户搜索限流；保留已成功的 10 笔结果，改用只读精确 ID 查询定位本次新令牌，清理未发请求的残留测试令牌后续测。恢复查询一度使用错误的时间列名，读操作失败后按 token schema 改为 created_time，无数据写入。最后所有测试令牌均已删除，已有 OpenRouter/Claude 渠道静态配置摘要及其他价格选项保持一致。完整结果见审计 JSON 和服务器私有回执。

真实缓存命中/1h 写入、昂贵长上下文、全部工具与多模态未逐项调用；本次短请求和计费引擎核验不代表这些能力已完整验收。

私有操作目录 `/opt/meta-api/operations/openrouter-claude-20261101` 保存快照、计划、前后值、失败恢复、上游和网关验收回执，凭据仅在服务器内读取使用。恢复时先停用 1639，再按预期当前值撤销本次新增的 ModelRatio/CompletionRatio 条目；不得整张覆盖旧价格表或删除已有缓存配置。新模型元数据与停用渠道可保留用于审计。

本次是配置与运维记录更新，不发布新应用版本。见[决策笔记](../../.agents/notes/implemented/process/2026-11-01-openrouter-claude-enterprise.md)。
