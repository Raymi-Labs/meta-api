# Agent Note: 明确上游并发拒绝的失败诊断

Status: implemented

## Problem

生产 Kimi 上游用 HTTP 403 返回 `You've reached your concurrent request limit`，现有最终失败诊断仅按 403 归为鉴权失败；账号池包裹后的 503 及其他上游 `gateway_concurrency_limit` 也缺少准确提示。管理员容易误查密钥，用户看不到降低并发的建议。

## Decision

只在最终失败诊断中，对确认来自上游且状态为 403/429/502/503 的错误，识别已观察到的明确并发拒绝句子或 `gateway_concurrency_limit` 错误码，归入现有 rate_limit 分类。保留原始状态、错误码、管理员 attempts 及 Retry-After 证据，不扩大重试、冷却、停用或额度语义。复用现有多语言原因及建议，无新增接口、配置或数据库迁移。

## Existing capability review

使用日志 → 原日志接口 → RecordFinalRequestFailure → describeRequestFailure → 日志表。普通用户与管理员继续沿用现有脱敏边界；判断只改变诊断文字，不改实际 HTTP 返回。活跃笔记中的[请求失败诊断](../../implemented/bug-fix/2026-10-17-relay-failure-diagnostics.md)负责首次请求体错误与内存限流；[分组限流](../../implemented/bug-fix/2026-10-17-group-rate-limit-scope.md)负责本地限流，两者与本次部分相邻，保持原决定；未发现相同上游并发分类归属。

## Alternatives considered

- 将所有 403 改成 429 能简化展示，但会把真正的权限或鉴权失败当成临时容量问题，也改变客户端重试行为，因此保留协议返回并按明确错误证据分类。
- 扩展共用 IsUpstreamRateLimitError 可复用现有识别，但会同时改变渠道冷却及切换行为，超出本批诊断修复范围，选择独立的诊断判断。

## Verification

- 定向测试覆盖真实 Kimi 403、包裹该原因的账号池 503、明确并发错误码以及原生 429。
- 401、普通 403、仅提及并发配额的权限错误及本地拒绝保持原分类。
- 最终日志保留原始 HTTP/错误码、零扣费与单条失败；不制造 Retry-After，也不改变重试判断。

`GOMAXPROCS=2 go test -p 2 ./service -run 'TestUpstreamConcurrencyDiagnostic|TestFinalFailureConcurrency' -count=1` 以及包含既有流式失败日志和客户端取消的定向 `-race -parallel 1` 检查通过。使用本地 SQLite 日志库，无真实上游调用或通知。

## Consequences

依赖明确上游措辞；未知格式保持原分类，后续必须取得真实证据再扩充。诊断改善本身不增加供应商并发容量。
