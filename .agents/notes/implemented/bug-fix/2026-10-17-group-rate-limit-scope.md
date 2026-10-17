# Agent Note: 独立分组限流的计数范围

Status: implemented

## Problem

独立分组只覆盖限额，Redis 与内存计数仍共用用户键。高限额分组的请求会使低限额分组提前拒绝。上游 45fbb149f 修复了范围，但本地还有成功请求预占、续租和失败释放，不能覆盖替换。

## Decision

只在分组存在独立配置时使用用户与分组组合键；没有独立配置的分组继续共用用户全局额度。总请求与成功预占采用相同范围，保留成功判定和失败释放。

## Existing notes

[静态文件保护](2026-10-12-upstream-security-static-compat.md)处理其他入口，无重复归属；[请求完整性](2026-09-29-kimi-request-stream-integrity.md)中的失败边界保持不变。成功限流继续调用现有 `service.RequestSucceeded`，本次不改其判定。

## Alternatives considered

直接移植上游文件最省工作，但会丢失本地并发预占。所有分组都独立计数会让多个令牌绕过全局限额，因此仅独立配置的分组拆分。

## Verification

内存与 miniredis 下验证独立组互不占用、默认组共享、不同用户隔离、总数和成功限制、在途预占及失败释放。相关 race 检查和全仓 Go 测试通过。

## Consequences

独立分组升级时切换计数键，首个窗口重新计数；全局用户键保持兼容。

[失败诊断](2026-10-17-relay-failure-diagnostics.md)补齐内存 429 错误体与失败标记，不改变本篇计数、预占或释放决定。
