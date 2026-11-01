# Agent Note: Luna 全站售价统一为官方价乘 33

Status: implemented

## Problem

用户将 Luna 的销售定价改为官方美元价 ×6.6×5。GPT-5.6 Luna 与 GPT-6 Luna 当前仍沿用此前站内价倍乘的阶梯，两个 Pro 已在 OpenRouter 接入时按官方价 ×33 配置；输入、输出、缓存写入及长上下文的差距并非相同倍数，不能整体除常数。

## Decision

复用现有 options 管理 API 和原子 tiered_bundle。覆盖全站四个精确型号 gpt-5.6-luna、gpt-6-luna 及其 -pro；逐分项使用当前官方标准 API 价乘 33，保留 272000 阈值和原档名。两个 Pro 数值已符合要求，只核验；一次更新两个普通型号的表达式。不把匹配字符串 luna 的第三方 Lunaris 归为 OpenAI。现有分组折扣与渠道共享同一模型价，范围不只渠道 1640。

## Existing notes audit

- [OpenAI 原价乘 6.6](../../implemented/process/2026-11-01-openai-retail-multiply-66.md)：本次仅取代其中 Luna 的旧售价决定，其他型号继续沿用。
- [OpenRouter OpenAI 接入](../../implemented/process/2026-11-01-openrouter-openai-enterprise.md)：两项旧 Luna 价格不再保留，Pro 定价已符合；不改其路由、采购上限或分组。
- [缓存写入计量](../../implemented/bug-fix/2026-11-01-openai-cache-write-metering.md)：复用生产 v1.11.14 默认 30m 的 cc 计量；不借用 Claude cc1h 表示 12h，不放开既有未审核时长限制。

## Alternatives considered

- 将当前价整体缩放最省配置，但旧长上下文输出与缓存写入比例不同于官方，不能得到准确结果；逐分项重新计算。
- 只改 1640 无法统一全站同名模型价格，而且现有计费就是模型共享定价；在现有全局表达式中更新。
- 四个型号全部重写会造成无效变动；只改数值有差异的两个普通型号，Pro 逐项验证即可。

## Verification

官方标准价与长上下文分项重新核实；实际 Go 结算通过 108 项检查，前端解析验证四型号两档四分项。生产已原子更新两个表达式，四型号数据库/公开目录读回一致，非目标选项、分组、渠道和元数据保持。6 笔真实请求核对内容、单条消费日志及令牌扣费，两个普通型号均有实际缓存写入/读取证据。初次 Chat 未报告写入量、补测 implicit 参数上游 400 均保留记录；原生 Responses 默认缓存补测成功，失败请求零扣费，全部测试令牌删除。详见[配置与验收](../../../../docs/channel/luna-retail-33.md)。仅配置和审计文档，生产继续 v1.11.14，无新版本发布。

## Consequences

模型基础售价会下降，最终账单还乘请求所属分组/用户/时段倍率。12h 缓存、长上下文真实上游、全部渠道能力不由短请求证明；历史账单不追溯，在途请求沿用计费快照。
