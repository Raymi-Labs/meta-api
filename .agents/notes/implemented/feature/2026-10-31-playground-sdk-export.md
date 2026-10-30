# Agent Note: 模型体验的 SDK 代码导出

Status: implemented

## Problem

用户要求右侧按参考图提供协议标签、SDK 示例、语法高亮和直接复制运行的代码。现有工作区只导出 Chat Completions 的底层 HTTP 请求；企业页已有 SDK 基础示例，模型详情已有协议声明和模板提示，但它们不包含当前对话、图片与工具回合。

## Decision

复用工作区负载与协议声明，扩展右侧导出为 Chat Completions / Messages / Responses、Python / JavaScript / cURL / JSON。Python 与 JavaScript 使用 OpenAI 或 Anthropic SDK，显示可复制的依赖安装、环境变量和执行命令。高亮复用已有 highlight.js，生成内容先交库转义，不拼接不可信 HTML。代码只读取 META_API_KEY，不导出会话或真实密钥。

协议选择仅控制导出，聊天仍走现有会话接口。转换普通文本、系统提示、图片和工具定义/回合；不支持且有语义的字段阻止跨协议复制并显示字段名称，不默默丢弃。Messages 缺少 max_tokens 时明确采用 4096；仅系统提示或空白状态补 Hello! 示例，按用户要求不显示额外的空白消息提示。未知协议支持保留模板并复用模型详情提示。

## Related decisions

- 部分扩展：[三栏工作区](../../implemented/feature/2026-10-30-playground-workspace.md) 的单协议代码范围；本轮用户明确要求截图中的协议选择，保留草稿与账户隔离。
- 复用：[信息卡与工具控制](../../implemented/feature/2026-10-30-playground-model-controls.md) 的工具序列，不执行生成的工具。
- 模型详情模板/企业页用于基础协议及 SDK 用法核对，不替代当前负载导出。

## Alternatives considered

- 只美化现有 HTTP 代码：改动较小，但无法满足截图中的 SDK 与协议选择。
- 直接复用企业页 Hello 示例：代码可运行，但丢失用户当前提示词、参数和工具，不适合工作区。
- 所有字段都透传到其他协议：保留字面值，却容易生成上游拒绝或语义不同的请求；采用显式转换和不兼容字段提示。

## Testing

- 三协议 × Python/JavaScript/cURL × 流式/非流式共 18 次代码对回环 HTTP 服务实际执行；核对路径、鉴权、负载和输出。Python OpenAI 2.48.0 / Anthropic 0.125.0，Node OpenAI 7.45.1 / Anthropic 1.3.0。
- Anthropic SDK 显式指定兼容版本 2023-06-01；新 Node SDK 使用 Bearer，Python/cURL 使用 x-api-key，现有鉴权中间件支持两者。SDK 不进入浏览器依赖，专用运行测试通过环境变量指定安装目录。
- 工作区 14 条测试及账户隔离/SSE 回归通过；覆盖系统/图片、零值、并行工具结果、待回填禁用、跨协议不兼容字段、自定义 Chat 扩展字段及空白示例。
- 浏览器验证高亮转义、复制内容一致、运行命令、三协议、无法转换提示、390px 暗色抽屉无页面横向溢出。17 语言资源与完整前端/文档构建纳入提交检查。

## Consequences

声明协议不等于每个渠道支持所有参数；本地模拟验收不代表真实上游调用成功。SDK 安装不包含在页面复制代码中，运行说明以可展开、可复制的命令提供，明确适用 macOS / Linux / WSL。思考内容、音频及无法保真映射的扩展参数仍需使用 Chat Completions，转换失败不影响原聊天请求。流式示例输出完整事件，保留工具调用与思考数据。
