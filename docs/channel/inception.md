# Inception / Mercury

Inception 是厂商，Mercury 是其模型系列。本站复用通用展示厂商分类，转发协议仍由渠道类型决定。

## 标准聊天接入

Mercury 3 的建议配置：

| 字段 | 值 |
| --- | --- |
| 渠道类型 | OpenAI |
| API 地址 | `https://api.inceptionlabs.ai` |
| 模型 | `mercury-3` |
| 展示厂商 | 自动识别，或手动选择 Inception |

本项目会拼接 `/v1/chat/completions`，API 地址不追加 `/v1`。密钥、可用模型、分组与价格由管理员按实际供应商能力配置。本次分类更新不配置密钥、自动上架模型或写入促销价格。

官方普通聊天接口支持流式用量统计、工具调用和结构化输出。特殊的 `diffusing=true` 返回修订中的完整文本，不能按普通增量内容直接拼接；本次不增加该模式适配。Mercury Decide 的 `/v1/decisions`、Mercury Edit 2 的 FIM/Edit 端点需另行适配和验收，厂商识别不代表这些端点可用。

## 分类规则

- `mercury` 必须是独立名称片段，如 `mercury-3`、`inception/mercury-3`；不会把 `notmercury` 或仅有 `inception` 命名空间的未知模型归入该厂商。
- 渠道自动识别先应用模型映射，仅在全部模型明确属于同一厂商时归类；混合、未知或同时带其他已知厂商片段的模型留在协议分类。手动展示厂商和“跟随渠道类型”优先。
- 令牌与日志按既有分组命名约定识别 `Inception`、`Inception Labs`、`InceptionLabs`、`Mercury`，例如 `Mercury · 优质`；不会修改实际分组标识或权限。
- 模型广场仅对实际可用模型补充默认厂商资料，保留管理员已设置的非零归属、自定义图标和简介。模型规则共享时按具体模型应用展示回退，不污染其他匹配模型。
- 当前图标依赖不含 Inception，使用既有首字母 `I` 回退；不使用其他厂商的标识。

## 验收边界

分类、目录和本地测试不证明上游真实可用。正式接入后应分别核对非流式/流式完成、工具往返、缓存与思考用量及实际扣费；生产渠道配置和部署独立进行。

官方参考（2026-10-21 核对）：[模型与价格](https://docs.inceptionlabs.ai/get-started/models)、[聊天接口](https://docs.inceptionlabs.ai/api-reference/chat/create-a-chat-completion)、[流式与扩散](https://docs.inceptionlabs.ai/capabilities/streaming)。
