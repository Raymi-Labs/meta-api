# Agent Note: Claude stop 安全解析

Status: implemented

## Problem

Chat 转 Claude 对 stop 数组元素使用 string 强制断言，混入数字、null 或对象会触发请求级 panic/500；程序构造的 []string 则被忽略。线上已确认使用此转换入口，但没有确认真实请求曾触发该异常；本地用同版转换器复现，不向线上注入异常请求。

## Decision

按上游 bc795cc11 的兼容策略解析 string、[]string、JSON 解码后的 []any。数组仅保留字符串项，保留顺序、空字符串和重复值；未知类型不生成 stop_sequences。不修改其他请求字段、错误处理或结算路径。

## Existing notes audit

检索 stop_sequences、StopSequences 和 Claude stop，无同范围活跃笔记。[Claude 流终态](../../implemented/bug-fix/2026-10-31-claude-stream-terminal.md)处理响应结束，不管理请求 stop；[工具参数保真](../../implemented/bug-fix/2026-10-31-claude-tool-number-preservation.md)管理同一转换器的工具输入，保持原规则。

## Alternatives considered

- 非法数组整体返回 400 能更严格提示客户端，但会拒绝同时包含有效停止词的兼容请求；本轮选择上游已采用的过滤策略，不新增通用参数校验政策。
- 仅加类型断言保护改动最少，但会遗漏程序构造的合法 []string；统一覆盖三种已有表示。

## Verification

- 修复前混合/全非法数组复现 panic，[]string 丢失；修复后 11 种输入通过，包括真实 JSON 编解码和程序构造两条路径。
- 空值、空数组、未知类型、空字符串、重复值及数组原值保持均有回归；Claude/OpenAI/service 包测试、Claude 请求转换定向 race 和后端构建通过。

## Consequences

非法项被忽略而不是报错，完全非法时不发送停止词；这是明确选择的兼容行为，不代表参数已全面校验。没有生产设置、数据库、计费或流式终态变更。
