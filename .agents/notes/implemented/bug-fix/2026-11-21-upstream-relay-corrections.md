# Agent Note: 分组可见性、任务终态与协议修正

Status: implemented

## Problem

公共价格及性能接口可返回访问者不可用的分组。非终态任务显示 100% 后退出轮询。Gemini 回复的文本边界、推理档位和新版 GPT 的参数限制未完整适配。部分请求错误会影响渠道健康，已删除用户的密钥可能收到服务端错误。

## Decision

扩展已有分组、角色及模型可见性判断，管理面板保留授权汇总。任务沿用终态条件和现有结算。复用 Go 转换器、错误分类与令牌鉴权入口。

## Existing decisions

[Gemini 混合内容](../../implemented/bug-fix/2026-10-31-gemini-mixed-content.md)处理请求方向，此处补响应方向。[参数覆写预览](../../implemented/feature/2026-11-14-param-override-preview.md)沿用同一条件执行器。

## Alternatives considered

整分支移植会带入独立的 RelayKit 和新前端。只在浏览器隐藏分组仍会在 HTTP 数据和汇总中泄露信息。选择在本地后端边界适配。

## Testing

本地三库实际执行 99% 与 100% 进度、六种任务状态、排队和超时边界回归。SQLite 结算用例重复处理超时与 Suno 失败，验证只退一次。HTTP 回归覆盖访客、分组成员、失效成员、管理汇总、软删除密钥（含 Redis 热缓存和无缓存）和聚合前过滤。Gemini 回归对比混合思考片段与 JSON 字节，覆盖原生输入和 Chat 输入；OpenAI 与 Azure Chat 适配器检查两个 GPT-6.2 型号和日期快照。

## Consequences

历史非终态任务重新接受轮询和超时判断。公开性能样本随访问者分组变化。既有数据库结构可继续使用。
