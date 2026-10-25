# Agent Note: 请求保活、流式写入与完成交接

Status: implemented

## Problem

Claude 的保活只覆盖单次 HTTP 等待，准备和重试间隙没有心跳；取消 pinger 后没有等待其退出，已提交 SSE 后最终错误仍返回 JSON。Responses 完成事件写出后继续等上游关闭，随后取消可把已完成请求改成失败；事件写入错误没有传播。scanner 对慢写另起 goroutine 等待，清理五秒后可能仍有 worker 使用已返回的 Gin context。

## Decision

复用当前 Relay、协议 helper 和 StreamStatus。Claude 流式请求从准备阶段启用请求级保活，内层 HTTP 调用复用，在业务写入及 scanner 接管前停止并等待；保留标准 Claude ping、现有间隔上限与已输出不重放边界。已提交 SSE 的最终错误用对应协议 error 表达，失败仍沿用原退款。

响应 Header 返回可变 map，必须先停止并 join 保活，再允许准备路径设置 X-Request-Adjusted、Retry-After 等诊断头；pinger 自己使用原 writer，不能自等待。每轮渠道选择和 HTTP 等待前重新启用，每轮结束在健康 writer 读取状态前停止。访问响应头后至下一次启用，以及健康采样/重试策略处理期间可能无心跳；这是避开并发提交响应头和 Gin 状态竞争的显式交接边界，不提供无间断心跳保证。

scanner 只保留一个可关闭的上游读取 worker，业务事件与 ping 由调用 goroutine 串行写入；关闭上游后等待读取 worker 退出。读取与处理逐行握手，不能把下一次 Read 的取消抢到已读输出事件前面而丢失未计费用量诊断。SSE writer 使用累计字节写预算：三十秒基础预算加每秒八 KiB 的排队字节传输时间，使大事件有传输时间，同时约束停止读取的连接；空闲后重新计时。保留默认扫描上限、聚合模式禁止客户端 SSE/ping 和所有适配器的 bool callback 契约。本地原先没有上游的固定三十秒大帧超时问题，本轮适配实际存在的慢写清理生命周期，不宣称复现上游同一缺陷。

Responses 只在完整、有效且未失败的 completed 事件成功写出并 flush 后确认交付并停止；真实写错、短写、empty-output、incomplete 和缺终态不能借取消竞态变为成功。Flush 只调用外层可观察的 FlushError 或 Flush 一次，不绕过包装和二次 flush；flush 返回后再检查 context，隐藏 flush 失败导致的取消不能误确认交付。已确认交付后清理上游所发生的取消不撤销成功，协议之外的裸 EOF 不构成成功。请求失败标志和 HTTP >=400 始终优先于显式完成标志。

可确认的本地下游写错使用 downstream_write_failed/502/skip-retry，controller 在健康采样时记为未归因失败并保留分母，不惩罚上游渠道、不切换重放且沿用失败退款。该分类要求本地 error 类型且没有 upstream status，供应商同名 error 不适用；可确认 context.Canceled 继续走客户端断开分类。

DeepSeek 官方 Messages 精确历史思考拒绝的纯保活补救由[独立例外](2026-10-25-deepseek-heartbeat-adjustment.md)持有，复用本篇 writer 的交接并增加来源证据；不放宽通用重放规则。

## Existing notes

[K3 请求完整性](../../implemented/bug-fix/2026-09-29-kimi-request-stream-integrity.md)、[空输出保护](../../implemented/bug-fix/2026-10-13-responses-empty-output-no-shared-cooldown.md)部分重叠，保留失败退款、工具身份、禁重放及空输出不触发共享冷却；本篇补充完成交接和 worker 生命周期。[分组健康](../../implemented/bug-fix/2026-10-22-group-health-outcome-accounting.md)继续持有健康分类，仅共用实际写入证据，不更改分母或数据库。通知额度、任务计费与其他活跃笔记不持有本决定，无需替换。

## Alternatives considered

- 直接移植上游多 worker/StreamResult 接口可复用整套实现，但需要改写本地所有适配器，扩大终态语义风险；保留 bool callback 并简化写入所有权。
- 仅完成事件后返回可消除等待，但写错误仍会丢失，无法证明完成真的写出，因此同时贯通写入结果。
- 固定每次写三十秒容易验证，却会切断持续读取的大图片客户端；采用累计字节预算。继续五秒后放弃等待可以缩短返回，但会留下访问 Gin context 的 worker，不能保留。

## Verification

- `go test -p 2 -parallel 1 -timeout 120s ./relay/helper ./relay/channel/openai ./relay/channel ./relay/common ./controller ./service -run 'TestStream|TestResponseChunkData|TestResponses|TestOaiResponsesStreamHandler|TestRelayRequestKeepAlive|TestRelayResponsesInterruptedOutputRefundsAllFundingSources|TestRelayGroupHealthRecordsCancellationStagesAndDelivery|TestRequestOutcome' -count=1` 通过。
- 同一范围的新增流式、准备期、完成交付、退款、请求结果用例执行 `go test -race -p 2 -parallel 1 -timeout 120s ./relay/helper ./relay/channel/openai ./controller ./service -run 'TestResponseChunkData|TestStreamWriteBudget|TestStreamKeepAlive|TestStreamScannerSlowWrite|TestResponsesDelivered|TestResponsesInvalid|TestResponsesRealHTTP|TestRelayRequestKeepAlive|TestRelayResponsesInterruptedOutputRefundsAllFundingSources|TestRequestOutcome' -count=1` 通过。串行调度测试避免既有全局设置相互改写，实际处理器和心跳并发仍受 race 检查。
- 真实 net/http 覆盖完成后上游不关闭、partial 后客户端关闭、完成小帧已缓冲且进入 Flush 后客户端 TCP RST；隐藏错误路径只 flush 一次且不确认成功。准备阶段真实图片下载阻塞时 Claude 标准 ping 可到客户端，随后失败发协议内 error 而非 JSON；嵌套 HTTP 两次尝试及诊断头后重启保活通过。
- Header 首次提交阻塞时访问诊断头必须等待心跳退出，重复 Start/Stop、取消和业务写交接通过。字节预算覆盖二 MiB 大帧、排队累计和空闲重置，deadline 慢写返回后上游 reader 已退出。
- 完整 Relay/SQLite 回归覆盖钱包、订阅和独立令牌：真实预扣；write/flush/取消、empty-output/incomplete 均退款无消费记录；已交付 completed 及之后取消保持消费和健康成功。显式下游写错记未归因，未取消的写错不伪报客户端断开，不切换渠道、不冷却上游。没有数据库或 SQL 变更。

## Consequences

不再因完成后等待上游 EOF 而把成功误判为取消，也不再丢弃可观察的写错和让 worker 在 Gin context 回收后继续写。代价是逐行读取/处理握手、写预算状态及心跳交接增加少量同步；主动访问响应头会暂停保活，长准备若新增响应头访问须检查恢复位置。

写入成功只能证明服务端可观察的交付，不代表客户端实际消费。Gin 隐藏且没有产生 context 取消的底层 flush-only 错误仍不可观察；flush 时取消采取保守失败，即使客户端可能已得到数据。不得通过绕过包装或二次 flush 猜测。字节预算依赖 net/http deadline 支持，第三方包装若不暴露 Unwrap 不会获得底层 deadline；自定义 Body.Close 必须能终止阻塞 Read。本轮不新增全局总超时、数据库或生产配置，不进行真实供应商、MySQL/PostgreSQL 部署或生产性能验收。
