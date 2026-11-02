# Agent Note: Responses 工具声明与用量告警分离

Status: implemented

## Problem

Responses 响应可包含 namespace 工具分组以及 function/custom 声明。当前用量处理器将响应中未出现在请求统计表的任何工具类型打印为 ERROR；线上一小时出现上万条 namespace 告警，对应请求均有成功消费记录，正常流量掩盖真实故障。

## Existing capabilities and impact

复用 `relay/channel/openai/relay_responses.go` 的共享用量入口，覆盖原生 JSON、流式终态和 SSE 聚合响应；`relay/common/relay_info.go` 从请求构建工具统计表，`relay/compatible_handler.go` 与 `service/tool_billing.go` 分别消费 Token 与内置工具用量。前端、鉴权、路由和数据库不变。

已检索活跃笔记 namespace、工具声明、工具用量、Responses 计费。DeepSeek 原生 Responses 与 Kimi 请求完整性笔记只覆盖协议转发和保真；计费一致性笔记覆盖 Token 缓存组成，没有持有本次日志分类决定，保留原笔记。本轮仅新增该局部诊断边界。

## Decision

- namespace、function、custom 是非独立计费声明，跳过用量表查找与告警，不修改响应内容。
- 其他缺失统计项改为 WARN；每个响应按 web_search、file_search、image_generation、unknown 四类去重，不输出未知类型原文、工具名称或参数。
- 保留已登记工具计数、搜索实际输出次数、图片实际调用检测，以及 Token/缓存用量和错误终态。

## Alternatives considered

- 只降低全部日志级别：修改最小，但正常 namespace 仍持续刷日志，无法解决噪声量。
- 删除全部缺失工具诊断：可完全安静，但会掩盖计费工具元数据缺失，故保留有界警告。
- 为 namespace 新增独立调用费或展开其工具统计：会改变计费语义，超出本次诊断修复范围。

## Testing

JSON、SSE 与 SSE 聚合响应中的嵌套工具、调用 ID、参数保持；输入、输出、缓存 Tokens 和已登记计费工具计数保持；正常声明不产生 ERROR/WARN；重复未知类型不会刷屏或泄露原文，真实响应失败仍失败。`go test ./...`、完整服务端 `go build`、定向 `go test -race`、`make verify-notes` 与发布策略检查通过。前端和文档完整构建通过（独立 PR #155 补齐主线已有翻译缺口）。回归对比有无声明时的工具费用，覆盖真实失败终态与重复未知类型。

## Consequences

正常声明的日志与内部无效计数被移除；计费工具的缺失元数据仍可诊断。原生 JSON/SSE 保持原文，聚合路径继续沿用既有 DTO 序列化；本次不扩展聚合路径未建模字段的保真能力。

未知类型合并为 unknown 降低类型级诊断粒度，以请求 ID 定位；不证明所有供应商的工具费用均正确。发布不等于生产部署，不自动更新生产。
