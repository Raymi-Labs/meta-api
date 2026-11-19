# Agent Note: 档位入池基础与禁止提前启用

Status: implemented

## Problem

阶段 A 草稿的矩阵下标、人民币换算、月基数计数及重复回调处理会产生错误分配。新增档位路径还没有周期发放与风控闭环，直接开启会由旧 worker 即时发款。规划 B–I 不能凭入池单测通过就认定可上线。

## Decision

复用现有支付事务、台账和结算 worker；本批仅落地计算基础并保持启用门禁关闭。`InviteTierReleaseReady=false` 阻止持久化/加载 enabled=true，内部测试可以直接设置快照测试引擎。必须在周期结算、有效消费、欠款抵扣及剩余验收完成后再移除此门禁。

订单分配回执与台账同事务提交，包含零奖励订单，防止重复回调按新身份重新分配。矩阵按收款人身份及一级档位索引；成员团队分成与二级部长位分别入账。使用实际人民币金额判断奖金门槛和月基数，quota 经 decimal 向下取整，超 int64 拒绝。月基数新增整数分列，二级与部长位共同占用；回执存占用分及 quota，撤销只释放一次。旧 decimal 列暂保留，不改变存量列单位。

配置补丁先合并为完整快照再持久化，进程内互斥防止同进程并发覆盖；不宣称支持多管理员跨实例并发补丁合并。新三列唯一索引先确认再移除旧两列索引，重复数据报错阻止迁移，绝不删金融记录。金额字段及既有严格断言同步扩为 int64。

## Existing capabilities and impact

[引擎完整提案](../../implemented/feature/2026-11-18-invite-tier-commission-engine.md) 部分重叠，保留其未落地的注册奖励与生命周期决策；[账单提案](../../implemented/feature/2026-11-18-invite-commission-statements.md) 与 [风控提案](../../implemented/feature/2026-11-18-invite-promoter-risk-identity.md) 仍待实施。旧 [即时结算](../feature/2026-10-31-invite-realtime-settlement.md) 的生产行为不改变。登录封禁提示与工单改动属于工作区已有独立改动，不纳入本提交。

## Alternatives considered

- 仅靠台账唯一键：能防重复行，但无法防重算身份后产生不同收款人，也不能保护月基数计数；采用独立分配回执。
- 原地将 decimal 月占用列改为整数分：字段少，但旧值单位改变会误读；新增明确的整数分列并保留旧列。
- 默认关闭但允许管理员直接开启：方便演示，但会使未接通的周期/风控路径发款；保留显式启用门禁。

## Consequences

36 种档位矩阵、月基数截断、奖金唯一、回调幂等、撤销释放、覆盖过期、数据库错误传播、币种与溢出、配置快照均有回归测试。SQLite 与隔离 MySQL 8.0.46/PostgreSQL 16 测试覆盖新索引和 bigint、并发回调；不等同 MySQL 5.7/PostgreSQL 9.6 最低版本或生产 schema 验收。

本批不构成可发布版本。注册奖励、冲回钱包钳制/欠款、C–I 的完整业务流程和前端仍需实现，不能标记整份规划完成。集中注册按用户确认只记录风险、人工冻结；不猜测网段与成团人数阈值。历史资格不补发策略、全链路锁顺序和三库生产兼容性仍需后续验收。

删除阶段 A 草稿中每次启动无条件执行的 bigint ALTER；正常与快速迁移均沿用 GORM 的字段类型检查，不在 AutoMigrate 后重复锁表改列。

验证结果：`go test ./model/... -race`（154.643s）、`go test ./common ./controller`、档位专项 race 与 MySQL/PostgreSQL 并发测试、`make verify-notes`、Bun 前端构建及 Go 可执行文件构建通过。前端预渲染因本机缺少 `/usr/bin/chromium` 跳过；未做新界面浏览器验收（本批无新增界面）。初次 Go 构建阻塞在旧静态资源读取，保留旧 dist 到临时目录并重新生成后通过。

## 后续完整交付

本篇保留基础阶段暂不开放的理由。完整周期与风控接入后，启用门禁由[周期账单决策](../feature/2026-11-18-invite-commission-statements.md)接管：默认关闭，具备持久化密钥后可人工开启；已入池新台账在关闭开关后继续结算。
