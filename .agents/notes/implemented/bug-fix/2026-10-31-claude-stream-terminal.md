# Agent Note: Claude 流完成交付与中断失败

Status: implemented

## Problem

2026-10-31 只读核查第三方 Claude 最近24小时日志，两个渠道共14条上游 TCP reset，另有7条响应头超时。日志没有逐请求 effort，不能归因为 max 参数；公开版本 compat.134 的 ClaudeStreamHandler 与本地相同：流读取异常仍返回 nil error，Chat转换追加DONE，完整message_stop后仍等待EOF。这会掩盖中断并让不完整流进入消费结算。

## Decision

复用StreamScannerHandler、已交付终态、下游写错分类和controller协议内error/退款机制。HTTP Claude流只在message_stop（包括附带的最后usage）实际写出/刷新成功后确认完成并立即关闭上游。不把message_delta.stop_reason或裸DONE当成完整Messages终态。上游中断返回502或超时504，禁止重放；已观察usage仅作管理员未结算证据。下游取消明确标记未完成但保留既有已用量结算，避免客户端获得部分结果后主动断开触发免费；不把取消当成功、不发成功终态。Chat同样不伪造成功usage/DONE，controller的error后结束帧仍作为错误封装使用。

原生Messages只记录允许的客户端请求思考设置（requested，不宣称是覆盖后上游生效值）与流结束摘要，不记录请求正文、工具参数或任意上游内容。Chat来源不冒充原生effort证据。客户端错误信息不包含上游地址、原始读错误或凭据。完整流现有价格、工具调用、缓存usage语义保持；失败沿用钱包/订阅/独立令牌退款，不追溯历史余额。隔离工作树不包含主工作区视频修改，不改生产配置、超时、渠道路由或上游HTTP版本。

## Existing notes audit

- [流式交付](../../implemented/bug-fix/2026-10-24-stream-delivery-lifecycle.md)：扩展相同的终态交付与worker回收规则到Claude HTTP解析器，原Responses规则不变。
- [Claude缓存计费](../../implemented/bug-fix/2026-10-08-claude-cache-billing-consistency.md)：保留缓存守恒；本篇显式替代其未处理的一般HTTP断流仍结算路径，不扩大到AWS事件流或非流式。
- [渠道等待](../../implemented/feature/2026-10-05-channel-nonstream-header-timeout.md)：相邻配置，单纯延长等待不能修复TCP reset，不修改。

## Alternatives considered

- 调大总超时或强制HTTP/1.1能缓解特定链路，但当前reset不集中在固定时长，且未核实HTTP协商，不能据此全局改连接行为。
- 所有中断自动换渠道可尝试完成输出，但已有工具/思考/内容交付时会重放副作用，不采用。
- 保留部分结算并仅发送error可以少改账务，但会让调用者收到失败却承担不完整结果，且未知最终usage无法证明结算完整性；采用现有显式上游失败路径并说明成本影响。

## Verification

初始回归两项八个子场景复现旧版截断返回nil以及完成后继续读上游。修复后模拟EOF/reset/空流/超时/只有stop_reason/只有DONE均明确失败，不重放，不输出成功终态；思考和工具部分输出也不能重放。message_stop交付后即使上游不关闭或Close导致取消也保持成功。写/短写/flush失败不算完成，不归罪上游。真实loopback HTTP上游发送message_stop后故意不关连接，处理器立即关闭上游、上游handler退出。

完整controller 42个场景覆盖Chat和Messages、钱包/订阅/独立令牌的真实预扣退款、主动取消的原有部分结算、日志与健康统计，原生max/adaptive/预算原样转发；管理员诊断不进入普通用户日志。仅隔离SQLite，没有新增SQL/schema，也没有MySQL/PostgreSQL实机验收。

`go test -p 2 -parallel 1 -timeout 150s ./relay/channel/claude ./relay/helper ./relay/channel/aws ./relay ./controller ./service -count=1`通过。解析/写入/诊断定向race与controller本轮42场景独立race通过。本轮矩阵从每例重建并恢复全局DB改为同一隔离DB/全局配置、每例独立账号/令牌/订阅，避免后台collector与重复DB替换竞争。较早将既有72项缓存账务回归与矩阵放在同一race进程时，后台group_health在测试DB被恢复为nil后panic；其他旧夹具/后台worker生命周期限制仍未全局修复，不声称完整controller race套件通过。没有以生产数据库验证或付费模型调用替代该限制。

## Consequences

中断能被客户端和健康统计明确识别，完成事件交付后不再等上游EOF。失败退款会让站点承担上游可能已产生的成本；无message_stop的不合规供应商将显式失败，不能用伪造终态兼容。修复不能修复上游网络、保证超长任务不断或证明max可靠。线上部署、上游协作与真实客户端续接仍需独立验收。公开版本与日志核查不等同上游主机/反代配置审核；上述验证未写入生产，代码推送合并与服务器部署须分别确认。
