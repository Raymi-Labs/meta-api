# Agent Note: 请求 Schema 属性顺序与原始数字保真

Status: implemented

## Problem

客户端工具参数和结构化输出的嵌套 Schema 会经 DTO、协议转换、禁用字段过滤和参数覆盖解码成 map；重编码排序属性，并可能把大整数转成 float64。上游依赖属性顺序生成输出时，网关不应无意改变该顺序。

## Decision

沿用本地协议转换与管理员覆盖规则，只把需要透传的嵌套 Schema 保留为 RawMessage；确实需要读写 Schema 的 provider 用局部 raw 对象访问。move/copy/数组追加保留原有值的 raw 字节；禁用字段只解析顶层和目标子对象。保留缺失/null、未知工具过滤、thinking、可信身份头与现有显式覆盖优先级。不引入 RelayKit 或新前端。

- DTO 的 FunctionRequest.Parameters、Claude Tool.InputSchema、FormatJsonSchema.Schema 和 Gemini responseSchema 接受既有程序构造的 map，但客户端 JSON 解码后保存为 RawMessage。ClaudeRequest.Tools 整体和 Gemini functionDeclarations 在进入 any 字段前保留 raw；读取工具时只解析外层元数据。Claude input_schema 的对象校验及可选 null 语义不放宽。
- Chat→Claude 补齐缺失的 type/properties 时用 raw 局部写入；Chat→Responses 展平格式外壳但保留内部 schema；Claude/Gemini→OpenAI 通过原有转换函数携带 raw 参数。Token 估算读取 raw JSON 文本，不把字节数组格式当成工具参数。
- Gemini 继续执行已有支持字段白名单、type/nullable 规范化、tuple items 取首项、工具 Schema 64 层和输出 Schema 5 层限制；raw 清洗按输入字段顺序重建对象，不转 float64。程序构造的 map 清洗路径保留。
- AWS 格式化和透传体保留 raw tools，tools:null 仍省略；Ollama Chat/Generate 共用转换函数，将 json_schema 外壳中的 schema 原文传给 format，不再误传 name/schema 包装层。
- 通用禁用字段过滤只重编码顶层及需修改的 stream_options，Gemini functionResponse.id 清理由精确路径删除；参数 move/copy 与数组 append/prepend 保留现存元素的 raw 值。管理员新写入值仍遵循现有覆盖规则，不用原始请求反向覆盖管理员设置。

## Existing notes audit

[DeepSeek 原生 Responses](../../implemented/bug-fix/2026-10-18-deepseek-native-responses.md)部分重叠，保留最终出站与大整数约束；[历史 thinking 兼容](../../implemented/bug-fix/2026-10-23-deepseek-messages-thinking-history.md)部分重叠，保持有限重试与有效历史；[认证用户身份头](../../implemented/feature/2026-10-21-authenticated-user-header.md)部分重叠但独立管理出站头信任，不改变其规则。PITR/发布的数据库 schema 笔记无关，没有应替换或否决的活跃提案。

## Alternatives considered

- 整体移植上游 RelayKit 可获得统一转换实现，但会替换本地资金、工具与历史兼容边界，范围远大于保真修复。
- 只在最终发送前恢复原始 tools/format 能少改 DTO，但会覆盖已授权管理员参数修改和跨协议转换，不可用。
- 只修 DTO 最简单，但后置禁用字段/覆盖操作仍会排序，必须覆盖整个实际发送链。
- 把公开 DTO 字段全部改成 RawMessage 可强制单一表示，但会破坏现有适配器和测试中程序构造的 map；选择边界自定义解码，保留两种表示，并为必须读 Schema 的适配器显式分派。

## Verification

- DTO、Claude/DeepSeek/Gemini/AWS/Ollama 和 openaicompat 全包普通测试通过；新增断言对比嵌套 reasoning→answer 原始顺序及整数 9007199254740993，而非仅对比解码后的结构。
- `go test -race -p 2 -timeout 180s ./dto ./relay/channel/claude ./relay/channel/deepseek ./relay/channel/aws ./relay/channel/ollama ./service/openaicompat -count=1` 通过。
- `go test -race -parallel 1 -timeout 180s ./relay/channel/gemini -count=1` 通过，覆盖清洗规则和深度截断。默认并行 -race 暴露既有 relay_gemini_usage_test.go 多个测试同时 gin.SetMode 的全局竞态；本次没有更改那些测试，也不宣称该包默认并行 race 已通过。
- service 定向竞态回归覆盖 Claude/Gemini→OpenAI→Responses、既有协议转换和通知限流；原生请求、null/缺失、无效 input_schema/legacy functions、AWS 透传以及 Ollama 两个入口均有回归。
- relay/common 的独立回归覆盖禁用字段、Gemini 工具响应 id、move/copy/append/prepend 及字符串、布尔、null、大整数原值。`controller/relay_schema_payload_test.go` 使用真实本地 HTTP 上游，覆盖完整 Relay/DeepCopy/过滤/参数覆盖/Chat→Responses 最终出站；精确比对两个 Schema 的原始字节，确认覆盖仍生效且仅发一次请求，定向 race 通过。隔离暂存快照的 Go 全量及相关包定向 race 通过，不依赖并行开发的额度提醒代码。

## Consequences

Schema 的属性顺序和数字不再因 DTO/已覆盖转换链的无关读写丢失，代价是部分 any 字段可能持有 map 或 RawMessage，新增读取方必须兼容两种表示；Gemini 维护 raw 与 map 清洗路径的行为一致性。未来需要统一有序对象模型时再评估替换，而不是在局部适配器随意解码整个 Schema。

不把原本不支持的 Schema 声称为支持：保真不保证供应商执行属性顺序，Gemini 的必要清洗和深度边界仍会有意改变不支持的字段；Ollama 只处理已有 json/json_schema 模式，不新增模型能力。没有调整模型/价格、渠道、数据库、生产服务或用户原有额度提醒改动。
