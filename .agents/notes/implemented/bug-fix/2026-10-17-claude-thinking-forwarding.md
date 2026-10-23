# Agent Note: 原生 Claude thinking 与 beta 保真

Status: implemented

## Problem

本地 DTO 会丢弃 block_binding 和未知 thinking 字段，Header.Get 仅读取第一条 anthropic-beta，Bedrock 转换还会保留空白项。

## Decision

参照 767a82b87，DTO 通过 common JSON 包保留原始扩展并支持重试深拷贝，合并全部 beta header lines；Bedrock 将非空逗号项转换为数组。保留渠道设置追加头及公共显式覆盖优先级。原生 handler 因模型 suffix 重建 adaptive thinking 时单独保留 block_binding，不继承旧推理预算；原生适配器沿用直通，不引入 RelayKit，不改变跨协议 thinking 生成与计费。

## Alternatives considered

- 只新增 block_binding 改动最少，但未来扩展仍静默丢失；采用局部未知字段容器。
- 直接转发原始请求体能保留一切，但会跳过本地模型映射及参数处理；仅在 DTO 的 thinking 范围保真。

## Testing

dto、relay、Claude、AWS、model_setting 全包回归通过；相关定向竞态和构建通过。验证未知字段、null/对象、deep copy 隔离、原生转换、adaptive 绑定恢复、全部 beta 行、模型配置追加以及 Bedrock 数组。未向真实供应商发送请求。

## Consequences

供应商可能拒绝未知字段，网关只保真不保证供应商支持。活跃 OpenCode 会话头笔记部分相邻但控制的头和目标不同；保留其覆盖规则，其他 Kimi/日志笔记不相关。

[DeepSeek 历史思考兼容](2026-10-23-deepseek-messages-thinking-history.md)仅在官方明确拒绝缺失历史时改变本请求的思考开关，原有历史 thinking 块与扩展字段仍保真；正常原生请求维持本篇约定。
