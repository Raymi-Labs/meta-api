# Agent Note: Pulse 已绑定用户的昨日日志汇总

Status: implemented

## Problem
Pulse 需要按绑定身份查询昨天的消费，不应重新接入整个日志库。

## Decision
复用内部签名与限流；新增独立只读角色，仅返回昨天已记录的消费汇总。绑定事实源在 Answer，由受信发送端校验。

## Alternatives considered
本人统计接口需要用户凭据；发奖密钥不应授予统计用途。逐条返回日志和直读库都会过度披露。

## Verification
SQLite 覆盖起止秒、退款排除、跨账号隔离、空结果；控制器拒绝覆盖 ID 和日志关闭；中间件覆盖重放、签名角色及重复密钥。

## Consequences
不恢复消费权益；日志留存与账单不同。

与 [退役消费贡献](../simplification/2026-11-08-retire-pulse-consumption-api.md) 和 [移除板块](../simplification/2026-11-08-remove-pulse-panels.md) 部分重叠：两项决定继续生效，仅提供日志读数。

后续[自动派生只读凭据](2026-11-20-pulse-derived-usage-key.md)替代独立密钥必填决定。
