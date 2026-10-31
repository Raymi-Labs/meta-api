# Agent Note: Claude 工具参数数字保真

Status: implemented

## Problem

Chat 工具历史转 Claude，以及 Chat/Responses 非流式结果转 Claude 时，把 arguments 解码到 any/map 会产生 float64，改变超过 2^53 的整数和精确小数。SendMessage 参数清洗发生重编码时也有同一问题。生产镜像对应 2d4a3358d；2026-10-31 最近一万条日志中，Claude 渠道有 262 条成功 Chat 请求，证明请求转换路径正在使用。日志不保存工具正文，无法据此计算历史损坏次数。

## Decision

参考上游 4d962e464 的数字保真约束，复用 common.DecodeJsonUseNumberStrict，在现有四个解码边界保留 json.Number。保留 map 形状以兼容 SendMessage 条件清洗；非法 JSON、空值、非对象输入继续沿用各入口既有回退。流式原样传递参数的路径不额外解析。

## Existing notes audit

[Schema 保真](../../implemented/bug-fix/2026-10-24-schema-order-preservation.md)部分重叠但管理的是工具定义，不是运行时 arguments；保留并互链。[Claude 流终态](../../implemented/bug-fix/2026-10-31-claude-stream-terminal.md)仅涉及流结束与结算，不改变。没有应替换的活跃提案。

## Alternatives considered

- RawMessage 完整透传能额外保留对象顺序，但本地 SendMessage 规则需要删除条件不允许的 reason，直接替换输入类型会绕过清洗。
- 只移植上游响应修复改动最少，但同一工具在回填 Chat 历史或 Responses 清洗时仍会损坏；统一已识别的 Claude 边界。
- 等待具体线上损坏投诉可避免修未触发路径，但当前生产确有转换流量，静默修改工具目标 ID 比显式失败更难诊断；使用同版本确定性回归验证缺陷，不采集用户正文。

## Verification

- 修复前四个边界均复现 9007199254740993 被改成 9007199254740992；修复后序列化的大整数、嵌套数组、精确小数及指数原值一致。
- SendMessage 允许字段保留且条件 reason 仍删除；非法 JSON 和尾随第二个值不被接受。service、Claude、OpenAI 全包测试及新增转换/清洗定向竞态回归通过。

## Consequences

输入 map 的数字动态类型由 float64 变为 json.Number；边界下游只读 bool/string 或重编码，需用既有回归验证。没有数据库、价格、渠道配置或生产部署变更；不追溯修正历史请求。
