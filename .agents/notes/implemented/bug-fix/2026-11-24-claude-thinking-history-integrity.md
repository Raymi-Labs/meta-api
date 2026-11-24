# Agent Note: 保留 Claude 消息历史中的空思考块

Status: implemented

## Problem

Claude Opus 5.5 可以把思考正文留空，仅在签名中携带后续对话所需信息。生产的同一笔失败请求在出站前被删除了 24 个空思考块。日志没有记录这些块是否带签名，其他请求经清理后也有成功记录。

## Decision

原生 Messages 通过现有 DTO 转发历史内容；空文本、签名、消息边界和顺序均随请求送出。上游明确拒绝历史思考块时，共用 400 处理器交还原错误。采样参数重发沿用已有的出站请求体。

官方说明指出，省略思考展示时签名仍承载完整信息。[Claude thinking 文档](https://platform.claude.com/docs/en/build-with-claude/thinking)

本决策取代[采样兼容笔记](2026-10-19-claude-sampling-compatibility.md)中沿用的空块清理，补充[原生 thinking 转发](2026-10-17-claude-thinking-forwarding.md)的消息历史边界。DeepSeek 明确拒绝后的模式调整、adaptive 参数迁移、schema 顺序与心跳调整属于独立机制。

## Alternatives considered

- 仅保留带签名的空块可以修复官方示例，但仍会改写调用方提交的历史并可能合并消息。
- 全局开启原始请求透传可绕过清理，但也跳过现有模型映射和其他显式网关设置。

## Testing

使用模拟上游：完整 Messages 请求的 JSON/SSE、透传及采样参数重试均比对整个消息列表，覆盖带签名的空块、无签名空块、redacted_thinking 和逐条 output_config。明确拒绝空块的单测核对首次 400 的完整错误体和零次补救重发。新断言在旧实现中复现删块和重发，修复后通过。

## Consequences

曾依赖自动移除无效历史块的调用端将收到上游错误，须从原始会话修正对应消息。
