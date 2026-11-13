# Agent Note: 企业所有者本人额度申请可审批

Status: implemented

## Problem

线上验收发现Owner可以直接给自己分配额度，却不能处理自己的额度申请；审批列表显示待办却无操作入口。原实现将管理员禁止自批扩大到了Owner，形成无法在企业内处理的申请。

## Decision

Owner沿用直接分配的资金权限，可以批准或拒绝本人的待审批申请，已有待办直接适用。Admin仍不能处理本人或其他管理员的申请，Member无审批权限。前端操作可见性与后端事务权限同步；不根据申请发起时的角色授权，而在审批时重验当前有效角色。审批仍受未分配余额、申请金额、有效期、企业状态和一次性状态转换约束，保留申请人与审批人同为Owner的审计记录。

## Alternatives considered

隐藏Owner申请入口并要求撤回后直接分配，不能解决现有待办且多一次操作。放开全部角色自批会使Admin自行增加预算，不采用。Owner审批本人申请与其已有直接分配权一致，不新增企业资金或转入个人钱包。

## Verification

修复前新回归复现本人请求被ErrEnterpriseSelfOperation拒绝；修复后model/router相关测试在SQLite加race通过，覆盖Owner批准/拒绝本人申请、4次并发仅一次划拨、余额不足待办与余额不变、Admin自批/其他企业越权拒绝、转移所有权后原Owner不得自批、审批人审计与待办计数清零。前端实际可操作判定及身份状态共7项通过。Vite构建、288篇笔记门禁和diff检查通过；本轮测试与构建已退出，无预览服务。跨库用例复用既有CI，当前本轮未连接MySQL/PostgreSQL，不沿用上轮结果声称本次三库已验收。

## Consequences

这是对旧实现统一禁止自批的收窄；不声称双人审批制度，Owner仍是企业预算最终管理者。已有申请不自动批准，不改历史账目，无数据库迁移。

## Related notes

检索命中的[首期企业方案](../../implemented/feature/2026-11-11-enterprise-workspace.md)部分重叠，其禁止Admin自批与预算规则继续适用；[合并验收](../../implemented/process/2026-11-13-enterprise-merge-acceptance.md)保留，本文修正其后线上发现的交互/授权不一致。
