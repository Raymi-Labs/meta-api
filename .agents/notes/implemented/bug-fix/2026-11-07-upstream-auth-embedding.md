# Agent Note: 2FA 窗口、Vertex 令牌缓存与 Gemini 向量用量

Status: implemented

## Problem

2FA 过期锁定仍累积旧失败次数。Vertex AsyncCache 首次 miss 后不能正确填入新 token，重复签名和交换 JWT。Gemini 向量接口忽略 usageMetadata，实际结算依赖本地估算。

## Decision

定向适配 9b473a3fe、a5a465926、ba5f73677。2FA 保留数据库 CAS，过期窗口从 1 开始。Vertex 按渠道/密钥槽缓存到供应商有效期前五分钟，singleflight 合并并发交换，凭据指纹包含完整私钥与代理以识别无 key ID 的换钥；失败和短寿命 token 不缓存，保留任务适配器 AcquireAccessToken 契约和渠道代理。Gemini 原生单条、批量与 OpenAI 兼容入口共用上游向量用量解析，正数用量优先，无有效用量保持原估算；不把聊天输出或向量维度算成输出 token。

原生 batchEmbedContents 经大小写敏感的 embed 子串判断会误入聊天 handler；分派改为两种明确的 endpoint 后缀，让批量入口实际到达已有向量处理器。HTTP token 交换检查状态、空 token、有效期范围，错误不回显响应中的凭据，使用 common JSON wrapper；缺失 expires_in 沿用一小时默认值。

## Existing notes audit

[向量输入校验](../../implemented/bug-fix/2026-10-24-gemini-embedding-input-validation.md)部分关联，保留整批拒绝和退款契约；[Vertex 地址](../../implemented/bug-fix/2026-10-17-vertex-address-compatibility.md)部分关联，保留区域/项目/API Key 路由。其余 Gemini 图片/混合消息和认证表单笔记不管理本次缓存与结算，无需替换。

## Alternatives considered

- 整段 cherry-pick 可减少手工工作，但上游 RelayKit 与本地 DTO、任务代理和资金渠道不同，选择行为级迁移。
- 修补固定 30 分钟缓存较简单，但不遵循 expires_in，也不合并并发交换和识别换钥，采用到期缓存。
- 继续只估算 embedding 能保留旧金额，但忽略供应商实际 token；仅无有效上报时保留估算。

## Testing

定向 Go 测试覆盖锁定过期/有效锁定/并发；token 复用/过期/多槽/换钥/失败/短寿命/HTTP 交换；三个 embedding handler 的用量缺失、null、零、负数、模态细分。Controller 的本地 HTTP 上游与 SQLite 回归覆盖三个入口乘以钱包/订阅/token-only 共九条资金路径，上游 37 tokens 同步返回、日志与余额。既有非法输入退款回归保留。没有真实供应商付费请求；MySQL/PostgreSQL 未在本轮实测，数据库查询沿用现有跨库 GORM CAS，没有 DDL。

## Consequences

本轮相关模块全测与定向 `-race` 通过，Go 构建/版本检查、Bun 全构建、29 项发布工具测试与 `make verify-notes` 通过。本机无 `/usr/bin/chromium`，预渲染按现有脚本跳过。`go test ./...` 仅 `pkg/route_insight` 因后台采样访问 nil DB panic；在未修改的 0a25e17b8 独立工作区 `go test ./pkg/route_insight -count=10` 复现同栈，属于现有测试时序问题，本轮不扩展修改路由观测模块。

向量结算随真实用量变化，不修改价格。进程内缓存随重启失效，任务 AcquireAccessToken 保留按次获取。没有数据库 schema 迁移，也不代表生产部署或真实供应商调用验收。
