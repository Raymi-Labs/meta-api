# 缺失模型指标的公开参考来源核查（2026-11-17）

本次为独立、限时、只读研究；唯一写入文件是本报告。结论依据 OpenRouter 第一方匿名 API 和官方文档，不依据搜索摘要或第三方文章。未读取凭据、浏览器 Cookie 或个人资料，未发起推理、收费请求或认证 API 请求，未改代码、配置、能力地图或其他审计文件。

## 结论

- Flint、North Mini Code 的精确免费变体各返回一个端点；最近 24h、30m、5m 可用率均为数值 100。延迟和吞吐量父对象均为 null，所以没有可用的 p50 数值。[S1][S2]
- 官方 schema 明确说明：延迟是最近 30 分钟的首 token 时间，单位毫秒；吞吐量是最近 30 分钟的输出 token/秒。两者只对 API key 或 Cookie 认证请求可见，匿名请求返回 null。因此这里的 null 不能用来断言“没有流量”或“平台没有统计”。[S4]
- 24h uptime 是满足条件的分钟中“至少 80% 的 provider attempts 成功”的分钟占比，只有尝试数不少于 10 的分钟才纳入，并排除限流和调用方造成的失败。100% 并不等于每个请求均成功，也不等于覆盖全天每一分钟。[S4]
- 本次匿名响应没有 cache_hit_rate、完整生成时长或实际 token 消耗量。不能用缓存支持开关、token 上限、单价、文档示例数值补这些缺口。[S1][S2][S3][S4]
- 官方另有认证后的单请求用量/生成时长，以及平台排名数据集；本次仅检查文档，没有取得目标模型对应实值，不构成可直接填入的参考数据。[S5][S6][S7]

## 采集范围与时间

网络请求从 **2026-11-17 17:42:09.075 UTC** 到 **17:43:36.175 UTC**，约 87.1 秒，低于 5 分钟；共 12 次只读 GET，无自动重试。所有抓取均已结束。

| 精确模型 / 实际端点 | 获取完成时间 UTC | HTTP | 24h / 30m / 5m | latency_last_30m | throughput_last_30m |
|---|---|---:|---|---|---|
| recursive/flint-v1.0:free / Recursive / recursive/bf16 | 17:42:09.847 | 200 | 100 / 100 / 100 | null | null |
| cohere/north-mini-code:free / Cohere / cohere | 17:42:10.155 | 200 | 100 / 100 / 100 | null | null |
| openai/gpt-4 / Azure / azure | 17:42:45.233 | 200 | 100 / 100 / 100 | null | null |

GPT-4 精确型号取自官方 endpoint 文档的路径示例，作为额外对照；实际返回的是 Azure 端点，不能把文档中的 OpenAI 示例端点或示例速度当作该次实测。上述时间是抓取完成时间；响应未给出指标的统计截止时间、原始样本数或有效分钟数。响应 HTTP Date 分别为 17:42:08、17:42:08、17:42:43 GMT，不等于指标测量截止时间。[S1–S4]

之前的 /tmp/meta-availability-flint.txt 和 /tmp/meta-availability-north.txt 已只读检查，指标与本次回读一致；本报告以新抓取结果为准，不将旧文件时间假定为统计时间。

## 字段、单位、边界

