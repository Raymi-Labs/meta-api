# Agent Note: 路由逐次尝试遥测与影子决策

Status: implemented

## Problem

[托管路由提案](../../rejected/architecture/2026-11-05-managed-token-routing-and-pricing.md)要求先测量、后改调度。现有分组健康按组/模型/小时记录，同组重试折叠为最终结果，没有渠道维度、短窗口和分位数，不能回答“哪条线路现在可靠、智能路由会不会选得更好”。没有这些数据，任何健康或成本选路都无法验证。

## Decision

新增 `pkg/route_insight`，只观测、不改变调度和计费：

- **逐次尝试**：`controller/relay.go` 重试循环在每次上游尝试后记录渠道/实际组/模型/结果，结果区分 success、service_failure、rate_limited（429）、capacity_rejected（VM 执行前拒绝）、unknown、user_error、client_canceled、local_delivery（本地写回失败）。重试成功不会抹掉前一次失败。成功尝试记录本次首字与完成耗时到固定区间直方图（`LatencyBoundsMs`），P50/P95 取所在区间上界，属保守估计。
- **存储**：`route_attempt_metrics` 按分钟、`route_shadow_metrics` 按小时；每进程 UUID 行幂等覆盖，查询汇总各副本，与 group health 同一模式。保留 3 天 / 30 天。
- **健康快照**：后台每 15 秒从数据库读最近 15 分钟（含所有实例），按渠道+模型生成 5 分钟/15 分钟窗口。只有 success/service_failure/rate_limited 作为上游证据；unknown、用户、取消与本地失败不惩罚线路。证据少于 10 次为 unknown；不健康判定现为成功率低于后台阈值（默认 90%）或 5 分钟内上游失败 ≥50%（至少 5 次），见[有序健康选路](2026-11-07-ordered-health-routing.md)。快照超过 2 分钟未更新时一律视为 unknown。
- **影子决策**：每次尝试前，在请求已获准的候选内（令牌路由组、渠道白名单、重试排除和生产相同过滤器；指定渠道和无内存缓存时跳过，不扫数据库）模拟智能路由总开关的规则（按原顺序、组内跳过不健康、整组不健康换下一组），实际渠道落在推荐档位内即算一致，见[有序健康选路](2026-11-07-ordered-health-routing.md)；不越权、不跨模型。最初版本按「健康优先、等价取最低采购价」推荐，已改。
- **成本与故障域**：渠道 `setting` 新增可选 `route_purchase_ratio`（相对模型基础价的采购倍率，缺省为未知，不当 0）和 `route_fault_domain`。销售分组倍率不作为成本。渠道编辑页可填写，只在管理员「智能路由」页显示，不参与选路和计费。
- **逻辑请求**：每个请求另记最终是否交付、尝试次数与是否重试。
- **接口**：管理员 `GET /api/route_insight/channels?minutes=&model=`（渠道成功率、下界、P50/P95、当前状态、采购价、故障域）与 `GET /api/route_insight/shadow?hours=&model=`（同意/不同意次数、两者的失败率、跨组、避开不健康、更便宜、交付率、平均尝试数，以及本实例最近 200 条分歧样本）。
- 开关：环境变量 `ROUTE_INSIGHT_ENABLED`，默认开启；关闭后既不记录也不计算影子决策。

## Alternatives considered

- 扩展 group_health 加渠道列：复用最多，但其同组折叠和小时粒度是公开健康口径，改动会翻转已有统计决定；另建表保持旧口径不变。
- 每次请求同步查询数据库计算健康：数据最新，但把数据库放进主请求路径，违反提案的有界读取要求；采用后台快照。
- 只用本进程内存窗口：最快，但多实例各看一部分流量；改为经数据库汇总所有副本，代价是约 5–20 秒延迟。
- 存健康均值而不存直方图：省空间，但无法得到 P95；采用固定区间直方图（JSON 数组，三种数据库通用）。
- 用分组倍率当成本：已有数据可直接用，但它是销售价，会把高售价组误当高成本；改为显式采购倍率，未知不填零。

## Consequences

收益：可以按渠道看到真实逐次可靠性和尾延迟，并用“影子与生产分歧时的失败率”评估智能路由是否值得上线，全程不影响客户请求和账单。

代价与限制：每次尝试多一次候选枚举和健康查找（内存操作）；分钟表行数随渠道×模型增长，3 天保留需观察体积。不同意时的失败率只是生产流量上的相关性，不是反事实证明。分歧样本只在本实例内存。采购价和故障域需人工维护，未填时成本优化不生效。实时会话、异步任务、语音克隆等不在 `SupportsRequest` 范围内的请求不采集。

## Verification

`go test ./controller/ ./service/ ./model/ ./middleware/ ./pkg/... ./router/ ./dto/` 全部通过（Go 1.26.10，SQLite）。新增 `pkg/route_insight/route_insight_test.go` 与 `service/route_shadow_test.go` 覆盖：直方图分位与溢出、样本不足保持 unknown、20/20 下界、5 分钟突发、429/本地写失败/容量拒绝/流超时分类、低价不健康不胜出、等价健康选最便宜、缺价不当最便宜、无数据保持生产顺序、两副本汇总与幂等重刷、重试前失败可见、过期快照、影子同意/分歧计数，以及候选遵守分组、排除、白名单、指定渠道和无缓存。`make verify-notes` 因本机无 bun 未运行。新表另在 PostgreSQL 16 实机验证、并已在生产 MySQL 8.0.46 上完成迁移（见[撤回路由方案](../simplification/2026-11-07-withdraw-route-plans.md)），MySQL 未实机验证；未部署，未用真实流量校准阈值。
