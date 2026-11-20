# Agent Note: 自动派生昨日消费查询凭据

Status: implemented

## Problem
已有 Worker 结算链路的部署不应为了读取昨日汇总再人工同步一组随机值。

## Decision
沿用原绑定责任与只读角色。显式独立密钥优先，否则从有效 Worker 密钥按固定用途派生，跟随轮换。不改 Pulse 仓库。

## Alternatives considered
接受结算角色本身最省代码，但会鼓励将原始资金凭据传给读取方；SSO 票据用于浏览器登录而非后台批处理。

## Verification
固定测试向量与真实 GET 验签、重放拒绝、轮换/移除旧键、显式覆盖、禁用和凭据冲突均通过。相关 Go 包测试与构建通过。

## Consequences
默认值改变：已配置结算的 Worker 新增只读能力。不能将派生密钥提供给任意前端。

仅替代[原方案](2026-11-20-pulse-daily-usage.md)的密钥分发决定，原接口及绑定责任不变。
