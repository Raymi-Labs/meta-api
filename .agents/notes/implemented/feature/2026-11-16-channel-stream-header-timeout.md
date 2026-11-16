# Agent Note: 渠道流式响应头等待覆盖

Status: implemented

## Problem

Gemini 原生视频理解的 SSE 请求在网关等待响应头约 60 秒时返回 504。客户端总截止为 420 秒，不会延长网关出站等待；现有 nonstream_response_header_timeout_sec 明确排除流式请求。需要为慢渠道提供可撤回的等待设置，不全局延长其他渠道，也不把客户端重试作为处理方案。

## Decision

复用渠道 setting JSON、管理员更新/编辑入口和有界 HTTP 客户端覆盖缓存，添加 stream_response_header_timeout_sec。0/缺省继承全局；1—600 秒整数覆盖该渠道流式 Chat Completions、Completions、Responses、Gemini 生成请求的响应头等待。Images API、Responses 图片工具、音频接口、任务提交/轮询与 WebSocket 不变；非流式字段保持原语义。

不按 Gemini 模型名或生产渠道 ID 硬编码默认值。管理员可对目标 Gemini 渠道设 180 秒，其余继续继承；180 是建议试验值而不是所有渠道的新默认。延长仅作用于响应头阶段，不修改请求体、正文读取超时、取消、客户端总截止、反代时限、重试、路由与计费。流式覆盖记录为独立 channel_stream_override，保留 channel_nonstream_override 的准确语义。UI 复用原高级设置输入控件，保留必要标签、0/600 边界和 tooltip。

## Existing capability and notes audit

- [非流式等待](../../implemented/feature/2026-10-05-channel-nonstream-header-timeout.md)部分重叠：保留原字段、旧行为与客户端缓存，新增流式字段；不把旧笔记改为相反决定。
- [视频提交等待](../../implemented/bug-fix/2026-10-30-video-channel-wait.md)部分重叠：异步视频提交继续采用非流式字段，Gemini 视频理解是流式生成，二者不混用。
- [响应头超时一次切换](../../implemented/bug-fix/2026-11-15-header-timeout-failover.md)部分重叠：保持来源标记、现有重试范围和次数；新选项不放宽安全切换门槛。

## Alternatives considered

- 直接把普通 relay 全局 60 改 180 可以即时生效，但让所有不相关渠道失败等待同步变长，不能按渠道撤回，因此不采用。
- 将旧非流式设置用于全部请求可以减少字段，但会默默改变已经配置旧字段的流式行为，违背现有兼容性约定，因此不采用。
- 固定识别 Gemini 型号并延长最容易配置，但模型别名、代理渠道和多协议使前缀推断不可靠，也缺少运营可调节入口，因此不采用。

## Verification

- 定向 Go 回归覆盖 dto、service、relay/channel、controller：字段范围/整数校验、旧字段独立、默认短时限失败、覆盖后收到完整原生 Gemini SSE、并发未配置渠道仍按原时限失败，以及取消、总时限、代理、缓存有界/重载、设置部分更新保留。
- 真实 EditChannelModal 浏览器回归覆盖已有配置无改动提交、改为 300 后重开、清空为 0、历史缺省继承、保留非流式值和未知 setting、键盘打开说明不误提交；桌面与手机渲染确认输入没有溢出。
- 诊断测试确认 stream/nonstream 覆盖标记不会混淆。
- 非图像/非流式范围、Gemini 原生 IsStream 判断和共享客户端未修改均已对照代码。没有新增 SQL、schema 或数据库特有行为。
- `bun run check:language-resources` 36 项通过，Vite 生产构建通过；桌面/手机截图已检查，设计检测无新增命中。`make verify-notes` 通过。本轮测试/浏览器/构建进程全部结束，遗留测试进程为 0。

## Consequences

用户可以仅对目标 Gemini 渠道试设流式响应头等待 180 秒，并将 0 用作继承与撤回；其他渠道默认不变。旧版本不会应用新字段，必须先发布能力再保存目标渠道设置。本次未改变任何生产配置，没有新的付费模型请求，也不能将本地验证称为生产已生效。

延长会占用连接与并发名额更久，不会解决供应商永久故障或更新切换截断；客户端总截止、正文读取超时及反代/CDN限制仍可能先结束请求。生产后续以管理员日志的实际时限、完整终态与测试进程退出作为验收条件，不从单次成功承诺长期成功率。
