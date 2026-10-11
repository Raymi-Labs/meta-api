# Agent Note: 通过易支付启用 USDT / TRC20

Status: implemented

币种范围由[稳定币多网络配置](2026-10-09-epay-crypto-networks.md)扩展；下文单 TRC20 为首期约束，PayMethods、CNY 与 Epay 回调复用决定保持有效。

签名分隔符拒绝与套餐网关展示名称的兼容边界见[定向安全修复](../bug-fix/2026-10-12-upstream-security-static-compat.md)，不改变本篇订单和到账契约。

## Problem

Epay 项目已提供商户自配 BEpusdt 通道。业务站以 CNY 下单，支付类型为 `usdt.trc20`，到账后仍收到标准易支付签名通知。new-api 已支持自定义 PayMethods，但管理员只能编辑 JSON，付款提示与支付记录筛选没有覆盖此方式。

## Decision

在易支付设置增加默认关闭的 USDT / TRC20 开关，以现有 `PayMethods` 为唯一配置来源；保留其他方式和已有自定义字段。充值与套餐继续使用当前 Epay 地址、PID、密钥和人民币计价、订单、回调与入账流程。new-api 不保存 BEpusdt Token、钱包地址或私钥，也不独立换算币额或监听链上事件。充值确认与套餐购买说明实际网络和精确币额以收银台为准，支付记录增加可读标签与筛选。

## Capability audit

- 本地 Epay `docs/BEPUSDT_SUBSCRIPTION.md`、`includes/lib/BepusdtGateway.php` 与 `CollectionNotify.php` 确认 CNY 订单及标准易支付通知契约，直接收款和链上校验由 Epay/BEpusdt 承担。
- new-api 的 SettingsPaymentGateway、topup/index、SubscriptionPlansCard 通过 PayMethods 共享支付入口；登录路由连接 RequestEpay / SubscriptionRequestEpay，订单来源为 TopUp / SubscriptionOrder。金额报价为 CNY，服务端按配置检查支付类型。
- ValidateTopUpCallback / ValidateSubscriptionCallback 验证业务金额和支付提供方；数据库事务避免重复入账，历史订单不依赖当前 PayMethods。
- 活跃笔记中的渠道、令牌、协议转换、计费与流程记录均无本次支付配置的归属；同步远端后再次检索 Epay / PayMethods / BEpusdt / USDT，仅命中本篇，无需迁移或覆盖其他笔记。

## Alternatives considered

- 只提供 JSON 示例：已有能力足够下单，但缺乏明确配置入口和网络提示，管理员容易误填 `usdt` 或把 BEpusdt Token 当商户密钥。
- 新建独立 BEpusdt 支付提供方：可绕过 Epay，但重复网关配置、签名、扫链/回调和商户权益边界，与用户现有方案不符。
- 新增独立后端开关：容易与 PayMethods 出现双重状态；直接编辑既有配置可保持旧接口和手工配置兼容。

## Testing

- `bun test src/helpers/epayMethods.test.js src/helpers/paymentCurrency.test.js`：14 项通过，覆盖保留其他方式、自定义字段、精确类型、重复启用、空值、非法结构和币种格式。
- `go test ./controller ./model ./service`：通过。USDT 回归覆盖充值及套餐人民币下单、非法签名/金额拒绝、重复通知只发放一次、关闭不阻断在途订单，以及记录筛选/分页/用户隔离。
- 独立 SQLite 与合成账号的真实页面验收：管理员保存、刷新、非法 JSON、后端不可用时保存失败及恢复；普通用户充值和套餐通过本地 Epay 协议桩完成签名通知，余额为 $2、支付记录为 ¥14.60，套餐 ¥7.30 且仅 1 条生效；关闭后两个购买入口移除此方式，支付宝保留。
- 普通用户读写 `/api/option/` 均返回权限不足。桌面与 390px 暗色下配置区和套餐提示无溢出。页面验收发现 Semi 表单复用 values 引用造成开关显示滞后，改为拷贝快照后手动 JSON 和开关保持同步。
- 定向 ESLint、Bun 生产构建与 Chromium 公开页预渲染、版本化 Go 构建通过；提交前执行 `make verify-notes`。无数据库结构或生产配置改动。

## Consequences

复用现有订单和回调可以同时覆盖充值与套餐，不产生第二套密钥或付款状态。代价是依赖商户先部署、测试并启用 Epay / BEpusdt 通道，new-api 开关本身无法判定链上就绪。

本地协议桩和页面验收无法证明真实钱包到账。上线后仍需商户先完成 Epay 的到账测试，再做业务站小额付款验收。CNY 支付记录不应伪装为 USDT 金额或补造链上哈希；首期配置不代表支持 USDC 或其他网络，扩展范围见上方后续决定。版本发布与生产部署分离。
