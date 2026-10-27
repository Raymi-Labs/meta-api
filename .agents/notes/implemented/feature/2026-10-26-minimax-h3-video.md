# Agent Note: MiniMax H3 文生视频基础链路

Status: implemented

## Problem

H3 不在视频安全预检识别范围，MiniMax 视频适配器仅使用 Hailuo V1。任务表达式读取原始参数而适配器可以补默认值或覆盖 metadata，不能保证收费与实际请求一致。

## Decision

复用 MiniMax 渠道与既有任务提交、资金预留、终态 CAS 和退款，按映射后的完整 H3 型号切换 V2 提交和查询；旧 Hailuo 保持 V1。首期明确支持 H3/Highspeed/Lite/Max 文生视频，不支持带额外输入成本的参考素材、编辑或上下文任务。

模型映射后、计价与预扣前提供可选准备接口。H3 解析一次规范参数，明确 duration、resolution、ratio，冲突或不支持参数在本地拒绝；同一字节快照用于表达式和出站，不复用其他尝试的快照。计费要求明确按次价或表达式，不回退通用默认价。查询使用持久化上游模型决定 V1/V2，不依据当前渠道映射；V2 成功必须有合法视频 URL，异常响应不伪装终态。

H3 任务保存提交时 Key，后台与实时查询共用持久化型号/Key及终态处理；usage 换算 Token 不作为文本 Token 二次扣费。开始提交后禁止自动重发，避免响应丢失产生重复付费任务；本地失败退款不证明上游未收款。

后台已知 H3 型号的表达式模式提供 USD/请求输出秒模板，按允许分辨率全部填价后生成表达式，仍需明确应用更改保存。零价有效但缺价不可生成，生成保留原请求倍率表达式；不反解任意表达式为模板。文本 Token 估算器和文本预设在 H3 下隐藏，避免空请求校验的零值被当实际费用。17 种界面语言同步文案。免费估价复用正式 H3 参数规范化，不发上游或预扣。

模板兼容原设置验证器：无 duration 的配置预览分支为零，正式 H3 在计费前拒绝缺 duration；不支持档位使用负值分支触发引擎错误，不按免费处理。模板只对已知且具有参数验证的 H3 显示，不能移植到任意未校验模型。请求计费单位仍为原表达式内部百万单位，模板负责 USD/秒换算。

## Sources

2026-10-26 读取 MiniMax 官方 video-generation-v2-create.md、video-generation-v2-query.md：POST /v2/video_generation 返回 task_id；GET /v2/query/video_generation/{task_id} 返回 task，状态 queued/running/succeeded/failed/cancelled，输出 content.url。H3 768P/2K、Highspeed 768P、Lite 480P/768P（4–15 秒），Max 480P/768P（5–15 秒）。文生视频 ratio 必须具体，不用 adaptive。第三方是否兼容需实测。

## Alternatives considered

- 只新增模型名沿用 V1 最小，但协议与返回结构不同，无法可靠运行。
- 统一重写全部视频适配器可覆盖更多模型，但扩大回归风险；本次用可选接口仅规范 H3。
- 接收所有参考素材便于用户，但没有可靠输入素材计费，先明确拒绝而非低估费用。

## Verification

`h3_test.go` 用本地 HTTP 验证提交/各查询状态/保存密钥/旧 V1 回归，缺结果、错误 envelope、未知状态、任务 ID 不符和非法 URL 均不返回成功。参数覆盖缺价、冲突、非法时长/分辨率、未知型号、metadata 模型覆盖和参考素材拒绝。

`relay/minimax_h3_test.go` 经实际 RelayTaskSubmit 验证模型映射、别名规范参数、表达式计价与出站字节一致，5秒×$0.10×分组2= $1；实时成功/失败/取消和后台旧快照处理复用单次终态，钱包无重复扣退。非法输入不创建 Billing 会话、不发送上游并清除旧快照。controller 验证 H3 测试为无生成预检、估价同源和提交后不自动重试。

模板 JS 与后端共享表达式 fixture，验证设置 SmokeTest 可接受、正确金额、缺档和超时长拒绝。浏览器 mock 验证模板生成不保存、明确保存/重新打开、M2.5 隔离和手机布局；未连接真实服务。全套 task adapters、relay、service、controller、model、common、billing_setting 回归及 H3/任务资金/终态 race 回归通过；前端主站构建及修改源码 ESLint 通过，旧大分包/Browserslist 提示保留。

## 提交前复核（2026-10-27）

基于远端 `20f5a3dec` 复核本地未提交工作，relay 全部子包及 service/controller/model/common/billing_setting 回归通过。清理任务轮询中重复的响应体关闭注册，并定向复验 service。前端源码静态检查、H3与通用视频定价模板回归通过。

真实验收仍未通过：10月26日另一本地聊天直连 GetToken 提交一笔最低规格 H3，虽然返回任务ID，但等待后没有视频地址，原生V2与标准任务查询状态不一致，测试额度出现扣减/占用。本次不重新生成，不将本地模拟通过作为生产可用证据。生产启用前仍须确认供应商协议、任务终态与最终费用，并通过元衡普通用户链路验收。

## Consequences

本地模拟不等于真实渠道可用；不发付费生成、改线上价格、推送或部署。首期按请求参数计价，不宣称按最终实际秒数补差额。结构化价格表、参考生成和收费验收仍属于原提案。

复用原任务字段，无 schema 迁移；本地资金测试使用 SQLite，未执行生产 MySQL/PostgreSQL 实例验收。原 CAS 与资金变更非崩溃原子事务的限制保留。为了避免重复付费，H3 提交后的临时错误不自动重试，需要用户核对上游后决定；V2 返回的地址遵循原内容下载安全边界。

## Related notes

[多厂商共同契约](../bug-fix/2026-10-26-video-task-common-contract.md)将快照/提交防重/保存Key与配置辅助扩展至已有适配器，H3参数限制和V2协议仍由本文负责。

[视频框架](../../proposed/feature/2026-10-21-video-pricing-and-testing.md)部分落地，本篇只接管 H3 文生视频；[免费预检](2026-10-21-video-free-preflight.md)继续负责无生成测试；[任务资金与终态](../bug-fix/2026-10-24-task-submission-and-terminal-consistency.md)负责原资金和 CAS，复用而不改设计。其他厂商/日志/模型广场笔记无关。
