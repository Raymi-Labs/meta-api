# Agent Note: Claude 明确拒绝手动思考时的定向兼容

Status: implemented

## Problem

实际请求在原生 Messages 与 Chat 转 Messages 两个入口被上游明确拒绝：当前模型 requires adaptive thinking，要求省略 thinking 或使用 adaptive 和 output_config.effort。已有 suffix 适配只覆盖部分型号，不能据第三方渠道的别名推断所有同名模型能力；已有采样补救不会修改思考语义。

## Decision

复用共用 Claude 参数补救，在 Anthropic 类型渠道的真实 HTTP 400、结构化 invalid_request_error、明确指向本次最终模型的完整 adaptive 拒绝同时满足时，仅把本次最终请求的 thinking.enabled 改成 adaptive 并移除 budget_tokens。保留最终 payload 已有合法 effort。Chat 转 Messages 曾将调用方明确的 low/medium/high 编码为手动预算，最终 payload 缺省 effort 且无渠道参数覆盖时恢复该原始档位；来源不明、未知档位或存在覆盖时不补救。原生只给预算时继续缺省 effort，不用不可等价的预算数字推导新的档位。保留 max_tokens、消息历史、工具、display/block_binding 及其他请求字段；未知 thinking 字段或非法 effort 拒绝自动补救。

原始请求明确关闭思考、透传、取消、已输出响应、未知/歧义拒绝不重发。与采样补救共用同请求最多一次预算，原渠道重发，沿用错误体完整恢复、管理员 attempts、上游请求 ID 和一次计费/失败退款。补救返回后先停止并等待保活协程，再仅在响应头尚未提交时标明 claude_adaptive_thinking；已提交则在本次管理员 attempt 记录 adjustment_header_not_delivered=true，不伪装已交付。此可观察性检查不放宽开始补救时的已输出保护。不建立型号全局名单或能力缓存。

## Existing capability review

- 两个入口已共用 relay/claude_param_heal.go；不新增 handler 分支或新的重试链。
- [采样兼容](../../implemented/bug-fix/2026-10-19-claude-sampling-compatibility.md)部分重叠，保留其采样删除边界，本篇单独管理思考模式改变；[thinking 保真](../../implemented/bug-fix/2026-10-17-claude-thinking-forwarding.md)部分重叠，未知扩展不被静默丢弃。
- [DeepSeek 补救](../../implemented/bug-fix/2026-10-23-deepseek-messages-thinking-history.md)属于其他供应商和不同语义，不扩展其范围；[本轮保活改动](../../implemented/bug-fix/2026-10-25-deepseek-heartbeat-adjustment.md)独立管理 DeepSeek 的纯心跳例外；两者都承认已经提交的响应头无法补送，不放宽 Claude 已输出保护。无完全吸收或应废弃的活跃笔记。

## Alternatives considered

- 按模型前缀预先转换能节省一次请求，但不能证明第三方同名模型具有相同契约，故以当前上游明确拒绝为触发条件。
- 直接删除 thinking 也符合拒绝提示，但会丢弃显式启用意图及显示/绑定控制，故使用 adaptive 并保留可迁移字段。
- 按 budget_tokens 自动设置 effort 看似接近原深度，但两者没有通用等价映射，而且模型默认 effort 不同；已有显式 effort 保留，缺省不强加新档位。
- 自动删除未知 thinking 扩展可增加成功机会，但其语义未知；保留原错误供管理员处理。

## Testing

完整 Relay 矩阵运行在单独子进程，复用当前测试二进制保留 race 检测、环境哨兵防递归、失败完整返回。健康采集器会随真实 Relay 懒启动，原数据库 fixture 只还原 model.DB，后台采集器可能在其他无数据库测试或 race 退出等待期间继续 flush；子进程内数据库指针与连接保持至退出，场景间只清业务 fixture，健康表供真实采集器正常运行。进程隔离约束其生命周期，不改真实路由、计费、日志断言或业务采集器。与本轮 DeepSeek 矩阵共用测试 helper。

22 个完整 Relay / SQLite 场景覆盖原生 Messages / Chat 转 Messages、流式/非流式、明确拒绝后成功/仍失败、透传、正常接受、显式关闭。核对请求差异仅限目标字段，成功仅一条消费记录且用户/令牌仅扣 15 个最终用量单位，失败预扣全退；上游请求 ID、管理员调整记录和普通用户日志脱敏通过。

额外两条流式场景使用本地 HTTP 上游：首次精确 400 立即返回，第二次等待下游实际 flush 本协议标准 ping 后才响应。修复前复现补救完成后修改已提交 Header 的问题；现在客户端与 writer 都不出现事后调整头，管理员 attempt 明确记录未交付，两个入口均只有一条消费日志和 15 用量扣费。

helper 覆盖精确拒绝和供应商私有别名、结构化错误类型/状态/渠道/模型不符、未知 thinking 字段/effort、取消、已输出、过大错误体、非法 JSON、网络失败以及与采样补救共享一次预算；保留历史消息、工具、display/block_binding 和大整数。验证已有 effort 优先、原始 Chat 档位恢复、存在渠道覆盖时拒绝推断及原生预算不推导档位。

`GOMAXPROCS=2 go test -parallel 1 -p 2 -timeout 180s ./relay ./controller -run 'TestClaudeAdaptiveThinking|TestRelayClaudeAdaptiveThinkingCompatibility|TestClaudeSamplingHeal|TestRelayClaudeSamplingCompatibility|TestTryHealClaudeParam|TestExtractHealable' -count=1` 通过；新增 Claude adaptive 的 relay/controller 定向 `-race` 通过。原采样补救回归保持通过。

## Consequences

手动预算变为 adaptive 会改变推理控制方式，不保证输出质量、延迟或费用等价；max_tokens 保持原值但无法保留手动思考预算语义。调整在既有管理员日志中保留；补救等待期间心跳可能已经提交响应头，此时调用方收不到 X-Request-Adjusted，仅管理员 attempt 的 adjustment 与 adjustment_header_not_delivered 留痕。供应商现场效果仍需独立验收，不发送付费调用或部署生产。

官方依据（2026-10-25）：https://platform.claude.com/docs/en/build-with-claude/extended-thinking#migrating-to-adaptive-thinking 、https://platform.claude.com/docs/en/build-with-claude/effort 。文档支持协议字段迁移，不用于推断第三方别名能力。
