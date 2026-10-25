# Agent Note: DeepSeek 明确拒绝后的纯保活补救

Status: implemented

## Problem

生产历史思考缺失的 100 次失败均为 Messages 流式请求，先经第三方 Chat 失败再到官方 Messages 明确拒绝，已有单次补救未留下 adjustment。现有补救以 Writer.Written 判定不可重发，而请求级 Claude ping 会在等待期间提交 HTTP 200。原测试关闭 ping，未覆盖此组合；不能仅凭日志推断所有请求都被同一门槛拦截。

## Decision

完整 Relay 与本地 HTTP 上游已经复现：第一次 ping flush 后才返回精确 400，原代码仅发一次上游请求并失败。仅扩展既有 DeepSeek 官方 Messages 精确 400 补救：保活 writer 保存可验证的纯心跳交付证据，跨其自身停止/重建保留；正常写入、显式状态提交、短写、写/flush 失败、未知 writer 或取消均不能获得例外。检查会停止并等待 pinger，避免检查时仍有并发写。通用重试以及其他供应商的已输出保护保持原行为。

补救开始前已经保活时，管理员 attempts 留下 after_keepalive 标记。补救请求本身也可能等到心跳提交响应头，因此返回后先停止并等待 pinger，再检查能否写 X-Request-Adjusted；已提交时改记 adjustment_header_not_delivered，不能声称响应头已送达。成功仍只结算最终用量，补救失败退款；不补造历史、不更改映射、每请求最多一次。

## Existing capability review

[历史思考兼容](../../implemented/bug-fix/2026-10-23-deepseek-messages-thinking-history.md)部分重叠，本决定仅替代其“任何已写出都禁止”的纯保活子集，官方地址/精确错误/历史保真/计费约束保留。[保活生命周期](../../implemented/bug-fix/2026-10-24-stream-delivery-lifecycle.md)部分重叠，继续拥有写入交接与协程清理；本篇增加保活来源证据，不把心跳计为首字或成功。[上游能力切换](../../implemented/bug-fix/2026-10-12-upstream-capability-failover.md)、[K3 完整性](../../implemented/bug-fix/2026-09-29-kimi-request-stream-integrity.md)、[空输出](../../implemented/bug-fix/2026-10-13-responses-empty-output-no-shared-cooldown.md)仅相邻，通用重放边界不变；其他命中为思考字段保真、健康归因或 schema 排序，无需替换。

## Alternatives considered

- 维持任何已写出禁止最简单，但已有官方明确拒绝的补救会被本站保活阻断，正常长等待请求收益落空。
- 移除 Writer.Written 检查能恢复补救，却允许正文、工具输出、短写后重放，因此只接受本站已验证纯 ping 的正向证据。
- 全局关闭保活能保留原判定，却重新暴露长准备和切换期间的断连问题。
- 心跳后补写诊断响应头不改变网络上已经提交的头，不能作为交付证据；保留日志可观察性并明确响应头限制。

## Verification

- 修复前完整 Relay 保活场景失败，期望两次上游请求却只有一次；修复后同一场景成功且原始历史不变，仅 thinking.type 改为 disabled。
- 完整 Relay/SQLite 新增保活后补救成功、补救再次拒绝、第三方 Chat 502 → 官方 Messages 400 → 同渠道补救成功三条路径。上游为本地 HTTP 服务，明确等待客户端 writer 成功 flush 标准 ping 后才返回失败，避免依赖固定延迟。跨渠道路径保留三次上游 request ID、两条 attempts，仍只有一条最终消费日志与 15 用量扣费；最终拒绝只有一条错误日志且预扣全退。
- 另一条本地 HTTP 路径立即返回首次 400，第二次补救等待标准 ping 成功 flush 才返回成功；验证 after_keepalive 不误称补救前已经保活、adjustment_header_not_delivered 留痕、客户端未收到调整头，仍只有一条消费日志和 15 用量扣费。
- helper 证据场景覆盖未提交、成功 ping、wrapper 重建、正文/工具/空写/显式状态/flush、绕过 wrapper 的写入、ping 写错/短写/flush 错、取消、未知 writer、其他协议及协议切换、失败零写后换 wrapper 和重新包装。两个 ping helper 均检查短写，短写不再继续 flush。
- DeepSeek 与本轮 Claude 完整 Relay 矩阵各在一个独立子进程运行，环境哨兵防递归并复用当前测试二进制保留 race 检测。真实路由、计费、日志断言不变，子进程错误完整返回。原因是完整 Relay 会懒启动进程级健康采集器，原数据库 fixture 仅还原 model.DB，采集器在后续无数据库测试或 race 退出等待期间定时 flush 会崩溃；每个子进程只建一次数据库，指针与连接保持至进程退出，场景间只清业务 fixture，健康表供真实采集器正常运行。进程隔离将生命周期限定到本矩阵，不修改业务采集器。
- `GOMAXPROCS=2 go test -race -p 1 -parallel 1 -timeout 120s ./relay/helper ./relay ./controller -run 'TestStreamKeepAlive|TestPingDataRejectsShortWrite|TestDeepSeekThinkingHeal|TestRelayDeepSeekThinkingHistoryCompatibility|TestRelayRequestKeepAlive' -count=1` 通过；原 DeepSeek 拒绝边界、未保活 JSON/SSE 的单次计费、透传与取消保护、准备期和跨 HTTP 保活回归通过。

## Consequences

仅新增一个官方明确拒绝后的纯保活例外，不改变通用渠道重试或其他供应商补救门槛。纯保活仍不能证明模型输出、首字或成功；上下文取消和任何写失败均不能放行。状态跨 wrapper 自身重建保留；请求级登记的 pinger 在读取证据前先退出，失败负证据保留到请求结束，外部 wrapper 替换及再次包装也不能清除。未知 wrapper 不能继承已有正证据，也不能从普通已写出的 writer 推断心跳来源。

思考被关闭仍是已有定向降级的语义变化。心跳已经提交响应头时，管理员 attempts 的 adjustment_header_not_delivered 和 adjustment 留痕，不能承诺客户端收到 X-Request-Adjusted；after_keepalive 只表示补救开始前已保活，未提交时保留原响应头。真实供应商、代理链路和客户端效果需独立验收，本次没有数据库迁移、真实付费调用或生产变更。
