# Agent Note: 钱包恢复经典双栏布局

Status: implemented

## Problem

用户确认 compat.108 的蓝色账户统计、深绿色邀请统计与双栏充值布局更符合预期；新版横向余额区和最近交易拉长页面，用户明确要求恢复已展示的旧界面。

## Decision

复用当前钱包请求和弹窗，仅迁移展示：左侧恢复账户充值标题、账单/发票、蓝色三项统计和充值档位，右侧依次保留待发放、邀请与兑换码；创建兑换码回到兑换区并显示可创建额度。统计作为充值卡封面常驻，套餐默认充值及切换保持；失败不回显旧余额，创建入口保持加载/错误禁用，刷新仍读取 user/self 和订阅状态。

移除首页最近五条支付记录及其独立请求，完整账单继续由原弹窗提供。保留报价/支付同步防重、身份隔离和同 request_id 的不确定兑换重试，未修改后端资金、权限和数据库。

## Existing capability review

- `/console/topup` 经用户路由进入 TopUp；`router/api-router.go` 的 UserAuth 提供 self、topup/info、amount/pay、redemptions。`controller.CreateSelfRedemption` 按登录用户调用 `model.CreateWalletFundedRedemption`，后者保留事务与 request_id 幂等。
- `TopupHistoryModal` 继续承担完整支付记录、发票、提现及管理员对账，`controller.GetUserPaymentRecords` 按登录用户限制记录；不另建查询服务。
- [钱包层级](../../implemented/feature/2026-10-28-wallet-experience.md)：部分替代其横向余额和首页最近交易决定；保留资金操作/身份隔离。旧理由是余额可见性与辅助功能分层，本次通过蓝色统计常驻、兑换额度在创建处展示承接。
- [旧双栏](../../implemented/feature/2026-10-21-pricing-wallet-layout.md)：部分重叠，恢复其视觉和到账/实付语义；模型广场无关。
- [加载恢复](../../implemented/feature/2026-10-28-ordinary-user-ui-hierarchy.md)：部分重叠，继续保留充值失败重试和兑换输入独立；令牌/模型无关。其他命中的计费、视频、邀请推广笔记未改变其业务规则。

## Alternatives considered

- 整体恢复 compat.108 文件可以最快获得旧外观，但同时丢失之后的支付与兑换码重试修复，不采用。
- 只给新版顶部余额加蓝色背景改动较小，但仍保留用户不喜欢的长页面结构，无法恢复已确认截图。
- 采用当前组件迁移布局与旧配色，保留行为修复；移除首页摘要后查看交易需进入账单。

## Verification

Playwright 使用合成 API 检查 1440px 桌面双栏对齐、390px 手机以及实际主题配置的浅色/深色显示，无横向溢出；余额100、可创建额度20独立显示，充值确认金额、创建兑换码弹窗、普通用户账单 self API 与发票入口正常。余额读取503隐藏旧余额并禁用创建，刷新可恢复；topup/info 503时兑换输入可用，重试恢复充值表单；有套餐时默认充值，切换套餐后蓝色统计仍可见。管理员账单请求管理接口并保留发票、提现和对账页签；浏览器无未捕获脚本错误，未发支付或兑换写请求。

既有 payment-navigation、payment-record-view、wallet-payment-request、wallet-redemption-request 共12项Node测试通过（跳转包含桌面/手机10条网关路径），paymentCurrency 4项通过。修改JSX的ESLint/Prettier、主站完整Bun构建和Chrome预渲染通过；Bun构建最初因默认Chromium路径不存在跳过预渲染，随后指定本机Chrome独立执行全部5个公开页面成功。保留既有大包、Browserslist及第三方eval构建提示。

## Consequences

经典卡片恢复用户确认的外观；刷新、未知余额及可创建额度提示使展示比历史截图多保留少量必要信息。交易摘要需打开完整账单查看，减少首页请求；不改变完整记录的筛选、分页或权限。手机采用单栏，原充值、邀请、兑换顺序保持。代码回归与合成API不能证明真实支付到账，发布不等于生产升级。
