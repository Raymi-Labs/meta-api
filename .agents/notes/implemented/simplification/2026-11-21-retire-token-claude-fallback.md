# Agent Note: 令牌退出 Claude 自动换模型

Status: implemented

## Problem

调用方按模型选择能力和成本，原令牌开关会授权上游在拒答时切换型号。已发布版本可能留下令牌设置、Redis 缓存和已结算的逐模型账单。

## Decision

移除令牌开关、出站备选声明和新增逐模型结算。保留历史账单的只读解释和拒答消息 ID、类别。普通渠道故障继续走原模型已有的分组和渠道。

## Alternatives considered

- 只隐藏表单会让旧令牌的持久化值继续控制出站请求。
- 回滚整个版本会一并移除多段文本转换和历史账单展示。

## Consequences

曾开启该选项的令牌只请求原模型。升级允许旧表中保留该布尔列，历史缓存允许含旧字段。使用日志根据已保存的逐次用量显示历史费用。明确的 Claude 拒答通过原生结束原因或 OpenAI 兼容的 content_filter 返回；其消息 ID 与已知类别仅用于管理员诊断。

## Verification

控制器覆盖 v1.42.0 已开启的令牌，分别通过 Messages、Chat 及两种流式入口核对出站模型、请求头和拒答日志。SQLite、MySQL 8、PostgreSQL 16 与 Redis 7 验证旧值不影响令牌路由或余额。Chrome 核验新建、保存和编辑的高级设置。同模型渠道故障回退沿用控制器回归用例。

取代[令牌级 Claude 官方备选模型](../feature/2026-11-21-token-claude-fallback.md)的启用与出站决策。渠道故障沿用[现有回退预算](../bug-fix/2026-11-20-routing-health-recovery.md)。
