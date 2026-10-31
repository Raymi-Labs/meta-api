# Agent Note: 日志轮转后的当前 writer 与文件释放

Status: implemented

## Problem

Gin access/recovery 中间件在启动时捕获 writer；本地 SetupLogger 换文件后，中间件继续写旧文件，旧 fd 也不关闭。业务日志只锁取 writer 快照，公共 SysLog 没有同步，因此不能仅加 Close：并发写可能拿着旧文件。生产启用文件日志，但未确认历史轮转事故；使用真实临时文件与隔离 HTTP 服务验证。

## Decision

参考上游 4193734a2，common 提供稳定的当前 writer 代理和读写锁，写入时持读锁直到 I/O 完成。公共日志、业务日志、Gin access 和 server-wide recovery 均使用代理。SetupLogger 串行打开文件，在写锁下替换两个 Gin writer 并关闭上一轮由自身打开的文件；不关闭 stdout/stderr 或外部设置的 writer。打开失败继续沿用现有 fatal 行为，文件名、百万条近似阈值和异步轮转资格不变。

把 main 内 recovery 原样移入 middleware.PanicRecover，以同一入口验证栈日志与既有 JSON 错误体；RelayPanicRecover 仍保持原来的协议职责。写错误/短写从代理透传，不添加后台队列或丢弃策略。

## Existing notes audit

[日志计数同步](../../implemented/bug-fix/2026-09-29-relay-log-counter-synchronization.md)部分被扩展：原子计数/调度资格继续有效；只锁快照且不关闭 fd 的决定被本篇替代，双方互链，旧理由保留。其他流式交付/计费笔记不改变。

## Alternatives considered

- 只让中间件跟随 writer 能修复旧文件写入，但继续泄漏 fd，不能解决公共日志读取和轮转的并发问题。
- 只在替换后 Close 最少代码，却可能关闭业务日志已经取得快照的文件，造成写入失败。
- 单一互斥锁覆盖所有日志可保证安全，但会串行化所有输出；采用读写锁，写入仍可并行，只在轮转关闭前等待已有写入完成。慢 writer 会延后轮转，是本方案的明确代价。
- 自动关闭任意旧 writer 无需单独跟踪文件，但可能关闭调用者持有的 stdout 或自定义 sink；仅管理本日志包创建的文件。

## Verification

- 修改前隔离 HTTP 服务在 writer 替换后仍把 GET /after 写入旧目标，回归失败；修复后关闭旧文件再发普通请求与 panic，访问日志及 Recovery 栈均写新文件，JSON 类型和错误文本保持。
- 真实文件连续 16 次轮转，256 条并发公共/业务日志各出现一次，旧 fd 全部关闭，当前文件可写；外部文件 writer 保持打开。测试使用独立临时目录，不读取或修改生产日志。
- common/logger/middleware 定向 race 通过，覆盖进行中的 Write 持锁、短写和错误返回、既有原子计数/单一异步轮转资格、日志脱敏和鉴权 query 不变。
- `go test ./...`、后端构建、前端与文档构建、笔记校验通过。本机无默认路径 Chromium，预渲染按原脚本跳过；没有 UI 改动，发布 CI 继续执行既有构建流程。

## Consequences

读锁贯穿 I/O，阻塞输出会延后轮转；不新增队列或超时改变现有日志交付。外部绕过代理、未持锁直接读写 Gin globals 仍不在并发保证内；文件名和日志内容不变，无保留策略、数据库或生产配置变更。
