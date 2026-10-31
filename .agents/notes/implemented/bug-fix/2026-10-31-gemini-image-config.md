# Agent Note: Chat 顶层 Gemini 图片配置保真

Status: implemented

## Problem

Chat DTO 丢弃顶层 imageConfig，Chat 转 Gemini/Vertex 时无法携带该配置；现有 extra_body.google.image_config 支持 snake_case 两个字段且有拼写校验，不能因此被替换。上游1dafb19a4提供顶层字段兼容。

## Decision

为 GeneralOpenAIRequest 增加 RawMessage imageConfig，在 Gemini generationConfig 初始化时原样携带。既有 extra_body.google.image_config 的有效映射继续后执行并整体覆盖顶层值；不合并两个不同配置，不改变既有拼写错误拒绝。缺失不生成默认值，显式null保留，其他JSON交给上游验证。

## Existing decisions

检索活跃笔记没有图片配置归属。[Schema 保真](../../implemented/bug-fix/2026-10-24-schema-order-preservation.md)部分重叠：沿用 RawMessage 和 DeepCopy 保真，但不对图片配置套用工具 Schema 清洗；[混合消息](../../implemented/bug-fix/2026-10-31-gemini-mixed-content.md)是相反转换方向，不改变其消息规则。

## Alternatives considered

- 只保留 extra_body 可避免新字段，但调用者已有顶层配置会继续被静默丢弃。
- 合并两个入口可保留更多字段，但混用两组长宽比、尺寸及未知字段难以解释优先级；沿用已有显式 extra_body 覆盖顺序更可预测。

## Testing

新增测试在旧实现复现顶层配置丢失；18个 Gemini/Vertex 组合通过。`go test ./dto ./relay/channel/gemini ./relay/channel/vertex ./relay/channel/openai`、Go 构建通过。从 JSON 解码、DeepCopy、Gemini/Vertex 实际 adaptor 到最终序列化，检查顶层配置、未知字段大整数、缺失/null、旧入口、冲突覆盖及非法拼写；原始输入保持，原生 Gemini 入口不变。

## Consequences

收益是已有客户端图片配置不再被网关静默丢弃；代价是需维护两个入口的覆盖规则。额外字段可能被供应商拒绝，不为任意 JSON 宣称可用。增加共享 DTO 字段意味着其他 Chat 兼容转发也能保留该显式扩展，与其他已有扩展字段相同；不按模型猜测、设置或收费。
