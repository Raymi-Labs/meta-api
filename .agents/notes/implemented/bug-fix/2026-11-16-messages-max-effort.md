# Agent Note: Messages 最高推理档位的显式适配

Status: implemented

## Problem

Messages 转 OpenAI 的全局映射默认 max→xhigh，且管理端和运行时均不接受 max 作为目标。已有精确渠道规则可保留 max，但仅覆盖 Chat；Messages→Responses 无法使用该规则。截图只能证明日志值，不能证明原始请求或上游内部执行强度。

## Decision

扩展已有精确模型规则至 Responses，保留 Chat 专属 GLM 模板边界。原生 Chat/Responses 和 Messages 转换复用规则；原始 Messages effort 在全局转换前匹配，实际协议决定规则。全局映射允许 max 但不改变旧默认。规则只按管理员明确选择生效，不按 GPT 前缀推测能力。禁用思考优先，省略不补；预算沿用已有桶语义，只有显式配置规则才改变桶映射。参数覆盖最后执行，最终出站日志及管理诊断一致。

## Existing decisions

[渠道思考参数适配](../../implemented/feature/2026-10-25-channel-reasoning-profiles.md)部分重叠：延伸精确规则协议边界，保留 GLM Chat 模板及旧配置。其他性能、计费和工具 Schema 笔记与此决定无关。

## Alternatives considered

- 全局直接 max→max：修复简单，但旧模型/渠道可能拒绝，且预算桶不证明能力，不采用。
- 只改日志显示：无法修复真实请求，且误导用户，不采用。
- 增加独立 GPT 开关/按名称自动猜测能力：与已有精确模型规则重复，不能证明渠道支持，不采用。

## Testing

先运行定向回归，明确复现 Responses 精确规则输出 xhigh 而不是 max、全局 max 目标不生效及管理校验拒绝三个失败，再修复。

`go test ./dto ./service ./controller ./relay ./relay/channel/openai -count=1` 全部通过。新增 HTTP/SSE 矩阵覆盖三入口、两种出站协议，共94个组合：max 保留、显式 xhigh、旧配置、错误协议规则隔离、后置覆盖/删除、缺省、上游400单次失败与原生透传；增加非幂等规则验证同一请求不会重复映射。服务层覆盖 disabled、思考预算来源、模型精确匹配、Responses→Chat 回退诊断清理和普通日志值。既有工具历史/模板/失败日志/尝试清理测试随包回归运行。

前端渠道规则及17语言资源测试21项通过；两个改动组件的 Bun bundle 和 Prettier 校验通过。最初缺少前端依赖，按已有锁文件安装后重新运行通过，没有修改依赖清单或锁文件。这里只进行了本地模拟上游和组件编译，未进行真实上游付费调用或生产部署。

## Consequences

max 是否受具体上游支持必须独立验证；不进行收费测试或生产配置修改。部署后需为已验证渠道配置精确模型规则才能改变旧行为。全局 max 配置影响全部继承者，推荐渠道规则。
