# Agent Note: DeepSeek Messages 历史思考缺失的定向兼容

Status: implemented

## Problem

官方 DeepSeek 原生 Messages 对带工具的历史缺少 `content[].thinking` 返回 HTTP 400，现有渠道规则只匹配 Chat 的 `reasoning_content`，Claude 参数修复仅处理 Anthropic 渠道。通用失败提示无法说明历史思考缺失。已确认的错误不能证明所有 400 都可通过关闭思考修复，也不能证明同令牌相邻请求属于同一会话。

## Decision

复用现有有界错误读取、同渠道重发和管理员调整记录，在 DeepSeek 专用渠道、官方 HTTPS 地址、原生 Messages 路径、结构化 invalid_request_error 的精确历史思考缺失消息同时满足时，仅本请求把 `thinking.type` 设置为 `disabled`，最多重发一次。保持模型映射、消息、工具、有效历史思考及其他参数；官方将开关与 effort 独立定义，并明确忽略 `budget_tokens`，因此不删除 `output_config.effort` 或预算字段。已取消、已向客户端输出、透传及已关闭思考不重发。成功仅结算最终用量，最终失败沿用退款和错误日志。

最终错误从保留的结构化上游 message 识别该精确文案，避免本地追加的 request id 干扰分类，提示完整回传历史思考、新建会话或关闭思考；不向普通用户暴露供应商地址和请求内容。调整通过 `X-Request-Adjusted: deepseek_thinking_disabled` 和管理员 attempts 明确记录，最终用量日志把 reasoning_effort 标为 none，不能将非思考答复伪装成正常思考模式。

纯保活已经提交响应的定向例外由[纯保活补救](2026-10-25-deepseek-heartbeat-adjustment.md)接管；正文/工具输出、写失败及其他供应商仍沿用本篇边界。

## Existing capabilities and notes audit

[Claude 采样兼容](../../implemented/bug-fix/2026-10-19-claude-sampling-compatibility.md)部分重叠，继续持有采样参数范围和有界重试机制，本次不把语义变化纳入采样参数删除名单。[Claude thinking 保真](../../implemented/bug-fix/2026-10-17-claude-thinking-forwarding.md)部分重叠，有效历史块继续保真，仅明确拒绝后的本次思考开关例外另行记录。[DeepSeek Responses](../../implemented/bug-fix/2026-10-18-deepseek-native-responses.md)控制另一原生协议，保持独立；无应删除或拒绝的旧提案。

## Alternatives considered

- 全局预先关闭思考可避免一次失败请求，但会改变所有正常请求的语义，故仅以实际明确拒绝作为降级证据。
- 伪造或补齐历史 thinking 可保留表面开关，但无法恢复模型真实历史，故不生成或删除历史内容。
- 只要求调用方重新发送完整历史最保真，但现有缺失历史无法原地恢复；本次已授权兼容修复，选择可观测的一次非思考重发，仍保留客户端完整历史的根治提示。
- 修改第三方模型映射不能恢复历史思考，且用户明确要求保留既有映射，不纳入改动。

## Testing

14 个完整 Relay/SQLite 场景覆盖 JSON/SSE 成功、仍失败、渠道/全局透传、正常思考、默认开关和已禁用开关；成功钱包/令牌只扣最终 15 个用量单位且只有一条消费记录，失败预扣全退且只有一条错误记录。同渠道两次上游 Request ID 与管理员 attempts 可追溯，普通用户日志不暴露管理诊断。

25 个 helper 边界场景覆盖其他渠道/域名/路径/协议、第三方重定向、错误类型、非 JSON/过大错误体、已输出、取消、无重放体、重试传输失败及最多一次。原始消息、有效 thinking 签名和大整数保持原样；服务分类覆盖本地 Request ID 追加以及非目标错误。新增回归及既有 Claude 采样完整 Relay 回归通过。

`GOMAXPROCS=2 go test -race -parallel 1 -p 2 -timeout 120s ./relay ./service ./controller -run 'TestDeepSeekThinkingHeal|TestDeepSeekMissingThinkingDiagnostic|TestRelayDeepSeekThinkingHistoryCompatibility' -count=1` 通过。新增两句诊断在现有排查报告中有七语言翻译和回归；`make verify-notes`、`git diff --check` 通过。

## Consequences

一次失败后额外上游请求增加延迟；该次回复使用非思考模式，需响应头和管理员记录可见。模拟验证不能代替官方付费账号可用性验证，也不表示已部署生产。

官方依据（2026-10-23 核对）：https://api-docs.deepseek.com/zh-cn/guides/thinking_mode 、https://api-docs.deepseek.com/zh-cn/guides/anthropic_api 。
