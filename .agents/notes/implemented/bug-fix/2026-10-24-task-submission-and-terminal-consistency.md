# Agent Note: 原生任务预扣、请求头与终态一致性

Status: implemented

## Problem

异步任务重试已经重新计算报价，但已有计费会话会跳过提交前差额预留。Gemini/Vertex 实时查询直接落终态，后台随后排除该任务，可能漏掉完成结算或失败退款。原生任务提交和轮询未统一应用已有渠道 Header Override。

## Decision

- 复用现有任务报价和 BillingSession，重试在发出请求前补足目标额度；钱包使用现有原子余额条件预留，订阅和 token-only 继续走现有 Reserve/回滚。额度拒绝属于本地错误，不重试或计入渠道故障。保持同步资金语义和既有 Pulse 边界，不引入上游插件快照体系。
- 实时查询和后台轮询共用已解析结果处理器：非终态到终态的 CAS 成功者执行结算或退款。复用第一次查询结果，不嵌套查询或重复申请容量槽；重复终态、失败落库和 CAS 竞争均不重复调账。
- 任务提交、各原生轮询和 Hailuo 文件查询在适配器默认头之后应用现有覆盖。后台只使用静态值、当前请求凭据和任务落库的用户身份，跳过原始客户端头占位符；Suno 批次按持久化用户和密钥分组，不混用身份或保存客户端头。
- 内容查询复用相同身份与保存密钥；仅现有带凭据的内容请求应用覆盖，并移除 Host 覆盖。Gemini 结果 URL 先原样提取，仅与配置地址 scheme/host 精确同源后才添加保存的 API key header/query；追加时转义密钥且不重排已有签名 query、不替换已有 key。Gemini 返回的外部下载 URL 与 Vertex 下载不新增渠道私有头或 query。带凭据的内容请求无论是否配置 Header Override 都阻止跨源重定向；同源重定向仍可用，保留现有重定向检查，不把该强制规则扩大到无覆盖的其他适配器轮询。
- 内容下载解析、transport 和 HTTP 状态失败日志只记录任务 ID、状态和错误类型，不输出可包含注入密钥或上游签名的 URL/完整 transport 错误。
- 终态 CAS 获胜后，只有实际资金和令牌调整成功改变的 quota 才单列保存；持久化失败明确记录人工对账需求。token-only 任务只调整令牌，不误增或扣减钱包。

## Existing notes audit

[通用视频定价与验收](../../proposed/feature/2026-10-21-video-pricing-and-testing.md)部分重叠于任务生命周期，保留其结构化定价和付费验收提案；[Vidu 参数保真](../../implemented/bug-fix/2026-10-17-vidu-billed-fields.md)负责出站参数，不改变；[认证身份头](../../implemented/feature/2026-10-21-authenticated-user-header.md)继续作为身份来源规则，本次补充后台由已持久化任务归属提供身份的入口。现有额度受阻提醒开发独立；共用 billing_session.go 仅单独增加任务首次预扣的原子余额门槛，不混入提醒实现。

## Alternatives considered

- 照搬上游任务插件代码可复用完整补丁，但本地原生适配器、计费和渠道容量契约不同，不引入新运行体系。
- 实时查询重新调用整个轮询器可以集中结算，但会二次请求并在容量为一时争抢自己持有的槽；抽取结果处理器直接消费已取得的证据。
- 提交后统一补差额复用原结算最简单，但已无法阻止未获资金保障的上游付费任务；应在报价确定后、提交前拒绝。
- 内容重定向时移除已知敏感头能兼容跨源 CDN，但还需正确清理自定义头、query 和自动 Referer，容易遗漏；选择仅拒绝带凭据内容请求的跨源跳转。上游直接返回的外部签名下载 URL 不新增凭据，保留正常下载。

## Testing

- `task_submission_billing_test.go` 覆盖重试较贵/较便宜、首次并发余额门槛、历史信任会话补扣、钱包/订阅/token-only 余额不足、token 失败回滚及累计退款。`task_submission_funding_test.go` 以真实提交入口验证资金拒绝的上游请求计数为零；控制器回归验证 LocalError 不因 429/5xx 被重试。
- `task_terminal_result_test.go` 覆盖成功/失败、重复终态、旧快照 CAS 竞争、落库失败、三种资金源以及调账失败保留原金额。`task_realtime_consistency_test.go` 用真实本地 Gemini HTTP 响应验证实时失败只发一次请求且复用上限为一的容量槽。
- `task_request_headers_test.go` 覆盖全部 11 个原生适配器的实际本地 HTTP 查询、提交身份防伪、保存密钥、后台无客户端头、非法配置及跨源重定向；Suno 回归验证按持久化用户和密钥分批。
- `video_proxy_gemini_security_test.go` 用双本地 HTTP 服务器验证同源保存密钥、已有签名 key 保真、直接/再次查询所得异源 URL 不增 key/身份头、同源跳转可用及无 Header Override 的跨源跳转不触达第二源；transport 错误即使嵌入完整凭据 URL，日志和下游错误也不泄漏。签名 query 保留原始顺序、转义和片段，无法可靠解析的同源 query 安全拒绝。
- `go test ./service ./relay ./relay/channel ./relay/channel/task/... ./controller -count=1` 通过。新增专项 `go test -race` 在 service、relay、relay/channel 和 controller 通过。不以本地模拟请求宣称真实 provider 或生产完成；无 schema 变更，新增金额保存使用跨数据库 GORM Update，未执行 MySQL/PostgreSQL 实例验收。

## Consequences

任务在提交前取得当前报价的资金保障；实时状态与后台轮询不会因入口不同漏掉正常终态调账，token-only 不改变钱包。代价是可信异步任务也需要真实预扣，Suno 为用户身份和保存密钥分批后可能增加请求数；渠道现有容量准入继续控制每批请求。

内容端不再把渠道密钥发送给上游返回的任意外部下载地址。依赖“先访问带凭据地址再跳转到其他源”的内容服务会明确失败，需要上游直接返回可独立访问的签名 URL 或后续单独定义经过审核的凭据清理策略；不以自动重试或全局放宽 redirect 恢复这种链路。

终态 CAS 与资金调整仍非数据库事务；本次守住 CAS 获胜方单次执行语义，不声称进程在 CAS 后崩溃可自动恢复。资金源成功而令牌调整失败、或金额写回失败仍须按明确日志人工对账；若需要崩溃恢复，应单独设计持久化结算账本/事务，不通过反复处理终态盲目重试。新的配置覆盖会真实改变任务请求头，非法配置会阻止请求，后台身份必须来自任务归属。现有受阻提醒脏改动的完整性由该开发任务负责，本次不混入或提交。