| 字段 | 第一方定义 / 时间窗口 | 本次实际可得 | 能否填缺失项 |
|---|---|---|---|
| data.endpoints[].latency_last_30m.p50 | 端点最近 30 分钟首 token 时间的中位数，ms | 三个模型的父对象全部 null | 不能；缺失不转 0，不用文档示例 |
| data.endpoints[].throughput_last_30m.p50 | 端点最近 30 分钟输出速度中位数，tokens/s | 三个模型的父对象全部 null | 不能；不能据此计算生成时长或 token 总数 |
| uptime_last_1d | 最近一天、符合流量门槛的分钟可用率，0–100 百分比 | 三端点均 100 | 可作为标注来源/窗口的端点参考可用率，不是本站成功率 |
| uptime_last_30m / uptime_last_5m | 字段窗口分别为 30 分钟 / 5 分钟；5m 文档明确沿用上述有效分钟规则，30m 的字段描述未单独展开算法 | 三端点均 100 | 可作为对应窗口参考，不能生成不存在的历史或样本数 |
| supports_implicit_caching | 缓存支持布尔值，不是测得命中率 | Flint、North 均 false | 不能填 cache_hit_rate=0；也不证明所有缓存形式均不可用 |
| cache_hit_rate | 目标匿名响应及 endpoint schema 未列此字段 | 不存在 | 不能填；仅说明本轮来源无此字段，不推论所有接口均没有 |
| perf_last_30m_by_workload | 按请求类型划分的最近 30 分钟端点性能，仅认证可见 | 三个匿名端点均未返回此键 | 不能填；schema 能力不等于目标数据已获取 |
| perf_last_30m_by_workload.<workload>.request_count | 该 workload 在窗口内被接受的请求总数 | 未返回 | 不能当成本站请求量，也不能当成 uptime 的分母或有效分钟数 |
| context_length / max_completion_tokens / max_prompt_tokens | 容量上限；不带消费时间窗口 | 目标接口有容量字段，部分为 null | 不能填实际使用量、平均输入/输出长度 |
| pricing.prompt / pricing.completion / 缓存单价 | 每 token 价格，非用量计数 | 目标免费端点输入/输出价格为字符串 "0" | 不能填 token 用量或缓存命中率 |

以上 endpoint 字段定义和规则来自 [S4]；实际值/缺字段来自 [S1–S3]。所有参考均属于 OpenRouter 的具体模型端点，不能换名为模型厂商全局指标、所有供应商的统一模型指标、本站分组指标或本站用户调用结果。

### 延迟单位：已找到明确来源，仍有实值验证边界

[S4] 的 PublicEndpoint.latency_last_30m 引用 PercentileStats，后者原文为：

> Latency percentiles in milliseconds over the last 30 minutes. Latency measures time to first token. Only visible when authenticated with an API key or cookie; returns null for unauthenticated requests.

p50 原文为 “Median (50th percentile)”。这给出当前毫秒假设的明确文档依据；若展示秒，应在该语义下换算 ms / 1000。本次没有认证后的非空实值，无法独立验证运行时数值是否始终遵守 schema，不能宣称完成了真实单位校准。

同一页面示例同时写旧字段 latency_last_30m.p50=0.25、workload 的 latency.p50=250；两处说明均指向毫秒。示例量级并不统一，不能凭该样例推翻文字定义或武断更改为“旧字段单位秒”。这些均为文档示例，绝不作为目标模型补值。

[S4] 对 throughput_last_30m 单独说明：

> Throughput percentiles in tokens per second over the last 30 minutes. Throughput measures output token generation speed; pN is the speed that N% of requests reached or exceeded, so p90 is lower than p50. Only visible when authenticated with an API key or cookie; returns null for unauthenticated requests.

因此吞吐量的 p90 方向与耗时百分位不同；不能把 p90 当成更快的上尾速度。通用 PercentileStats 被复用，不应把它的毫秒描述误套到吞吐量字段。

### 可用率不等于请求成功率

[S4] uptime_last_1d 原文：

> Uptime percentage over the last day: the share of minutes in which at least 80% of provider attempts succeeded, counting only minutes with 10 or more attempts. Rate-limited and caller-caused failures are excluded. Null when no minute had enough traffic.

这意味着：即使返回 100，也不能推出失败请求为零；没有有效分钟时应为 null，不应转成 100；API 没有给出分子/分母或有效分钟总数，不能构造这些值。status=0 是状态值，不能推导比例。

## 完整生成时长、缓存和 token 用量的其他官方能力

### 单请求生成记录（仅查文档，没有获取记录）

[S5] 定义 GET /api/v1/generation?id=...，要求 generation ID，安全方案为 Authorization Bearer API key。其数据是某一次生成，不是匿名模型级 24h 或 30m 聚合：

| 字段 | schema 明确定义 | 填补限制 |
|---|---|---|
| data.created_at | 生成创建时间，ISO 8601 | 单请求时间，不是聚合窗口 |
| data.generation_time | Time taken for generation in milliseconds | 生成耗时，非公开模型 p50；文档此处未明确计时起止边界，不擅自与其他时延相加 |
| data.latency | Total latency in milliseconds | 该接口总延迟；不能与 endpoints 的首 token latency 混用 |
| data.tokens_prompt / tokens_completion | 该请求输入 / 输出 token 数 | 不能代替模型全平台日使用量 |
| data.native_tokens_prompt / native_tokens_completion / native_tokens_cached | provider 回报的原生输入 / 输出 / 缓存 token 数 | 数据可能为 null；未拿到目标请求数据 |
| data.usage | Usage amount in USD | 名称 usage 在这里是金额，不是 token 数 |

