# Agent Note: 已禁用令牌的拒绝不再刷使用日志

Status: implemented

## Problem

生产两个不同令牌在连续三次额度不足自动禁用后仍被客户端重试。鉴权正确返回 401，但最终观察器把每次拒绝写入使用日志；用户名为空、原因泛化且复制信息缺少用户与令牌，掩盖实际调用失败。

## Decision

复用 TokenAuth 对已识别且状态为 Disabled 的令牌拒绝事实，在同一请求标记跳过最终失败入库。HTTP 401、原始错误提示和服务端访问/错误日志继续保留；第三次触发禁用的额度失败、过期/耗尽/IP/上游鉴权错误仍记录。判断基于鉴权时快照，不能在请求结束时查询当前状态，否则会吞掉触发禁用或并发在途的真实失败。

仍需入库的早期失败在缺少用户名时按已有用户 ID 解析；旧空用户名在管理员表格以用户 ID 兜底。复制排查信息仅在管理员模式增加用户和令牌身份，普通用户沿用公开字段，不输出密钥或 admin_info。已有可信额度拒绝标记用于日志 quota 分类，保留对外 HTTP/错误契约。

## Existing capabilities and note audit

功能地图 → UsageLogsTable / UsageLogsColumnDefs → /api/log/（AdminAuth）与 /self（UserAuth）→ controller/log.go → model/log.go；最终日志入口为 middleware/request_failure_log.go → service/request_failure_log.go。单一入库入口避免前端过滤导致总数、分页、导出不一致；不修改历史行。

[令牌自动禁用](../../implemented/feature/2026-11-01-token-quota-auto-disable.md)部分重叠，保持计数、代数、Redis 主库校验和手动恢复；本篇收窄禁用后的日志范围。[失败诊断](../../implemented/bug-fix/2026-10-17-relay-failure-diagnostics.md)、[上游并发诊断](../../implemented/bug-fix/2026-10-25-upstream-concurrency-diagnostics.md)、[请求 ID](../../implemented/bug-fix/2026-10-20-response-request-id-integrity.md)部分相邻，保留原有失败和诊断链；额度通知、流式空响应、任务提交与运维笔记不改变。无同主题过时提案。

## Alternatives considered

- 前端隐藏所有 authentication 行：能立即减少可见行，但吞掉过期等有用诊断，分页/导出/统计仍保留噪声，且无法准确识别历史原因。
- 按令牌做时间窗口去重：保留周期样本，但增加共享状态和恢复边界；已有服务端日志提供追踪，使用日志直接排除已禁用拒绝更明确。
- 关闭全部错误日志：实施简单，但会丢真实额度、限流和上游错误；只抑制明确禁用拒绝。

## Verification

`go test -p 2 ./model ./middleware ./service ./controller ./router` 全包通过。`TestDisabledTokenFailureLogLifecycle` 经实际 TokenAuth 验证八次额度拒绝只保留前三行、自动/手动禁用不进入下游、401及 Request ID 保留、额度不变、过期/IP/上游鉴权仍记录、在途禁用不吞失败、手动启用重新记录，以及用户名、用户隔离、分页、精确 Request ID 与导出一致性。

`TestRelayQuotaDisablePreservesFinalCause` 开启失败日志，经真实 Relay 的 Chat/Claude/Responses/Gemini × 钱包/令牌/周期包/订阅 16 种组合验证前三次失败入库和第四次禁用拒绝不入库；既有 middleware 自动禁用回归仍关闭日志，覆盖计数独立性。以上新增/更新回归的 race 检查通过，无真实上游请求。

前端 15 项排查报告/筛选/视图回归及 Bun 生产构建通过；验证管理员复制身份白名单、普通用户复制不附身份/内部诊断、历史用户名兜底。界面仅增加既有位置的文字，无布局变更；未另做浏览器验收。Go 服务端构建（版本输出 v1.11.16）、发布策略及 make verify-notes 通过。本次无新增 SQL/schema，隔离数据库回归使用 SQLite；MySQL/PostgreSQL 未重复运行。

## Consequences

仅部署后新产生的禁用拒绝被抑制，历史行保留。停用的客户端仍会连接网关，需客户端停止重试才能消除连接流量。没有 schema、生产配置或额度状态变更。仍记录的早期失败在用户名缺失时多一次既有缓存/主键读取；读取失败不阻断原日志，界面可用用户 ID 定位。发布不等于生产已部署。
