# Agent Note: Stripe 余额充值入口显示名称与图标

Status: implemented

## Problem

余额充值自动添加的支付入口固定显示 Stripe，管理员无法在 Stripe 设置内使用用户熟悉的名称与图标。PayMethods JSON 虽可添加 type=stripe 的自定义名称，但充值按钮与确认弹窗各自判定图标，且设置入口分散。

## Decision

复用超级管理员 option 存储和登录用户 topup/info，在 Stripe 设置中增加 StripeDisplayName 与 StripeDisplayIcon。只覆盖余额充值入口的展示，保持 type=stripe、下单路由、金额、回调和历史账单归属。空设置延续已有 PayMethods 自定义内容，再回退 Stripe；输出时复制支付方式，不改共享配置。名称限制 40 字符并拒绝控制字符；图标使用固定枚举，不新增外部图片地址。

充值按钮和确认弹窗复用图标组件，后台提供预览。套餐是 Stripe subscription mode，微信不支持此模式，因此不把余额充值的显示设置套到套餐购买入口。Stripe Dashboard 的支付方式决定实际可用性，展示名称不启用或强制微信；设置页明确说明。

## Existing decisions

检索活跃笔记命中的易支付签名、多网络设置属于其他网关，与此配置无重叠。当前 Stripe Checkout 使用动态支付方式；不修改 SDK、Stripe 账户配置或结账参数。

## Alternatives considered

- 直接编辑 PayMethods JSON：已有名称能力，但管理员需要了解内部类型，且图标和确认页仍不统一。
- 把 stripe 类型改成 wxpay：可以复用微信图标，但会改变支付路由与回调身份，因此保留内部类型并独立配置展示。
- 根据名称自动判断图标：少一个设置，但多语言和任意自定义名称无法可靠判定，采用明确选择。

## Testing

Go 回归及定向竞态检查覆盖保存与重新读取、空值回退、旧 JSON 兼容、非法输入不写库、禁用状态、响应不泄密和不修改共享数据。真实本地服务与浏览器覆盖后台预览、保存及刷新、普通用户权限、充值与确认、390px 暗色、保存失败保留草稿；拦截支付请求验证路径仍为 /api/user/stripe/pay 且 payment_method=stripe，不发起真实付款。前端 ESLint、格式、生产构建及五个公开页预渲染、make verify-notes 通过。

## Consequences

管理员需要先确保 Stripe Checkout 实际提供所展示的支付方式；仅改名不能证明微信支付可用。保留历史账单 Stripe 标识，避免将未知真实支付方式改写成微信。
