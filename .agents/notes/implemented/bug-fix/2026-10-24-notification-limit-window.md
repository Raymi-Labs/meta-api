# Agent Note: 通知固定窗口与并发频控

Status: implemented

## Problem

现有通知计数键包含自然小时，跨整点会提前恢复额度，超过一小时的窗口无法按配置生效。Redis 的读取、检查、递增相互独立，内存的 sync.Map Load/Store 也不能保证检查与递增的原子性；并发请求可超过通知上限。零、负值或溢出的分钟数还会产生无效过期时间。

## Decision

复用 CheckNotificationLimit，按用户和通知类型建立从首次通知开始的固定窗口；Redis 使用 SETNX 和 INCR 的单事务，内存以互斥锁保护窗口重置、计数与清理。getDuration 在实际使用处统一校验分钟数，非正值或 time.Duration 溢出均回退既有 10 分钟；非正通知次数禁止发送，放行次数最多达到上限。不改变发送前占用额度的现有语义，不接管发送结果、外发或持久化通知。

## Existing notes audit

[额度受阻通知](../../proposed/feature/2026-10-24-quota-blocked-notifications.md)部分重叠但仍持有新通知触发、独立发送预算与审计决定，本修复只处理已有频控函数；[DeepSeek 原生 Responses](../../implemented/bug-fix/2026-10-18-deepseek-native-responses.md)及检索命中的 Kimi 转换笔记与本窗口语义无关。

## Alternatives considered

- 保留小时后缀可以按自然小时查看计数，但无法表达从首次触发起的任意分钟窗口，且整点前后会额外放行，因此移除。
- 引入滑动窗口可以均匀控制每个时间区间的发送量，但需要额外时间记录且会改变既有窗口语义；本修复保留固定窗口。
- 仅修 Redis 能覆盖共享实例，但无 Redis 部署仍会并发超限，因此同批保护内存路径和清理。

## Verification

`GOMAXPROCS=2 go test -race -timeout 180s service/notify-limit.go service/notify_limit_test.go -count=1` 通过。覆盖 120 分钟窗口跨整点、到期、拒绝请求不延长 TTL、64 并发只放行 3 次、用户/通知类型隔离、内存清理与重置、非正次数、分钟数溢出以及 Redis 错误不放行。包级集成检查随本批修复统一执行，不发送测试通知。

## Consequences

固定窗口保持配置时长并消除并发超发；内存路径增加一个短临界区以保护计数和清理。旧小时键保留至原 TTL 过期，新版本首次使用新键会开始新窗口；无 Redis 模式仍是单进程频控，不能作为跨实例发信预算。发送失败占用现有频控额度的问题由通知交付流程另行处理。
