# Agent Note: Gemini 兼容向量输入整批校验

Status: implemented

## Problem

OpenAI 兼容 `/v1/embeddings` 转 Gemini 时，共享 `EmbeddingRequest.ParseInput` 只保留字符串，混合数组中的对象、数字会被静默删除，输出索引与原始输入不再对应。Gemini 转换器对缺失/空输入返回裸 error，经通用错误包装后成为 500。通用 Relay 在 provider 转换前可能已预扣，不能把转换器前置拒绝等同于从未发生预扣。

## Decision

只在 Gemini `ConvertEmbeddingRequest` 增加完整文本输入校验：接受非空字符串及非空纯字符串数组，拒绝任何空字符串项、对象、数字、布尔、null 和嵌套数组，既不删项也不改写合法文本。失败返回 `invalid_request` HTTP 400 和 skip-retry，再复用通用失败退款；不发送上游、不产生最终消费。保留共享 DTO 的 ParseInput，以免改变 OpenAI token-ID 等其他 provider 的契约。

通用 `GetAndValidateEmbeddingRequest` 在进入适配器前拒绝 null/缺失 input；同一空值判断补齐请求指针保护并返回 typed 400、skip-retry，而不是裸 error 导致 500 或空指针。此错误格式修正影响所有 embedding provider 的空输入，但不限制其他输入类型；一维/二维 OpenAI token-ID 数组仍按原契约通过校验与适配器。

原生 Gemini 请求不调用兼容转换器，本次不改变其 content.parts 契约；`embedContent` 的 `GeminiEmbeddingHandler` 路径有回归，原生 `batchEmbedContents` 的既有控制器分派另见验证边界。合法兼容输入仍生成 batch 请求，保留模型映射、维度、顺序和重复项。

## Existing notes audit

活跃笔记中没有 embedding 输入校验的已有归属。[Schema 保真](../../implemented/bug-fix/2026-10-24-schema-order-preservation.md)涉及 Gemini 但管理工具/结构化输出 Schema，不覆盖 embedding；本次不修改其决定。任务资金/下载凭据笔记与本次文本向量接口无关；使用日志导出中的输入校验也无关，没有应替换或否决的提案。

## Alternatives considered

- 修改共享 ParseInput 能集中拒绝非法输入，但该 DTO 服务多个 provider，OpenAI token-ID 输入与未来多模态向量契约不等同于 Gemini 文本转换，拒绝扩大为全局限制。
- 只拒绝 map 内容块能最小照搬上游安全子集，但仍会丢弃混合数字、布尔、null 或数组；选择验证每一项并保留整批原顺序。
- 交给 Gemini 校验可复用供应商规则，但现有转换已经删掉不支持的项，上游看不到原始错误，不能保护输入/输出对齐。

## Verification

- `embedding_input_test.go` 覆盖字符串、解码后的纯字符串数组及程序构造的字符串数组，断言文本原值、顺序、重复项和维度。混合文本/对象、数字、布尔、null、嵌套数组、纯对象、空值及含空项数组全部返回 400、invalid_request、skip-retry，没有部分转换结果；既有通用错误包装保留 typed error，重试判定明确拒绝。
- `relay_gemini_embedding_test.go` 使用真实本地 HTTP 上游与隔离 SQLite 资金记录，钱包/订阅/token-only 均实际观察到 token 预扣，随后非法输入零上游请求、最终余额完全恢复、无消费日志。合法字符串与三条数组保留数量和出站文本；原生 Gemini 单条 embedding 保留 taskType、维度并正常转发。
- `embedding_input_validation_test.go` 验证顶层 null、缺失 input 与 input:null 返回 400；一维/二维 OpenAI token-ID 数组通过原校验和 OpenAI 转换器后保持原值。
- `go test ./relay/channel/gemini ./relay/helper -count=1 -timeout 120s` 通过。
- `go test ./relay/channel/gemini ./relay/helper ./controller -run 'TestGeminiEmbedding|TestEmbeddingMissingInput|TestEmbeddingValidationPreserves|TestRelayGeminiEmbedding' -count=1 -timeout 120s` 通过。
- `go test -race -parallel 1 ./relay/channel/gemini ./relay/helper ./controller -run 'TestGeminiEmbedding|TestEmbeddingMissingInput|TestEmbeddingValidationPreserves|TestRelayGeminiEmbedding' -count=1 -timeout 120s` 通过。

## Consequences

Gemini 不再静默删项或把输入错误报成内部错误，输入/输出对应关系得到保护；代价是此前截断后“成功”的错误请求现在明确失败，客户端需要修正输入。仅空字符串拒绝，空白字符原样保留，不擅自清洗用户文本。共享校验只改变空请求/缺失输入的错误格式，不改其他 provider 的有效输入。

转换发生于通用预扣之后，依赖既有退款机制，不承诺资金处理与进程崩溃具有原子性。原生 Gemini content.parts 不经过本转换器，单条入口有回归；原生 batch 的既有控制器路径分派不在本次修复中，不声称完整原生 batch 验收。原生多模态能力和其他 provider 不在本次范围；若 Gemini 转换需要新增多模态能力，必须另行设计而非恢复过滤不支持项。测试不证明真实供应商的向量质量或可用性。
