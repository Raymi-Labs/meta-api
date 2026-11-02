# Agent Note: 健康统计只排除结构化本地额度拒绝

Status: implemented

## Problem

令牌或周期包预扣不足已产生 QuotaRejection，但 ClassifyFailure 只按 insufficient_user_quota 排除；pre_consume_token_quota_failed 落为未知并进入分母。反向风险是数据库故障被余额不足错误码包装后误排除。用户要求修复健康归因，不能美化平台故障。

## Decision

复用 QuotaRejection 和现有 OutcomeUserError/ExcludedUserErrorCount。只在本地 NewAPI 错误、无上游状态、明确额度错误码且 errors.As 能取得非空结构化拒绝时排除；裸错误码、字符串、上游伪造保持未知/服务故障。限制错误码以免结构化拒绝作为系统失败的嵌套原因时掩盖故障。计费资金预扣改为结构化订阅判断，追加预留的数据库错误使用 UpdateDataError；预扣回滚失败保留系统故障。金额、资金顺序、成功回退和历史聚合不改。

活跃笔记：[最终结果分类](../../implemented/bug-fix/2026-10-22-group-health-outcome-accounting.md)部分重叠，本次补充可靠的用户拒绝分类；[额度通知](../../implemented/feature/2026-10-24-quota-blocked-notifications.md)及[连续额度拒绝](../../implemented/feature/2026-11-01-token-quota-auto-disable.md)复用相同结构化原因，保留原规则。任务一致性及禁用日志去噪不重做。

## Alternatives considered

- 所有预扣错误都排除：简单，但数据库/事务失败也走预扣错误，掩盖平台故障。
- 在健康统计单独识别文字：改动小，但与日志/错误分类分裂，可被上游文案混淆且本地翻译破坏。
- 重建指标表或历史回填：可分离新旧窗口，但当前排除列与分母已够用，旧聚合不能还原逐笔原因。

## Verification

`GOMAXPROCS=4 go test -p 2 ./types ./service ./controller ./pkg/group_health ./middleware -count=1 -timeout=240s` 五个相关包完整回归通过。分类测试覆盖四来源、双层包装与错误文案装饰、裸码/同文字/上游状态/系统包装；service 覆盖伪装订阅拒绝的数据库文案、追加预留资源异常、真实令牌扣减后回滚查询失败。

扩展已有真实 Relay 测试，Chat/Messages/Responses/Gemini × 钱包/令牌/周期包/订阅/令牌数据库异常/订阅数据库异常，共24种组合，每种4次请求。验证不发送上游、余额不扣、日志 failure_class、健康落盘与查询汇总、排除数/分母，第四次鉴权拒绝不新增分组采样；数据库异常不触发额度自动禁用。同组服务故障不可被后续拒绝抹去，跨组成功恢复保留原语义。collector→汇总验证90成功10故障20拒绝=90%成功率、排除20、观察120；仅排除无成功率，探测补充只影响Uptime采样。既有权限、路由、取消及资金来源回退回归通过。

前端健康展示4项顶层回归通过；完整主站语言资源（含文档正文）36项检查及图片子应用语言覆盖通过；`make verify-notes` 校验220篇笔记及封印通过，`git diff --check` 通过。本轮遗留测试进程为零，无收费请求。

数据库为 SQLite 隔离 fixture，不改 schema 或 SQL。初次数据库故障 fixture 使用信任旁路导致未进入预扣，改为低于信任阈值且足够付款的有限额令牌后正确注入故障。

## Consequences

历史小时聚合不回填，记录生效版本，旧窗口自然滚动。共享分类影响日志 failure_class 与取消相关调用者，需回归。无 schema/价格/全局路由改动；SQLite回归不声称生产或三引擎实机验收。上线需受保护发布流程，本次合并不等于部署。


额外审查发现：原无限额令牌扣减只更新 used_quota，但 IncreaseTokenQuota 回滚会增加 remain_quota，导致其不用于限额判断的剩余额度字段漂移。本次不调整通用退款实现，不将其称为本次修复；订阅回滚验收使用有限额令牌同时验证真实余额与额度不变。该发现应作为独立账务修复，不能仅修改断言便宣称无限额退款完全正确。