[S6] 的 Usage Accounting 文档说明每次生成响应提供 usage；Chat Completions 包括 usage.prompt_tokens、completion_tokens、total_tokens、prompt_tokens_details.cached_tokens、prompt_tokens_details.cache_write_tokens；Responses API 对应 input_tokens、output_tokens、total_tokens、input_tokens_details.cached_tokens。cached_tokens 是读缓存 token 数，cache_write_tokens 是写缓存 token 数，不能合并冒充命中次数。

只有持有同一口径、同一窗口真实请求的输入和缓存读 token 数，才可能另行定义“缓存 token 占比”；它仍不等于“命中请求数 / 总请求数”。本轮没有这些数据，不输出任何推算比率。

[S4] 还明确 workload latency 对 image_generation / video_generation 表示端到端生成时长，其他 workload 表示首 token 或首结果时间；这不提供 Flint / North 文本完整生成时长。即使拿到 TTFT 中位数和输出速度中位数，也不能从它们还原完成时长中位数。

### 平台用量排名数据集（仅查文档，没有请求数据 API）

[S7] getRankingsDaily 说明每天按 total token 用量返回前 50 个公开模型，并将其余模型汇总为 other；total_tokens 的口径为 prompt_tokens + completion_tokens。行身份为 (date, model_permaslug)，可以按 day/week/month 粒度查询。category 或 language_type 过滤使用采样后放大的周粒度估计，不能当作精确日统计；不同上游使用各自 tokenizer，不宜当作完全同尺度比较。

该数据集要求有效 OpenRouter API key；文档要求引用为 “Source: OpenRouter (openrouter.ai/rankings), as of {as_of}.” 并声明 CC BY 4.0。它是 OpenRouter 全平台模型用量，不是本站/分组/端点用量。本轮没有认证请求，也未核实两个精确免费变体是否有独立行；落入 other 时不能反推出单模型量。

## 可负责提供给主审计的结论

1. 可以继续使用已有精确端点的 24h 参考可用率，但必须保存“平台参考 + 端点 + 窗口”的语义，不写成请求成功率，也不覆盖本站实测。
2. 目前不能通过这些匿名 endpoint 请求补上两个模型的延迟、吞吐、缓存命中、完整生成时长或实际 token 消耗。匿名 null 是文档明确的可见性行为。
3. 现有速度处理的毫秒单位有官方 schema 支持；本轮只确认文档契约，没有认证样本验证。不要凭样例量级或直觉自动改单位。
4. 后续若另行授权认证读取，可检查相同端点的真实性能；仍需处理 null、精确模型/端点匹配、窗口、数据时效和 workload 差异。这不是本次已完成项。
5. 单请求记录、平台排名、本站调用统计是不同数据范围，不能仅为了填满页面互相替代。

## 来源与请求结果

- [S1] https://openrouter.ai/api/v1/models/recursive/flint-v1.0:free/endpoints — HTTP 200，实时匿名响应。
- [S2] https://openrouter.ai/api/v1/models/cohere/north-mini-code:free/endpoints — HTTP 200，实时匿名响应。
- [S3] https://openrouter.ai/api/v1/models/openai/gpt-4/endpoints — HTTP 200，官方文档示例型号的实际匿名响应。
- [S4] https://openrouter.ai/docs/api/api-reference/endpoints/list-all-endpoints-for-a-model.md — HTTP 200，官方 OpenAPI YAML；重点为 PublicEndpoint、PercentileStats、PerfLast30mByWorkload 以及各 workload schema。入口 https://openrouter.ai/docs/api/api-reference/endpoints/list-endpoints 本次 HTTP 200，页面明确给出前述 Markdown 地址。
- [S5] https://openrouter.ai/docs/api/api-reference/generations/get-request-&-usage-metadata-for-a-generation.md — HTTP 200，官方 generation API schema，链接来自 [S6]。
- [S6] https://openrouter.ai/docs/cookbook/administration/usage-accounting.md — HTTP 200，官方单请求 usage 口径。
- [S7] https://openrouter.ai/docs/client-sdks/typescript/sdks/datasets/README.md — HTTP 200，官方数据集 SDK 文档。
- [S8] https://openrouter.ai/docs/client-sdks/typescript/sdks/generations/README.md — HTTP 200，官方 generation SDK 入口。
- [S9] https://openrouter.ai/docs/llms.txt — HTTP 200，官方文档索引，供发现 [S6–S8]；未进行宽泛 URL 枚举。
- 按 SDK 文档原始相对链接访问 https://openrouter.ai/docs/client-sdks/typescript/models/generationresponse.mdx 和 https://openrouter.ai/docs/client-sdks/typescript/models/rankingsdailyresponse.mdx 均 HTTP 404；错误页不作为字段证据，未继续猜测变体 URL。生成字段改由 [S6] 已明确链接的 [S5] 核查。

