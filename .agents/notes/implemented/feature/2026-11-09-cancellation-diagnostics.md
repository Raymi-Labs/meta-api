# Agent Note: 客户端断开的阶段与耗时诊断

Status: implemented

## Problem

用户压测失败统一显示客户端取消，无法从单条管理员日志区分等待上游与接收数据时断开。现有 latency_ms 是处理器结束时间，首字计时在不同适配器中不等同于 HTTP 正文到达。

## Decision

复用 RequestFailureLog 和管理员 admin_info，在请求上下文取消时冻结观察时间及当前 HTTP 尝试阶段。共用 HTTP 出站记录请求开始、响应头、第一次正文读取；不缓存正文。管理员展开详情显示断开耗时、上游正文是否开始与阶段，未覆盖的 SDK/WebSocket/历史日志保持未知。原 client_canceled 归因及健康/重试/计费规则不变；文案用连接中断，不凭时长断言主动停止或客户端超时。

## Existing capability review

使用日志 → /api/log 与 /api/log/self → RecordFinalRequestFailure → logs.other；用户接口删除 admin_info。首字时间无法覆盖非流式，也可能跨重试保留，因此不直接复用。检索已有[并发诊断](../../implemented/bug-fix/2026-10-25-upstream-concurrency-diagnostics.md)与[健康归因](../../implemented/bug-fix/2026-11-07-route-health-counts-unknown.md)，属相邻职责，均保持原决定；本篇新增诊断契约，无重复提案。

## Alternatives considered

- 用 45 秒阈值认定超时：易懂但没有客户端配置证据，短超时和长时间手动停止都会被误判。
- 直接使用 RelayInfo.FirstResponseTime：改动小，但不同协议语义不统一，非流式和多次尝试可能得出错误结论。

## Verification

定向 Go 单测及 -race 覆盖未响应头、只有响应头、已读取正文、未知覆盖、重试清零、取消后迟到数据不改变快照、日志权限隔离；管理员可直接看到诊断。无正文或密钥落盘。

## Consequences

取消回调记录的是服务端观察时刻，有调度误差；收到上游心跳也算正文，不能证明有效输出或客户端已收到。该信号不能单独证明断开根因。

相关 service/middleware/controller/relay/model 包测试及 `go vet` 通过；前端失败日志单测、语言资源检查及 Vite 生产构建通过。合成数据接入真实 useLogsData 与 Descriptions 的浏览器验收确认管理员阶段/耗时/三态值可见，普通用户隐藏；无真实生产调用。全量 `i18n:lint` 被现存其他组件的硬编码字符串拦截，本次文件不在报告中。没有配置 MySQL/PostgreSQL 测试 DSN，无跨库实机验证；本次不新增 SQL 或迁移。
