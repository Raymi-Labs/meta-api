# Agent Note: 连续额度不足自动禁用令牌

Status: implemented

## Problem

客户端在本站额度不足后持续调用，现有提醒不能停止这些请求；用户要求同一令牌连续三次额度不足后自动禁用，并确认包含账户钱包、订阅、令牌和令牌周期额度。

## Decision

复用 TokenAuth、结构化 QuotaRejection、最终请求观察器及 TokenStatusDisabled。仅实际模型/任务调用参与，以最终结果持久化的顺序计算连续性；成功及通过鉴权后的其他失败清零，查询不参与，单请求重试不重复计数。第三次切换禁用状态，额度恢复不自动启用，现有令牌页手动启用。令牌保存清零并递增代数，旧在途结果不能污染新一轮。

Token 新增计数及代数字段，GORM 兼容新增迁移。数据库事务更新同一令牌行串行化，避免内存/Redis 丢失；Redis 模式鉴权额外核对主库状态和代数，防止异步缓存回填重新放行已禁用令牌。最终观察同步完成，不依赖通知开关、日志开关或通知队列。数据库故障记录内部错误，不伪造计数成功。

### 错误文案与原因链

`NewAPIError.SetMessage` 只替换展示文案，通过可 Unwrap 的内部包装保留原始原因；重复替换不累积包装。Relay 在记录失败对象后追加 Request ID，最终观察器必须仍能识别 QuotaRejection。原始原因仅供服务端类型判断，客户端 OpenAI/Claude 错误仍输出既有格式化及脱敏文案。不能用 `insufficient_user_quota` 字符串替代可信原因判断。

## Existing capabilities and note audit

功能地图 → 令牌页面 → controller/token.go → model/token.go 管理已有启停；middleware/auth.go 和 service/billing_session.go 提供本地拒绝事实；middleware/request_failure_log.go 提供最终结果。普通用户仍仅管理自己的令牌。

检索额度不足、连续失败、令牌禁用相关活跃笔记：[额度受阻通知](../../implemented/feature/2026-10-24-quota-blocked-notifications.md)部分重叠，复用可信事实但不复用异步通知计数；通知规则不改变。[令牌创建](../../implemented/feature/2026-10-20-token-create-selector.md)仅表单相邻；其余运维、邮件、渠道健康笔记无关，无过时同主题提案。

## Alternatives considered

- 只使用 Redis 原子计数：热路径开销低，但重启、缓存清空及无 Redis 部署会丢连续性，第三次禁用又需同步数据库；选择主库原子更新。
- 在每个预扣失败处计数：接入直接，但资金回退成功或内部重试会重复累计；选择请求最终结果观察，保留结构化错误来源。
- 只删除普通令牌缓存：不增加鉴权查询，但异步旧快照可能再次写回启用状态；选择 Redis 命中后额外核对主库状态与代数。

- 仅复制 MarkRequestFailure 的错误对象：可以保护当前观察器，但其他 SetMessage 调用仍丢失 errors.Is/As 语义；选择在错误文案方法保留原因链。

## Verification

`TestTokenQuotaFailureDatabaseMatrix` 在 SQLite、MySQL 8.0（utf8mb4_unicode_ci）及 PostgreSQL 16 隔离库验证两次保持、第三次禁用、清零、24 并发、令牌隔离、保存代数和旧在途结果忽略。`TestTokenQuotaFailureStaleRedisCannotReenable` 验证旧快照回填及 Redis 离线仍拒绝已禁用令牌。

`TestTokenQuotaAutoDisableHTTP` 经实际 TokenAuth 和最终观察器，验证令牌余额/周期包/钱包/订阅四种拒绝、预扣不足但余额非零、日志和通知关闭、查询及 IP 拒绝不参与、成功和普通错误中断、手动启用后重新累计。视频任务转换保留本地错误链；Midjourney 两处本地钱包拒绝添加结构化标记；上游错误不从文案推断。`TestTaskFinalQuotaOutcome` 验证任务 DTO 转换和每个请求仅观察一次；`TestTokenQuotaInvocationRoutes` 覆盖生成、WebSocket、视频提交及查询排除。

相关包全量 `go test ./model ./service ./middleware ./controller ./router ./relay/...` 通过；新增回归在三种数据库下的 race 检查通过。竞态测试发现缓存回填读同一令牌指针与鉴权状态更新竞争，`GetTokenByKey` 现先复制快照再异步写缓存。前端 Bun 生产构建、Go 服务端构建、发布策略及 `make verify-notes` 通过。未做生产部署或真实上游付费请求；未做压力吞吐基准。

### 2026-11-01 真实转发漏计修复

生产 v1.11.12 的只读匿名样本在检查窗口有 52 次钱包预扣不足、无成功消费日志，令牌仍启用且计数/代数为 0。可用额度 11379、所需预扣 42210，原始错误为本地 new_api_error。问题出在 Relay 的 defer：MarkRequestFailure 保存同一错误指针后，SetMessage 用 errors.New 追加 Request ID，擦除了 QuotaRejection。原回归直接调用预扣和 MarkRequestFailure，没有执行 Relay 的错误装饰，因而漏检。

`TestRelayQuotaDisablePreservesFinalCause` 使用 TokenAuth → 实际 Relay → 最终结果观察器，覆盖 Chat、Claude Messages、Responses、Gemini × 钱包/令牌/周期包/订阅 16 种组合，并关闭日志与通知；修复前全部首次计数为 0，修复后依次为 1、2、3 且第四次返回禁用提示。拒绝发生在本地，测试禁止向上游发送请求。`TestSetMessagePreservesCause` 覆盖多次改写、errors.Is/As、空原因以及对外 JSON 不泄露原始原因；通知观察器补充装饰后错误回归。没有修改生产令牌、数据库或部署；升级后的新失败才会计数，不回算旧日志。相关 Go 包全量测试及定向 race、Go 构建、Bun 前端构建和笔记校验通过；首次前端语言扫描在并行 Go 测试时超过默认 5 秒，待后台测试完成后用相同构建命令重跑通过。本次没有 schema 或 SQL 改动。

## Consequences

每次实际调用增加条件更新；Redis 鉴权增加主键查询。禁用后充值或周期重置也需手动启用。并发已通过鉴权的调用不被中途取消。发布和生产部署独立。

新增路由需同步 `IsTokenQuotaInvocation` 的调用范围；身份校验失败不参与连续性。原有令牌耗尽状态仍保留，第三次转为禁用；API 后续返回明确自动禁用原因，令牌列表复用原禁用展示。内部代数/计数字段不开放给 API 客户端写入。
