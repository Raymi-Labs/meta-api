# Agent Note: 请求体首次失败与内存限流诊断

Status: implemented

## Problem

请求体读取失败后已关闭原 reader，后续读取把真正原因覆盖为 closed Body；无 Redis 限流返回裸 429，缺乏可追踪错误体与失败标记。

## Decision

适配 ef60ef27f/977450a3e，在请求上下文保留首次读取失败并重用。内存限流复用现有 abortWithOpenAiMessage，使用与 Redis 相同的本地错误消息和请求 ID/失败日志链。保留所有分路由字节限额、分组 subject、成功预占/失败释放，不迁入上游插件入口。

## Alternatives considered

- 每次重读无需状态字段，但已消费或关闭的请求体不能可靠重放。
- 新建错误格式或单独日志可补文字，但绕开现有客户端兼容和最终失败记账；复用原错误出口。

## Testing

common/middleware 全包与定向竞态回归、构建通过。读取错误/413 在多次读取后原因稳定且不重新读取，正常请求仍可重复使用；总数和成功数两种内存 429 有消息、请求 ID、失败标记。既有限流隔离、失败释放、请求体限制回归通过。

## Consequences

错误缓存限同一请求；不增加重试、不改变成功口径及费用。与 [Kimi 请求体上限](../../implemented/bug-fix/2026-09-29-kimi-retest-scope-correction.md)和[分组限流](../../implemented/bug-fix/2026-10-17-group-rate-limit-scope.md)部分相邻，保留其既有规则；本记录仅管理错误诊断，不取代原决定。

响应头与日志的本站 ID 契约、上游 ID 的独立诊断由 [HTTP 请求 ID 一致性](2026-10-20-response-request-id-integrity.md) 约束；本篇继续管理请求体和限流失败出口。
