# Agent Note: 无输出响应头超时的一次渠道切换

Status: implemented

后续[回退与健康证据闭环](2026-11-20-routing-health-recovery.md)部分替代首次超时仅一次切换的预算限制；其余交付、权限与协议保护保持。

## Problem

生产只读诊断确认：请求等待上游响应头60秒后返回do_request_failed/504，已发送保活时传输状态为200。当前超时固定skipRetry，RetryTimes=1也不会切换；用户明确选择网关最多换一个渠道。

## Decision

共用HTTP入口为真实响应头等待超时增加内部标记，保留默认skipRetry。Controller仅对独立Chat/Messages请求的首次渠道尝试且重试开启提供一次例外；复用原权限/分组/排除规则，最多两个上游尝试，自动分组不能重置此上限。仅受控writer证明未输出或只有成功保活才切换。指定渠道、取消、写失败、正文输出、透传/渠道参数覆盖/上游会话/MCP/服务端工具不放开。保留HTTP200与流内错误的真实区分。

## Existing notes audit

[流式交付生命周期](2026-10-24-stream-delivery-lifecycle.md)和[DeepSeek保活调整](2026-10-25-deepseek-heartbeat-adjustment.md)部分重叠：复用停止并join及失败负证据，原DeepSeek入口继续限Claude。[上游能力拒绝切换](2026-10-12-upstream-capability-failover.md)和[渠道非流式等待覆盖](../feature/2026-10-05-channel-nonstream-header-timeout.md)部分重叠：保持原规则，本篇仅覆盖已识别响应头超时。Claude终态保护保持，流读取超时不重放。无完全吸收或过时提案。

## Alternatives considered

- 全局延长超时让慢渠道继续等待，但增加所有请求占用且不解决坏渠道，不采用。
- 所有504重试可能重放任务和有状态请求，不采用。
- 关闭心跳可以保留HTTP504，但降低长连接存活，保留保活和流内错误。

## Verification

真实本地HTTP完整Relay覆盖Chat转Claude与原生Messages、首次超时后成功、双超时、第二路500仍不尝试第三路、重试关闭、指定渠道与白名单；两种协议保活后切换成功且只结算一次，失败全额退回预留，管理员诊断保留首次504。门槛与writer测试覆盖正文输出、短写、写/flush失败、取消、未知wrapper、有状态请求和服务端工具拒绝。`go test ./...`、相关包测试及定向race通过；后端构建、Bun前端构建、发布策略和笔记校验通过。本机缺少默认Chromium，前端预渲染按既有脚本跳过，无UI改动。

## Consequences

超时不能证明上游没有执行原请求，供应商成本可能重复，用户已了解并选择一次切换。最多增加一次等待；保持60秒默认值、令牌权限和费用规则，不自动部署生产。
