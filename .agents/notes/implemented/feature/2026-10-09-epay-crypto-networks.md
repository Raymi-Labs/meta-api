# Agent Note: Epay 稳定币与网络配置

Status: implemented

## Problem

用户需要同时收取 USDT、USDC，并提供多个网络。现有 [单通道接入](../../implemented/feature/2026-10-09-epay-usdt-trc20.md) 仅提供 TRC20 开关和固定付款文案；Epay 的商户账号、地址及路由也有限制，单独增加前端按钮无法使支付可用。

## Decision

沿用 PayMethods、CNY 订单与 Epay 签名回调，提供按币种分组的独立网络开关。首批为 USDT 的 TRON、Ethereum、BSC、Polygon、Arbitrum、Solana，以及 USDC 的 Ethereum、BSC、Polygon、Arbitrum、Base、Solana；不将币种和网络作任意笛卡尔组合。新增组合默认关闭，已有自定义条目保持原值。

Epay 同步扩展已审核类型、对应停用模板、账号网络及地址校验；每个账号绑定一个组合，各自完成到账测试后启用和设置该组合的默认路由。复用一个 BEpusdt 实例的不同网络仍各自占一个账号名额。旧 TRC20 账号、路由与在途订单兼容。

## Related decisions

签名 SDK 升级及历史歧义回调风险见[定向安全修复](../bug-fix/2026-10-12-upstream-security-static-compat.md)；币种范围与 CNY 金额不变。

与单 TRC20 接入笔记部分重叠，本篇扩展币种范围；原 PayMethods 单一来源、CNY 与 Epay 回调边界仍成立。其他活跃笔记中的支付风险/记录不改变，本轮不重构订单提供方。

## Alternatives considered

- 只修改 new-api：已有管理员 Epay 插件能转发多链，但用户采用的商户自配流程仍拒绝，不能覆盖完整用户目标。
- 在收银台任意改币种网络：体验更短，但现有订单快照、币额和地址在创建时确定，切换会使通知匹配失效；保持下单前选择并锁定。
- 网关支持列表全量启用：配置省事，却不能证明钱包、RPC 或具体代币合约已就绪；逐组合验收和显式开启。

## Testing

- 两个仓库的 12 个类型经脚本核对一致，7 份语言文件的新增键及插值一致。前端 27 项测试、95 个断言与定向 ESLint 通过。
- controller/model/service 全包测试和 go vet 通过；所有组合的充值、套餐 CNY 订单和通知幂等通过；USDT、USDC Base / Ethereum 记录筛选及用户隔离通过。
- 本地 SQLite 与合成 Epay 桩：管理员保存/刷新同时保留 TRC20、Ethereum USDT 和 Base USDC；普通用户 USDC Base 充值及 USDT Ethereum 套餐完成签名通知，账单均保留 ¥7.30 和对应类型。390px 暗色套餐弹窗、动态提示已验收；充值提示移动至整行以免在数量栏过度折行。
- Epay 使用隔离 MySQL 完成逐类型配置、测试、默认路由和业务 API 回归；真实项目页面验证错误地址拒绝、保存、Token 不回显及网络不可变。
- Bun 生产构建、Chromium 预渲染、版本化 Go 构建通过。

## Consequences

相同 EVM 地址不代表相同链，BSC 等可能使用跨链版本代币；支持范围以网关实际合约和收银台为准，不能仅按币符号付款。仅本地合成验收不代表链上到账。现有 GitHub Actions 执行限制可能继续阻塞发布，发布与生产部署分离。
