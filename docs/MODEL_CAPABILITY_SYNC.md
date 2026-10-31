# 模型资料自动同步

模型管理和模型详情共用上下文 `context_length`、最大输出 `max_output_tokens`、输入/输出模态、逐项能力 `capabilities` 和发布日期。详情在官方参考价之后、分组定价之前独立展示模型属性；桌面五栏排列，能力分两列，窄屏自动换行。

开启模型的“参与官方同步”后，主节点启动后立即同步，此后默认每24小时检查；打开已有模型的编辑框时也会自动检查过期资料，无需逐模型点击同步按钮。管理员仍可手动立即刷新。来源策略是**厂商官方优先，缺失字段用模型库补全**，人工配置优先保留。

## 模型目录自动补齐

管理列表和搜索使用与模型广场相同的启用渠道模型集合补建缺失记录；无需先点击同步，也不要求模型已被远端模型库收录。新增条目立即参与列表分页、搜索与总数统计，未知属性保持为空。

主节点每分钟检查本地目录；管理页发现新模型时通知后台提前更新资料。新模型沿用下面的官方优先来源和人工保护，已有模型默认每24小时刷新；部分来源失败后按5、10、20、40分钟至最多1小时的退避间隔重试。新增配置及重新开启同步也会通知后台。资料更新在后台执行，列表不会等待官方网络请求；更新完成后刷新列表可查看结果。关闭 `MODEL_METADATA_SYNC_ENABLED` 时仍能在管理页补齐本地目录，但后台和编辑框均不自动请求资料。编辑框加载最多等待15秒，失败保留已有信息；同模型成功检查在正常周期内复用，失败至少间隔5分钟。同进程后台、手工和编辑框任务互斥，编辑框不会反复触发全量网络请求。

已有前缀、后缀、包含规则覆盖的模型继续通过该规则行及其匹配列表管理；已人工删除的记录不恢复，渠道下架也不删除已有配置。后台创建与人工新增使用相同事务锁，避免重复条目。超过模型管理128字符上限、空白或首尾有空格的异常渠道名称不自动建档。

管理列表的图标、供应商标签、计数及筛选复用广场默认厂商归类；已人工指定的供应商优先，未配置时按现有名称规则补充展示，未知名称保持未知。接口通过只读 `display_vendor_id` 返回展示归属，`vendor_id` 保留原配置，打开编辑或保存其他字段不会固化推断。管理统计包含保留的历史配置和匹配规则行，因此不保证与广场当前可调用的具体模型数量相等。

## 来源与覆盖范围

