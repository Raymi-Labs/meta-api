# Agent Note: 流式失败日志计数同步

Status: implemented

## Problem

同一流式取消请求的主处理和扫描协程会同时记录日志。日志轮转计数及工作状态使用无锁整数和布尔值，使失败退款用例的竞态检测失败。计数只需近似准确，但并发访问仍必须同步。

## Existing capabilities and impact

日志入口均在 `logger/logger.go`，计数仅用于触发每百万条的日志文件轮转，不参与数据库、用量或返佣。主程序调用 SetupLogger 初始化；运行时由日志入口提交异步轮转。活跃笔记中[复测纠偏](2026-09-29-kimi-retest-scope-correction.md)记录了此验证缺口，本笔记只处理该局部并发问题，不修改该笔记的请求参数决定。

## Decision

- 日志计数和轮转调度标记使用原子操作，一次只有一个日志入口获得调度资格；异步任务负责释放自己的资格。
- 对本日志包内 writer 的替换与快照读取使用同一读写锁，不将普通日志 I/O 放入全局锁，不改输出内容和轮转频率。
- 并发计数、计数触发阈值与取消退款用例在竞态检测下验证，不通过关闭日志掩盖竞态。

## Alternatives considered

- **忽略近似计数竞态**：代码改动最少，但不能让验收可信，也不能保证布尔调度状态不会重复派发。
- **所有日志写入一个全局互斥锁**：同步关系简单，但会让失败高峰的输出串行等待。本轮只同步状态与 writer 快照，实际写入沿用既有 writer 的行为。

## Consequences

[当前 writer 与文件释放](2026-10-31-current-log-writer.md)扩展本篇的 writer 生命周期：下面“不关闭 fd、只锁快照”的范围描述保留为当时取舍，现在由覆盖完整写入的读锁和轮转写锁替代；原子计数和异步调度资格仍按本篇执行。

原子计数仍允许轮转边界附近的近似统计；不提供严格每文件行数保证。不重构 Gin 中间件已捕获的 writer，不改变日志保留和旧文件描述符管理；外部代码直接修改 Gin writer 不属于本日志包的同步保证。

并发写日志无需全局串行等待文件 I/O；轮转资格由调度成功的任务持有并释放，直接初始化不会清除另一个任务的状态。代价是每条日志增加原子计数及短读锁开销，不声称吞吐或首字时延因此改善。

## Verification

- 新增 `TestLogCounterConcurrentRequestFailures` 在修复前复现未同步计数的数据竞争。
- 日志并发计数、单个异步轮转资格、直接初始化不释放资格均有断言。
- logger/common/middleware/dto/controller 的日志、请求体、工具、三协议HTTP、Chat/Messages/Responses失败退款定向 `go test -race -count=3` 连续三轮通过，包含先前报竞态的 wallet/client_canceled 用例；使用 Go 1.25.1，GOMAXPROCS=2。
