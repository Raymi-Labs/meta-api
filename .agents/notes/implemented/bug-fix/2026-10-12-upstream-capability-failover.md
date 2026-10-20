# Agent Note: 明确上游能力错误的有界安全切换

Status: implemented

关联：[Responses 空输出取消共享冷却](2026-10-13-responses-empty-output-no-shared-cooldown.md)保留空输出的失败与禁止重试，只收窄共享冷却，不扩展本篇的能力拒绝切换范围。

## Problem

已配置的模型可能被所选上游以 model_not_found / unknown provider 拒绝，工具请求也可能被上游账号的 Chat Completions 能力限制拒绝。普通 400 不重试的规则把这些渠道能力问题直接返回给用户。部分上游把 JSON 错误与 response.failed SSE 拼接，严格 JSON 解码失败后还会丢失具体原因。

## Existing capabilities and impact

复用 [错误解析](../../../../service/error.go)、[重试策略](../../../../service/channel_retry.go)、[Relay](../../../../controller/relay.go)、[选渠](../../../../service/channel_select.go) 和既有失败日志。路由入口经鉴权、分组和令牌渠道白名单后进入 Relay；重试排除当前渠道，计费只结算最终成功，失败退款。

已有 IsChannelModelMismatchError 分支会重置重试次数，不能直接扩充该分支来处理本次错误。新增识别必须走消耗 RetryTimes 的普通切换路径，保留具体上游错误、请求 ID 和尝试渠道。管理员日志可保留诊断，普通用户日志仍使用脱敏理由。

检索活跃笔记：与 [K3 流完整性](../../implemented/bug-fix/2026-09-29-kimi-request-stream-integrity.md) 部分重叠，沿用输出后禁止重放的决定；[响应头超时](../../implemented/feature/2026-10-05-channel-nonstream-header-timeout.md) 只提供相邻保护，不改变超时重试规则；其他渠道用量、真实测试与 UI 笔记不持有本次决定。

## Decision

- 仅识别带真实上游 HTTP 400 证据、结构化错误类型和明确文案的两类能力拒绝；不把通用 model_not_found、普通参数错误、会话状态丢失或任意文本都视作可重试。
- 对已观察到的 JSON + 单个 response.failed SSE 格式做最多 64 KiB 的有界恢复，只接受明确能力错误、空输出的失败终态；保留原始状态及上游诊断。未知尾部、成功/输出事件和损坏内容仍按原错误路径处理。
- 沿用当前分组、模型及令牌权限选备选渠道，消耗配置的重试次数，排除失败渠道。指定渠道、取消、skip-retry、已向下游写入响应，以及依赖 previous_response_id / conversation 的请求不新增切换行为。
- 无备选或次数耗尽时保留最后上游原因，不变成误导的本地无渠道错误。不永久禁用渠道、不修改线上路由、权重、模型映射或超时。
- 使用本次已尝试渠道数限制总预算，避免 auto 分组切换重置优先级计数后扩大新增重试。成功消费日志的 admin_info 保留失败尝试及其上游请求 ID，普通用户日志清除全部 admin_info。

## Alternatives considered

- **把所有 400 纳入重试**：配置改动小，但会重放参数错误和会话状态错误，增加无效请求与费用，因此使用明确错误白名单。
- **扩展现有模型不兼容分支**：复用最直接，但该分支不消耗 RetryTimes，且会绕过部分普通重试保护，因此让新能力识别走有界普通重试。
- **只移除问题渠道的模型配置**：可立即避免命中，但无法处理其他上游未来出现的同类错误；这是独立的生产运维动作，本次仅修复网关。

## Testing

service 回归覆盖标准与混合格式、CRLF、真实上游状态、错误类型/代码、普通参数错误、成功/已有输出事件、任意尾部、长度上限，以及固定渠道、取消、已写入响应、保活、会话状态与重试预算。controller 的 11 个完整 Relay 场景使用真实本地 HTTP 上游、SQLite 与计费流程；两类拒绝分别完成三轮工具回环，验证首次失败未泄漏到成功流、请求历史一致、单次结算、终态失败退款、尝试关联及普通用户日志脱敏。

`go test ./service ./controller ./relay/... ./middleware ./model ./types ./router`、相关 service/controller 竞态检查、Go 构建和 make verify-notes 通过。前端生产构建及五个公开页 Chromium 预渲染通过；无前端业务改动，构建用于更新发布版本和嵌入资源。发布附件、镜像与生产部署分别核验，不把本地协议桩当第三方稳定性证明。

## Consequences

[HTTP 请求 ID 一致性](2026-10-20-response-request-id-integrity.md)约束重试共享的本站响应 ID，并按实际收到的上游响应分别记录渠道和 ID，不改变本篇的 failover 预算和输出保护。

收益是已知上游能力拒绝可以在授权范围内由备选渠道完成请求，管理员能定位被切换的失败尝试，用户不再被误导为参数错误。代价是每次成功回退最多增加既有预算内的上游尝试，管理员日志增加有界诊断记录。

识别采用窄白名单，未知上游文案仍可能直接失败；新增格式兼容不能接受任意损坏 JSON。响应已写入（包含保活）时保守停止切换，避免拼接两路流。未增加渠道能力数据库、永久禁用、超时重试或自动部署；第三方长期稳定性仍须部署后验收。
