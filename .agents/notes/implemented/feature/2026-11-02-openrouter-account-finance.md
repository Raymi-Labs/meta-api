# Agent Note: OpenRouter 账户充值余额与消费明细

Status: implemented

## Problem

账户管理已有 new-api/sub2api 钱包、CRS、CPA，OpenRouter 只有渠道余额查询，缺少账户级充值与消费入口。用户明确选择充值、余额及 OpenRouter 消费口径。

## Decision

账户管理增加 OpenRouter 标签，复用上游账户表、加密凭据、管理员 CRUD 和后台余额快照，新增 openrouter 类型。固定官方 HTTPS 域名，拒绝重定向，不允许自定义站点接收 Management Key。消费读取 /api/v1/activity，按日、模型、供应商展示最近 30 个完整 UTC 日；金额固定 USD，BYOK 外部推理成本单列，不加到 OpenRouter 支出。/credits 提供累计购入额度、累计支出及差额余额，不能冒充逐笔充值订单或净营业收入。消费与余额独立失败，旧请求不能覆盖新选账户。

官方示例为 `YYYY-MM-DD`，真实 activity 响应也使用 `YYYY-MM-DD HH:mm:ss`。适配层完整解析日期、UTC 无时区时间戳或 RFC3339 时间戳，统一输出 UTC `YYYY-MM-DD`，保持前端排序和日期筛选契约；不能直接截取前十位绕过非法日期检查。

现金收益属于独立的[资金分摊提案](../../proposed/feature/2026-11-03-upstream-cash-profit.md)，当前财务明细不代表已实现利润计算。

## Existing notes audit

检索账户管理、upstream account、OpenRouter：未找到现有钱包归属笔记；定价、缓存计费、图标、模型性能相关笔记不涉及本入口。
[账号密钥迁移](../../implemented/process/2026-11-01-openrouter-account-key-migration.md)仅部分相关：该操作管理的是生产推理密钥，本功能独立使用管理员输入的 Management Key，不读取或更换渠道密钥，不执行充值。

## Alternatives considered

- 只扩展渠道余额：可复用已有 credits 调用，但会按渠道重复同一账户且无消费明细，无法满足账户入口需求。
- 独立账号表：隔离清晰，但重复加密、CRUD 和同步生命周期；采用已有账户类型扩展，无 schema 迁移。
- 按余额变化生成充值订单：实现简单但退款、调整和手续费会导致误判；仅显示官方累计购入额度，逐笔账单链接官方页面。

## Verification

Go model/controller/router 的 UpstreamAccount/OpenRouter 回归通过，涵盖凭据加密与同类型留空保留、跨类型密钥隔离、非管理员拒绝、官方固定域名、401/403/429/500/重定向、缺少金额字段、部分成功与空数组；后台消费增量使用累计 usage，不因充值余额增加归零。Go 构建通过。

生产只读直连返回 HTTP 200、214 条 activity，全部日期形如 `2026-11-01 00:00:00`，因此旧代码全部拒绝；检查未发现缺失 usage 或负数计量。回归覆盖真实字段形状、混合日期格式、UTC 跨日、非法日期/时分、负数消费、规范化后排序和金额不变。修复未改变数据库、支付或推理路径；生产只读验证不等于修复已部署。

Bun 金额与筛选回归通过；真实 Chrome 组件验证创建/编辑/删除、20条分页、搜索与筛选重置、零值与BYOK独立口径、失败不补零、切换账户丢弃旧请求、390px窄屏。Semi 的受控分页不自动切片，页面显式切片但合计覆盖完整筛选结果。桌面与手机截图人工检查。全量前端构建、17语言资源检查、Chrome预渲染、verify-notes与release policy通过。

消费看板读取路径采用[后台快照](../bug-fix/2026-11-13-openrouter-finance-snapshot.md)，官方接口和金额口径不变。

## Consequences

新账户沿用既有创建时启用的默认行为，停用/启用仅在编辑提供，避免旧表布尔 default:true 对新建 false 的回填与表单承诺不符。

复用旧表与跨库 ORM，不新增 schema、数据库专属 SQL 或推理调用。需要单独的 Management Key；不从已有渠道密钥推断管理权限。余额与消费分别展示错误，固定官方 HTTPS、拒绝重定向并限制响应大小。全部17语言覆盖新增界面。

选用的 credits/activity 接口不提供逐笔充值订单；展示累计购入额度与官方账单入口。消费存在UTC完整日延迟，BYOK外部推理费用不计入OpenRouter消费。真实凭据只读验证了生产接口；修复后的生产界面和 MySQL/PostgreSQL 全流程尚未验收，发布不等于部署。已存余额快照仍遵循现有定时同步，不补历史订单。
