# Agent Note: Responses 空输出取消共享冷却并保留失败诊断

Status: implemented

## Problem

Responses 完成事件没有识别到有效输出时，网关把本次请求记为失败，同时触发渠道待禁用和按上游地址共享的冷却。单个请求的输出异常不能证明共用地址的所有模型或渠道不可用，这会扩大影响范围。当前失败日志没有完成事件的结构摘要，无法区分上游空输出、异常结构和解析遗漏。

## Existing capabilities and notes audit

复用 `relay/channel/openai/relay_responses.go` 的空输出检测、失败终态、禁止重试与现有退款路径；复用 `relay/common/relay_info.go` 的完成摘要计数和 `service/request_failure_log.go` 的管理员诊断。普通用户日志由既有 `model/log.go` 去除 admin_info，不增加权限或数据库结构。

[上游能力切换](../../implemented/bug-fix/2026-10-12-upstream-capability-failover.md)及[K3 流完整性](../../implemented/bug-fix/2026-09-29-kimi-request-stream-integrity.md)部分重叠，保留它们的已输出不重放和失败退款边界。此前本地用户局部隔离提案未实施，本次选定简化路线，不引入用户计数、半开探测或自动切换。其他价格、生图与 Kimi 用量笔记不属于本次改动。

## Decision

- 只移除完成但空输出分支的共享冷却调用；保留合成 response.failed、skip-retry 和失败结算。缺少完成事件、鉴权、配额与服务整体故障的既有保护保持原语义。
- 在空输出失败时保存本次完成事件的结构诊断：完成对象是否存在、状态、输出数量和已知类型、正文长度、工具数量、JSON 解码失败次数及响应是否已提交。复用完成摘要的计数，不保存正文、工具参数、密钥或原始未知字段；状态和类型使用固定枚举，类型列表最多 16 项，避免供应商字段携带任意文本。
- 将诊断放入失败日志的 admin_info，普通用户得到明确的空输出原因而不会得到成功终态。只处理本次完整流，不依赖上次请求摘要，不改变正常流和非流式结果。
- 部署后让原先的短期共享冷却自然到期；不清空 Redis，也不自动恢复原有禁用渠道。部署和真实上游验收单独记录。

## Alternatives considered

- 关闭空输出检测最简单，但没有结果的请求会被误记为成功，故保留失败检测。
- 按用户、渠道、模型、接口计数并冷却能减少重复失败，但增加状态、阈值与恢复机制，当前缺少证明必须冷却的证据，暂不实施。
- 自动尝试备用渠道能改善部分请求，但完成事件不能证明服务端工具未执行，重放可能增加费用或副作用，当前保持禁止重试。
- 只修改默认恢复测试模型能减少错误放行，但仍会把单请求故障扩大到共享地址，故直接取消此类错误的共享冷却。

## Verification

- 使用 Go 1.26.0、GOMAXPROCS=2 执行 `go test -p 2 -timeout 180s ./service ./controller ./relay/... ./middleware ./model ./types` 通过；`go build -p 2 ./...` 通过。
- 空输出定向回归覆盖无完成对象、空数组、空消息及上游声明用量，均明确失败且不调用冷却；缺少完成事件的既有冷却回归保持通过。文本和仅工具输出正常完成。
- 完整 Relay / SQLite 回归覆盖钱包、订阅和独立令牌，验证真实预扣、单次上游尝试、预扣全额恢复、没有消费日志、当前及同地址渠道保持可用。异常 JSON 次数贯通到真实失败日志，管理员可见结构摘要而普通用户不可见。
- 诊断回归验证来源对象后续修改不污染快照、缺失对象不继承旧摘要、任意状态/类型字符串降为 unknown、类型最多 16 项，正文及敏感样例不进入新增诊断。
- 定向 `go test -race -parallel 1 -p 2 -timeout 120s ./service ./relay/channel/openai ./controller -run 'TestEmptyResponses|TestOaiResponsesStreamHandler_(FailsWhenCompletedHasNoOutput|EmptyCompletionShapesDoNotCooldown|ToolOnlyCompletionRemainsSuccessful|FailsWhenMissingCompletedWithoutOutputAfterEOF)|TestRelayResponsesInterruptedOutputRefundsAllFundingSources' -count=1` 通过。初次并行竞态检查发现既有测试辅助函数并发改写 Gin 模式和全局 StreamingTimeout；新增用例不采用 t.Parallel，旧辅助函数相关用例串行调度，处理器内部 goroutine 仍由 race 检测。
- Bun 1.3.14 的 `make verify-notes` 和 `git diff --check` 通过。不运行真实供应商调用或付费测试；没有 MySQL/PostgreSQL 部署实测，生产部署与真实上游空输出根因仍需另外验收。验收同时检查本轮测试进程全部退出。

## Consequences

[请求保活与完成交接](2026-10-24-stream-delivery-lifecycle.md)补充有效 completed 成功写入/flush 后及时终止，保留本篇 empty-output 的失败、退款与不触发共享冷却边界。

单请求空输出不再触发当前渠道及同地址渠道的冷却，减少连带不可用；明确失败与管理员结构诊断保留。代价是新增少量有界日志，真正持续空输出的上游仍可能被选中，需要依据诊断人工处理；没有自动切换和性能保证。保留原输出识别，不把本次上线当作已修复未知解析遗漏的证据。没有数据库迁移或生产参数修改，原有冷却状态不会被主动删除。只有不同用户独立请求持续异常等证据证明需要自动隔离时，才重新评估局部计数方案。
