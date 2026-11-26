# Agent Note: Vertex Gemini 接收 Messages

Status: implemented

## Problem

Vertex 根据映射后模型选择 Google 的 generateContent 地址，但 Messages 入站总是生成带 anthropic_version 的请求体，已在流式请求中导致 400。

## Decision

在 Vertex Gemini 文本模式选定后复用 Messages → Chat → Gemini 的转换，响应按客户端协议回写，流式终态携带最终用量。显式透传沿用原始请求体。

## Alternatives considered

客户端切换到 Chat 能绕开故障，但只支持 Messages 的客户端需要平台兼容。再次调用网关 Chat HTTP 路由会重复进入鉴权、选渠和计费。直接复用进程内转换器保留同一次请求的上下文。

## Upstream

QuantumNous/new-api 的主分支在 c27abf36268001ebfcf4f478ba0f7bae1c19de22 的[转换注册表](https://github.com/QuantumNous/new-api/blob/c27abf36268001ebfcf4f478ba0f7bae1c19de22/relaykit/relayconvert/text_converter_registry.go#L148-L164)将 Messages 到 Gemini 明确组合为 Messages → Chat → Gemini，质量标为 discouraged。[Vertex 入站](https://github.com/QuantumNous/new-api/blob/c27abf36268001ebfcf4f478ba0f7bae1c19de22/relay/channel/vertex/adaptor.go#L96-L108)仍复制成 Vertex Claude 请求。本仓库也有两段转换，复用现有 Gemini URL、请求、响应与用量处理。关联的 [Vertex 地址决策](../../implemented/bug-fix/2026-10-17-vertex-address-compatibility.md) 管理区域与凭据；本次只补入口选择与 Messages 响应生命周期。

## Consequences

客户端沿用 Messages。Gemini 原生字段承载系统消息、工具定义与历史、预算和映射后的思考强度；原始模型别名继续用于选渠。跨协议字段服从转换器的支持范围。普通 Gemini 与 Vertex 共用的 Messages 流在发完内容后等待最终用量，只在成功完成时发送一次终止事件。上游拒绝、内容被拦和流式异常进入现有错误处理。Claude 原生请求继续带 Vertex 的 anthropic_version。

## Testing

本地 Relay 请求经过模型映射与模拟 HTTP 上游，覆盖 Gemini 流式/非流式工具、工具结果历史、思考强度、最终用量、上游 400、内容被拦、截断后错误和原生 Claude 目标。controller、relay、service、三个适配器及 relay/common 回归通过。Go 构建的 embed overlay 排除了本机同步生成的带编号前端副本，二进制版本检查通过。本地验收没有调用真实 Vertex 项目。
