# Agent Note: 合并 OpenRouter 充值与消耗看板

Status: implemented

## Problem

移除现金核实门槛后，现金收益/现金流展示的是额度运营数据，与模型消耗重复且名称误导。用户确认将三页合为充值与消耗，只关心购入额度、各渠道闪电消耗和余额。

## Decision

复用管理员 finance 路由、权限、启动预热和五分钟快照，在同一次日志遍历中额外按渠道/UTC日汇总；不另查日志或官方数据。保留历史 upstream_model_name、模型/日期的上游对照和 BYOK。单页上半部累计额度/消费/余额，下半部同一日期范围的上游USD、本站额度、请求数及渠道/模型/日期表；表格搜索不改变全账户区间指标，生命周期卡片不随日期变化。最近30个完整UTC日快照前端筛选7/30日，切换日期或维度不发请求。CSV沿用本站额度列和转义规则。

移除旧现金页组件和标签。旧 cash/report API、账本及权限保留，仅取消旧7/30日自动登记预热；兼容调用登记后仍由原调度后台刷新，闲置30分钟回收。日常界面只消费 finance 一份快照。

## Existing notes audit

- [模型消耗](../../implemented/feature/2026-11-12-openrouter-model-consumption.md)、[finance快照](../../implemented/bug-fix/2026-11-13-openrouter-finance-snapshot.md)、[账户财务](../../implemented/feature/2026-11-02-openrouter-account-finance.md)：部分重叠，继续承接权限、历史模型和金额口径。
- [现金快照](../../implemented/feature/2026-11-15-cash-report-snapshots.md)、[运营修复](../../implemented/bug-fix/2026-11-15-cash-operating-data.md)：部分取代入口和预热决定，旧API数据源/隔离继续保留。
- 现金账本v1和现金利润提案：只涉及审计与未来真实资金核算，不在本次实现范围。其余命中的支付、企业、路由笔记无关。

## Alternatives considered

- 两份快照在前端拼接：可以少改后端，但重复官方采集、统计和时间口径仍存在，不能完成统一。
- 仅复用cash/report：已有渠道表和SQL汇总，但丢失历史上游模型对照、BYOK和官方单侧模型，不符合合并保留能力的目标。

## Verification

三种维度总计一致、同名渠道按ID分开，保留历史模型和未知值。快照失败保留所有关联分项；账号隔离、禁用、权限、日期/搜索/分页/导出及移动布局回归。旧cash接口不再主动预热但已登记请求可用。Go model/controller/router 回归、专项 race、Bun 汇总测试和两组真实Chrome回归通过；Go构建、完整前端构建与预渲染通过。手机布局已截图检查；笔记和发布流程校验随提交执行。

## Consequences

finance既有遍历在后台处理30日日志，增加渠道分组不增加扫描次数；内存与耗时需保持既有两路并发/两分钟上限。日期限定官方可提供的30个完整UTC日；旧cash自定义范围API保留。发布和生产部署分别验收。

区间显示币种由[人民币对照](../feature/2026-11-15-openrouter-cny-comparison.md)接续；数据源、归属和快照不变，账户累计保留USD。
