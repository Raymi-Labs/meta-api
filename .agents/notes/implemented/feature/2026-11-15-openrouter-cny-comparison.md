# Agent Note: OpenRouter 区间消耗人民币对照

Status: implemented

## Problem

统一看板的区间消费分别以USD和闪电显示，用户要求可直接对照的人民币展示，明确本站闪电除以5，并确认上游美元固定乘6.7。线上站点usd_exchange_rate为1，不能静默复用为人民币汇率。

## Decision

只改变区间消费展示：USD金额及BYOK乘6.7，本站原始quota先按quota_per_unit和custom_currency_exchange_rate还原闪电，再除以5。卡片、渠道/模型/日期表、CSV同口径，六位小数只在最终格式化；未知或无效尺度仍显示未知。页面明确固定比例，既有账户累计USD维持原币。后台快照、财务账本、全站汇率、扣费和路由不修改。

## Existing notes audit

[统一看板](../../implemented/simplification/2026-11-15-unify-openrouter-usage.md)部分取代区间显示口径；其快照、筛选、归属与合计不变量保留。[账户财务](../../implemented/feature/2026-11-02-openrouter-account-finance.md)保留原始USD来源和累计口径，区间显示由本篇接续。其他命中的钱包支付、模型官方价格、体验页换算用于不同业务，不变更。

## Alternatives considered

- 复用全站usd_exchange_rate：已有设置方便，但现场值为1，用户明确要求6.7；不能以1换算或全站改汇率影响无关页面。
- 只修改顶部卡片：改动少，但表格及导出仍使用旧单位易误比较，所以区间全部金额一起换算。
- 直接将quota除以5：忽略内部quota与显示闪电的倍率，金额会相差500000倍；必须先还原闪电再换人民币。

## Verification

用户示例金额、零/未知值、非默认尺度、无效参数与最终精度回归；三维度及CSV一致，账户累计仍USD，页面解释比例。Bun换算回归及两组真实Chrome回归通过，覆盖分页/搜索/三维度/导出、错误保留及账号隔离；截图检查桌面和手机人民币展示。构建、笔记和发布校验随提交执行，生产部署独立验收。

## Consequences

6.7是用户指定固定对照汇率，不宣称市场实时汇率或实际结算汇率；按5闪电折算的消费不代表真实净到账或现金利润。改变区间CSV单位需写入兼容说明。