## 实际响应的指标投影

以下只摘取返回的现存字段。未返回字段没有人为补成 null；明确为 null 的字段保留 null。各模型均只有一个返回端点。

```json
[
  {
    "source_url": "https://openrouter.ai/api/v1/models/recursive/flint-v1.0:free/endpoints",
    "retrieved_at": "2026-11-17T17:42:09.847Z",
    "data": {
      "id": "recursive/flint-v1.0:free",
      "endpoints": [
        {
          "name": "Recursive | recursive/flint-v1.0-20261021:free",
          "model_id": "recursive/flint-v1.0:free",
          "context_length": 262144,
          "provider_name": "Recursive",
          "tag": "recursive/bf16",
          "max_completion_tokens": 235929,
          "max_prompt_tokens": null,
          "status": 0,
          "uptime_last_30m": 100,
          "uptime_last_5m": 100,
          "uptime_last_1d": 100,
          "supports_implicit_caching": false,
          "latency_last_30m": null,
          "throughput_last_30m": null
        }
      ]
    }
  },
  {
    "source_url": "https://openrouter.ai/api/v1/models/cohere/north-mini-code:free/endpoints",
    "retrieved_at": "2026-11-17T17:42:10.155Z",
    "data": {
      "id": "cohere/north-mini-code:free",
      "endpoints": [
        {
          "name": "Cohere | cohere/north-mini-code-20260617:free",
          "model_id": "cohere/north-mini-code:free",
          "context_length": 256000,
          "provider_name": "Cohere",
          "tag": "cohere",
          "max_completion_tokens": 64000,
          "max_prompt_tokens": null,
          "status": 0,
          "uptime_last_30m": 100,
          "uptime_last_5m": 100,
          "uptime_last_1d": 100,
          "supports_implicit_caching": false,
          "latency_last_30m": null,
          "throughput_last_30m": null
        }
      ]
    }
  },
  {
    "source_url": "https://openrouter.ai/api/v1/models/openai/gpt-4/endpoints",
    "retrieved_at": "2026-11-17T17:42:45.233Z",
    "data": {
      "id": "openai/gpt-4",
      "endpoints": [
        {
          "name": "Azure | openai/gpt-4",
          "model_id": "openai/gpt-4",
          "context_length": 8191,
          "provider_name": "Azure",
          "tag": "azure",
          "max_completion_tokens": 4096,
          "max_prompt_tokens": null,
          "status": 0,
          "uptime_last_30m": 100,
          "uptime_last_5m": 100,
          "uptime_last_1d": 100,
          "supports_implicit_caching": false,
          "latency_last_30m": null,
          "throughput_last_30m": null
        }
      ]
    }
  }
]
```

## 验证与进程

本轮验证为 3 个匿名模型端点的实际响应、官方 schema 和文档交叉核查；没有运行应用测试、构建、开发服务器或浏览器。所有网络请求已 awaited 完成，未启动后台任务或轮询进程；本次研究产生的测试/服务遗留进程为零。报告写入后另执行仅针对本文件的 diff 空白检查和内容回读。

## 主线集成结果

核实来源后，主线复用现有认证端点读取，只调整消费端一致性，没有读取私人请求记录或新增收费请求。详情/对比图表统一首字P50、速度P50及24h可用率参考，空历史图可显示已有本站汇总/历史摘要而不虚构曲线，未获取的缓存命中/文本完整生成时长不补数值。当前变更本地验收通过，未发布。
