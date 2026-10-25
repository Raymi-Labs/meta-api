# Agent Note: 按渠道厂商与模型选择思考参数模板

Status: implemented

## Problem

Messages 请求先经过全局 ClaudeToOpenAIReasoningMap，high/max 都可能成为 xhigh，再用渠道参数覆盖已无法恢复原档位。现场 GetToken 的 glm-5.3 Chat 接口明确只接受 low/high/max；该证据不代表所有 GLM 型号或协议。展示厂商原先仅用于分类，不能让修改展示分类隐式改变所有旧渠道请求。

## Decision

复用渠道 setting JSON、管理员渠道编辑/校验、选渠上下文和既有协议转换。新增 reasoning_adaptation：旧配置缺省 inherit 保留全局映射；新建表单默认 auto，管理员可在既有渠道显式选择 auto、固定 zhipu-glm53-v1 模板或 inherit。按精确上游模型和实际出站协议匹配，自定义规则优先于模板；自动模式使用既有保守厂商解析，只有 zhipu + glm-5.3 + Chat 命中首个模板。Flash 等未知模型不自动登记，管理员可为已验收模型添加精确规则。

在原始 output_config.effort 或 thinking.budget_tokens 档位进入全局映射之前适配。内置模板 low→low、medium→high、high→high、max/xhigh→max、minimal→low；这是一项明示兼容策略，不宣称不同厂商档位等价。仅实现 Chat 协议规则；原生 Messages、Responses 和 Chat→Responses 桥不套用 Chat 模板。直连 Chat 在既有 OpenAI 适配器中处理对应别名。透传保持原样，省略不补字段，显式 disabled/none 不通过档位映射重新开启。已有参数覆盖最后执行并保持优先；最终诊断在覆盖之后采集，重试时清空前一渠道状态。

诊断仅记录有界档位、模板和模型信息，普通用户隐藏管理信息。明确的上游 reasoning_effort 枚举拒绝给出专用提示，不扩大 400 重试。没有 schema 迁移、价格变更或生产配置写入。

## Existing decisions

检索 ClaudeToOpenAIReasoningMap、思考/推理映射、展示厂商和参数覆盖。没有对应映射笔记；[厂商展示](../../implemented/feature/2026-10-21-inception-vendor.md)部分重叠，保持分类本身不影响路由，只有显式适配模式消费其结果；[Schema 保真](../../implemented/bug-fix/2026-10-24-schema-order-preservation.md)部分重叠，保留后置覆盖和原始 Schema；[Claude 参数修复](../../implemented/bug-fix/2026-10-19-claude-sampling-compatibility.md)部分相邻，不新增自愈或重放。模型广场、钱包分类、身份头和缓存计费命中与本决定无关。

## Alternatives considered

- 继续每渠道粘贴六条参数覆盖可立即止错，但不能区分原始 high/max，也难维护；保留为迁移期间显式最高优先级。
- 按 glm 前缀强制全量映射维护简单，但 Flash/旧模型和原生协议没有能力证据，拒绝。
- 自动从任意 400 学习或重发可减少人工配置，但会把单渠道拒绝扩散为全模型能力并增加调用，拒绝。

## Testing

Go 全量测试通过；专项与竞态检查覆盖原始档位/预算、旧配置、自动/固定模板、精确模型、规则优先级、未知型号、透传、缺省/禁用、后置覆盖和尝试间诊断隔离。控制器本地 HTTP/SSE 回归覆盖 Messages/Chat、映射别名、工具历史、上游 400、参数覆盖与透传；额外验证 Messages/Chat → Responses 实际出站不套用 Chat 模板。失败记录有专用原因并保持单次请求，不增加 400 重试。

前端 helper 回归、17 语言回归、静态检查、主站/独立文档构建和五页预渲染通过。Chrome 使用真实渠道编辑组件和隔离 API fixture，验收旧配置继承、自动模式与精确规则保存/重开、未知 setting 字段保留、非法规则不提交、服务端保存失败保留输入后重试、新建默认自动模式，以及 390px 暗色显示。

本地 HTTP fixture 使用免费倍率，不作为付费上游计费实测；既有计费回归随全量 Go 测试运行。没有执行生产上游请求、数据库变更或部署。渠道 CRUD 的权限边界复用原管理员路由，诊断复用 admin_info 脱敏。

## Consequences

原始 high/max 不再因全局映射合并而丢失，渠道可复用已登记模板，仍可显式覆盖。代价是模板必须按真实接口维护，不能承诺厂商未来所有模型永久兼容；增加模板或协议应以独立能力证据和最终出站测试为前提。

medium→high 可能增加推理成本。旧渠道需主动启用；API 创建且省略配置仍继承原行为。旧手写参数覆盖仍能改变模板结果，不能自动删除用户规则。部署前备份渠道配置并分档位、流式与工具多轮验收；应用更新/回退沿用受保护入口。