| 来源 | 自动读取内容 | 接入条件 |
| --- | --- | --- |
| [OpenAI 模型文档](https://developers.openai.com/api/docs/models/gpt-6-astra) | 介绍、厂商、上下文、最大输出、模态、明确列出的推理/流式/函数调用/结构化输出及工具能力；明确缓存输入单价表示提示缓存支持；标准文本 USD / 1M token 输入、输出、缓存读写参考价；明确的30分钟/12小时等缓存写入时长与长上下文条件组合为独立档位 | 公开页面，无需密钥；只请求本地已配置或启用的 OpenAI 风格精确 ID，校验页面模型身份 |
| [Claude Models API](https://platform.claude.com/docs/en/api/models/list) | `max_input_tokens`、`max_tokens`、明确的 thinking/citations/structured_outputs/server_tools.web_search/server_tools.code_execution 支持状态（顶层 code_execution 是程序化工具调用，不冒充代码执行工具） | 配置专用 `MODEL_METADATA_ANTHROPIC_API_KEY`；固定官方端点和 API 版本 `2026-10-21` |
| [Gemini Models API](https://ai.google.dev/api/models) | `inputTokenLimit`、`outputTokenLimit`、显式 `thinking` 状态 | 配置专用 `MODEL_METADATA_GEMINI_API_KEY`；固定官方端点 |
| [basellm/llm-metadata](https://github.com/basellm/llm-metadata) `/api/all.json` | 上下文、最大输出、模态、发布日期、reasoning/tool_call/structured_output | 复用 `SYNC_UPSTREAM_BASE`；属于社区模型库，界面标为“模型库补全” |

厂商接口不一定提供所有能力。未提供的字段继续补全或显示未知，不根据模型名、某个 API 路径或工具家族推断支持；例如 Claude API 只提供部分输入模态能力，不将其当成完整模态列表。公开文档适配器覆盖下表厂商；其余来源继续由模型库自动补缺。没有公开资料、私有别名或尚未识别的字段保持未知，不保证每个模型的全部能力都有官方证据。专用凭据不从渠道读取，不发送到重定向或模型库；未配置对应凭据时不调用该官方 API。

`capabilities` 是布尔对象：`true` 为支持，`false` 为明确不支持，缺键为未知。管理界面提供三种状态选择。`max_output_tokens` 为非负安全整数，0 表示未知。旧客户端未提交新增字段时保留已有值；显式 0 / `{}` 清空。公开 pricing 和管理查询继续返回只读 `attribute_sources`，按字段区分官方、模型库与人工配置。模型详情的属性区域只展示属性值和能力状态，移除各列来源文字、来源链接及底部参考提示；官方价格区的来源链接和更新时间保留。

这些属性是模型参考资料，不改变本站渠道可用模型、工具协议、计费或权限。介绍、标签和来源确认的供应商也进入同一自动所有权流程。官方未提供标签时可使用模型库的 family/type，并保留社区来源；聚合供应商不能冒充模型原厂商。已有“同步”向导继续支持选择性手工导入。图标仍复用既有厂商显示规则，人工官方参考价始终优先保留。编辑框只提交真正修改的资料字段，能力按键发送 `capability_overrides`（布尔值修改，null 恢复未知），后端在行锁事务中合并当前能力；其他资料从 UPDATE 中排除省略字段，避免把后台刚更新的资料回写成旧表单值。

### 无需专用密钥的公开文档

所有本地精确候选模型自动选择适配器，新版本沿用固定官方路径及身份校验，无需逐个添加配置；不将相似名称或日期后缀直接视为同一个模型。

| 厂商 | 官方入口 | 自动读取范围 |
| --- | --- | --- |
| Anthropic | [模型文档](https://platform.claude.com/docs/en/models/overview) | 模型 API ID、介绍、确切发布日期、上下文/输出、模态、思考和可识别价格；可选 Models API 补充明确能力 |
| Google | [模型文档](https://ai.google.dev/gemini-api/docs/models)、[价格](https://ai.google.dev/gemini-api/docs/pricing) | 精确 model code、目录介绍、Token 限制、模态、能力矩阵及可识别 Standard 价格 |
| DeepSeek | [模型与价格](https://api-docs.deepseek.com/quick_start/pricing/) | 文档表格明确列出的模型、限制、函数/结构化输出、峰谷价格及 UTC 时间条件 |
| 小米 MiMo | [模型](https://mimo.mi.com/static/docs/quick-start/summary/model.md)、[价格](https://mimo.mi.com/static/docs/price/pay-as-you-go.md) | 跨行表格的模型限制和明确能力、海外 token/ASR 价格；国内人民币价不混入 USD 档位 |
| Kimi | [模型列表](https://platform.kimi.ai/docs/models.md) | 精确模型介绍和厂商；其他规格继续从模型库补缺 |
| 千问 | [文本模型](https://help.aliyun.com/zh/model-studio/text-generation-model)、[视觉模型](https://help.aliyun.com/zh/model-studio/vision-model)、[图像模型](https://help.aliyun.com/zh/model-studio/image-model) | 当前及历史表格明确列出的 ID、上下文和支持状态；只接受千问条目，不把代售模型归为阿里 |
| MiniMax | [文本模型](https://platform.minimax.io/docs/guides/text-generation.md) | 模型目录介绍和上下文；大小写或私有别名不猜测映射 |
| xAI | [模型](https://docs.x.ai/developers/models) | 精确 Model name、介绍、上下文、模态及能力；复杂地区/上下文价格未识别时保留已有值 |
| Z.AI | [模型](https://docs.z.ai/guides/llm/glm-5.3)、[价格](https://docs.z.ai/guides/overview/pricing) | API 示例的精确模型身份、介绍、明确限制/能力及标准 USD token 价格 |

公开文档仅允许同官方域名的 HTTPS 重定向，凭据 API 一律拒绝重定向。网页结构与访问限制可能造成部分请求失败；自动重试和社区补缺不能冒充已获取官方证据。

## 官方参考价

OpenAI 官方页面的标准文本 token 价格通过同一次文档请求同步到 `official_pricing`，不读取管理员填写的来源 URL，也不需要 API Key。固定来源校验模型身份、文本价格卡片、USD / 1M 单位和完整输入/输出价格；不从页面的其他模型对比表、Batch/Flex/Fast 选项或本站倍率取值。明确的长上下文门槛与倍率生成独立档位，条件说明原样保留，金额由当前官方页面读取而非写死。

官方价仍是展示资料，独立于本站计费；详情和分组折扣共用同一份价格。`attribute_sources.official_pricing` 返回来源及 `synced_at`，表示该价格表最后一次自动更新的时间；其他属性更新或无变化的检查不会刷新它，人工修改后不再标为自动价格。价格币种、说明和档位作为一整份快照跟踪，管理员修改任意内容后均保留人工版本。

自动价格还包括可识别的 Claude 标准价和缓存 TTL、Gemini Standard USD token 价（明确日期条件按当前 UTC 日期选择）、DeepSeek 峰谷价格及时间说明、MiMo 海外实时/Batch token 和 ASR 小时价、Z.AI 明确的 USD token 价格。地区、缓存 TTL、时间和计费单位分别保留；复杂或尚不支持的价格布局保持原值，不能从社区费用或本站倍率补成官方价格。官方文档不是稳定的 JSON 价格 API；结构、单位或已识别条件不匹配会保留上次完整价格并报告来源异常，不清零。缺少文本价格卡片的模型不新增自动价格。

## 人工配置与自动更新

- 只匹配完全相同的模型 ID，且模型匹配规则必须为“精确匹配”。不从标签推算上下文，不猜测别名或自定义后缀。
- 空字段可以自动补齐。自动填入的值会继续跟随来源更新；管理员修改为不同的非空值后，保留人工值；能力逐键跟踪所有权，人工 false 也受保护。
- 清空字段表示允许重新自动补齐；需要保持空值时，关闭该模型的同步开关。
- 来源没有记录、字段缺失、值不合法或请求失败时，保留已有数据，不清空。官方失败时，社区数据只补空，不降级覆盖上次同步的官方值。来源的 `pdf` 映射到本站“文件”类别，仅表示 PDF 输入，不代表支持所有文件格式。
- 启用渠道中的模型尚未配置展示元数据时，为不受已有规则覆盖的模型补建记录（不依赖来源精确匹配）；已删除记录不自动恢复，不会向渠道增加可调用模型。
- 已关闭同步的模型保持关闭。旧版本同步创建的个别记录可能实际保存为关闭，需按需在模型管理中开启，不批量猜测其原本意图。

内部 TEXT 状态保存实际自动应用过的字段值、来源及同步时间，用于识别手工修改。同步写入带原始字段条件，避免覆盖同时进行的人工保存。完整状态不公开，只返回与当前字段仍一致的来源链接；URL 凭据和查询参数不对外返回。启用模型的自动补建、管理员新增和旧版同步共用能力行的事务锁，防止重复创建；SQLite 使用写事务冲突保护。

## 发布日期与默认排序

模型广场卡片和表格默认按来源的 `release_date` 从新到旧排序，未知日期放在最后，同日期按模型名稳定排序。日期支持 `YYYY-MM-DD` 或 `YYYY-MM`；月精度保留原文本，仅排序时按月起点比较。不会用数据库创建时间、编辑时间、来源的 `last_updated` 或模型名版本号冒充发布日期。

发布日期只适用于精确模型 ID，前缀、后缀、包含规则不会向整个系列传播日期。管理员可在模型管理校正日期，自动同步沿用人工值保护。旧客户端未提交该字段时保留现有日期。原有版本号排序与价格排序仍可选。

## 配置与故障处理

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `MODEL_METADATA_SYNC_ENABLED` | `true` | 控制后台任务，关闭后仍可由管理员立即同步 |
| `MODEL_METADATA_SYNC_INTERVAL_HOURS` | `24` | 检查周期，至少1小时 |
| `SYNC_UPSTREAM_BASE` | `https://basellm.github.io/llm-metadata` | 与原模型库同步共用来源基址 |
| `MODEL_METADATA_SYNC_TIMEOUT_SECONDS` | 180秒 | 全批次网络预算，范围1–600秒；官方单次请求最多10秒，写入另有30秒期限。编辑框在15秒总期限内预留写入时间 |
| `SYNC_HTTP_TIMEOUT_SECONDS` | 30秒 | 既有共享 HTTP 客户端单次请求超时 |
| `SYNC_HTTP_MAX_MB` | `10` | 模型库最大响应大小；官方每次响应最多4 MiB |
| `MODEL_METADATA_ANTHROPIC_API_KEY` | 空 | Claude 官方元数据专用凭据，可选 |
| `MODEL_METADATA_GEMINI_API_KEY` | 空 | Gemini 官方元数据专用凭据，可选 |

手动接口为管理员 `POST /api/models/sync_capabilities`。返回匹配、更新、新建和跳过计数，以及 `pricing_updated`（已写入官方价格的模型数，包含新增记录）和可选 `source_failures`（不可用来源请求数）；部分来源失败时仍应用独立成功的来源，并在管理界面提示，全部失败时返回失败。跳过包含手工保护、禁用、无变化和并发冲突等情形，不能把跳过数当作失败数。后台失败记录日志并有界退避重试，不清空已有值。官方读取并发有界，OpenAI 最多4个、其他公开文档最多6个并发请求；Claude/Gemini 分页上限20页，游标异常中止该来源。后台只在主节点启动，同进程的后台和手动任务互斥。

本次价格同步复用既有 `official_pricing` 与内部 TEXT 状态，无新增列。多实例应继续遵循现有主节点部署规则。代码发布不等于生产部署；本功能要在升级后的服务上才会运行。
