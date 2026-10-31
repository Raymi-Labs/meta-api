# Agent Note: Gemini 请求中的文本媒体与工具调用共存

Status: implemented

## Problem

GeminiToOpenAIRequest 在有 functionCall 时使用 else-if 跳过全部文本/媒体。OpenAI 适配器的原生 Gemini 入口调用该函数，混合 model 消息因此丢失上下文。上游0165fef98修复同类转换；本地不是 RelayKit，不能直接移植其文件。

## Decision

复用现有转换器，将 tool_calls 和 content 独立赋值，保留文本字符串、多个媒体数组、工具顺序/ID 和空消息过滤。只处理原来支持的 parts，不扩展媒体语义，不修改协议路由、计费或工具关联算法。

## Existing decisions

检索活跃笔记后，[Schema 保真](../../implemented/bug-fix/2026-10-24-schema-order-preservation.md)部分重叠但只规定工具定义的 Schema，不涉及消息 content；[向量校验](../../implemented/bug-fix/2026-10-24-gemini-embedding-input-validation.md)是独立端点，不改变。没有需要否决的重叠提案。

## Alternatives considered

- 导入 RelayKit 可复用上游测试，但会扩大到协议和计费架构迁移，超过消息保真范围。
- 拆分成独立 assistant 文本消息也能保留文本，但改变轮次、工具调用关联及媒体所在消息；独立填充同一消息符合现有 DTO。

## Testing

修复前文本/媒体与工具共存的4个流/非流用例复现丢失，修复后全部10个组合通过。`go test ./service ./relay/channel/openai` 和 Go 构建通过。通过实际 OpenAI adaptor 从 JSON Gemini 输入转换并序列化，覆盖流/非流、文本加多个工具、媒体加工具、工具或文本单独出现、空消息；预算、模型、工具参数和输入对象保持。不发送真实供应商请求。

## Consequences

收益是混合消息不再静默丢上下文；代价是需维护两种协议各自的内容表示。OpenAI Chat 的 content 和 tool_calls 分字段表示，无法保证与 Gemini parts 的跨类别交错顺序完全一致。保留同类顺序不代表新增任何供应商媒体能力；没有真实请求正文证明线上已触发该缺陷。
