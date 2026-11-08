# Agent Note: 退役 Meta API 对 Pulse 消费贡献的专用支持

Status: implemented

后续[板块清理决定](2026-11-08-remove-pulse-panels.md)替代本文保留页面与 BFF 的决定；资金及历史数据边界仍适用。

## Problem

Meta Pulse v0.2.88 已停用 API 消费摄入和贡献权益，但 Meta API 仍强制保留消费日志，并继续生成用于 Pulse 付费消费映射的日志证明。用户页和运营页仍展示消费贡献、等级和摄入状态，造成权益已停用而产品侧仍暗示有效。移除这些专用依赖时不能影响 API 计费/退款、资金来源审计、现金核算、社区绑定或奖励结算恢复。

## Existing capabilities and impact

已核对用户入口 Pulse 页面（后续已删除）、运营入口 PulseOps 页面（后续已删除）、用户/运营 BFF、Pulse 设置页及 Benefit/SSO 路由。Pulse v0.2.88 的用户摘要仍携带历史贡献/等级字段；运营摘要仍返回历史周期、经济规则和摄入游标。BFF 现在分别投影为可用券余额，以及结算重试/死信数量与观测时间。

Pulse BFF、社区 SSO/绑定、Benefit 发放/查询/撤销和奖励幂等凭证继续保留。`PulseFundingLedger`、`PulsePaidQuota`、`PulseWalletReservation` 参与钱包预扣/退款和现金核算，继续保留。Meta API 停止将付费来源证明写入消费日志，并在新写日志时剥离客户端传入的旧 `pulse_funding` 元数据；日志自身、计费退款与资金来源审计继续运行。活跃笔记检索未发现针对 Pulse 消费贡献的既有笔记；既有现金账本笔记确认资金资格账本须保留。

## Decision

Pulse 配置不再包含消费日志门禁。旧 `PULSE_USAGE_LOG_REQUIRED` 环境变量不再被读取；持久配置 bundle 中同名旧字段在加载时跳过，避免旧配置破坏其余 Pulse 设置。Benefit 开关和消费日志开关各自独立。Pulse 用户页仅显示可用券与奖励历史；运营页仅显示结算重试和待恢复死信。BFF 对 Pulse 响应使用字段白名单，不向浏览器透传等级、贡献、周期、摄入游标或消费规则。

消费日志继续按 Meta API 自身开关写入，消费日志的 `Other` 不再包含 Pulse 专用资金证明字段；传入的旧证明字段不会进入新消费/任务日志。资金资格、钱包预留、现金账本和历史日志数据不删除或清空。Benefit、SSO、绑定和待结算奖励恢复接口保持不变。

生产环境日志库读取授权不由本次代码改动回收。必须等待新版 Pulse 部署并确认 Worker 不再连接 `LOG_DB`；如账号被共用，先拆分授权。

## Alternatives considered

- **保留消费日志门禁，依赖 Pulse 停用标志** — 改动较小且日志可供历史分析；但会继续把消费日志错误地当作 Pulse 运行依赖，并使 Benefit 设置影响 Meta API 的日志开关。
- **删除全部 Pulse 内部路由与资金来源模型** — 能缩小更多集成代码；但会破坏社区身份、待结算奖励恢复及钱包现金核算，超出停用消费贡献的范围。

## Consequences

- **收益**：API 消费、退款和补录不再通过 Meta API 日志证明形成 Pulse 消费权益；Meta API 的消费日志策略与 Pulse 奖励解耦；旧摘要不会在 Meta API 用户端或运营端暴露为有效权益。
- **代价与已知上限**：Pulse 自身仍保留停用历史贡献及消费摄入代码/数据，需按其独立发布流程退役；生产日志库权限须在确认 Pulse 新版无连接后另行回收。

## Verification

`go test ./common ./controller ./model ./service` 通过，覆盖消费日志与 Benefit 配置独立、旧配置兼容、消费证明剥离、BFF 字段投影、奖励与资金账本回归。前端 `test:pulse-settings`、`test:site-language`、指定页面 ESLint 和 `bun run build` 通过；预渲染因宿主缺少 `/usr/bin/chromium` 按仓库脚本跳过。`make verify-notes` 通过。
