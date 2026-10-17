# Agent Note: Claude 原生客户端上下文与审核响应保真

Status: implemented

## Problem

原生 Claude 入口重建请求头时未保留客户端身份与会话上下文；链式网关因此无法区分原生调用方。审核能力正文和多行 beta 已有保真能力，不应另建转发器或把所有客户端头无差别复制到上游。

## Decision

复用 Claude 适配器，仅在原生 Messages 格式下保留 User-Agent、X-App、Claude 会话/请求分类和客户端请求 ID。客户端鉴权、Cookie、代理鉴权和其他任意私有头不在默认透传范围。Connection 提名的逐跳字段不转发，适用于新增身份头以及现有 beta/version 读取。渠道管理员显式覆盖继续在适配器之后生效。

原生 Claude 到 Anthropic 适配器的公共 HTTP 客户端克隆现有重定向策略，拒绝跨源或降级重定向，避免新增身份/会话字段泄漏；同源仍执行原策略，其他协议与适配器不改变。保持 [thinking/beta 保真](2026-10-17-claude-thinking-forwarding.md) 的 DTO 和多行 beta 行为，以及 [OpenCode 会话头](2026-10-13-opencode-session-header.md) 的独立作用域。

不修改价格、用量、缓存 TTL、审核结论或生产配置。没有把版本字符串当作权限判断，也不伪造上游审核结果。

## Alternatives considered

- 通配全部请求头省事，但会泄漏客户端凭据、Cookie、代理上下文，拒绝。
- 只在渠道配置里填固定 User-Agent 掩盖真实版本，无法随客户端升级；保留实际来访值。
- 将所有请求都识别为可信原生客户端，会改变其他协议和安全边界；限定原生入口。
- 创建新透传端点会绕开已有鉴权、路由和计费，复用当前适配器更小且可回归。

## Consequences

链式代理可看到实际原生客户端上下文，但不能单独凭这些可伪造头授予权限。跨源重定向在原生 Anthropic 路径上现在明确失败，使用此类重定向的渠道必须配置最终合法目标。管理员显式覆盖保留其现有权限。未知 beta 是否支持由上游决定，不进行静默安全降级。

## Testing

Claude、公共 channel、AWS、relay、DTO、model_setting 包回归通过；相关竞态覆盖客户端密钥隔离、管理员覆盖、逐跳头、同源/跨域/降级重定向、CPA/OpenCode 相邻身份规则，以及 JSON/SSE 审核通过与拒绝结果、tool-use ID 和事件顺序。后端入口编译通过，使用临时 web/dist/index.html 编译夹具，未构建真实前端发布资源，因此该验证产物不是可部署发行包。没有向真实模型发送本轮请求，也没有上线。
