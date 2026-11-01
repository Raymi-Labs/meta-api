# Agent Note: default 继承令牌迁入福利

Status: implemented

## Problem

用户要求统计仍在 default 的令牌并全部迁到福利。显式主组和多组路由的 default 引用已为零，已有操作回执显示此前 1250 个显式令牌完成迁移；但仍有 51 个未删除令牌的 group 留空、routing_config 为空，其 47 个用户的组为 default。TokenAuth 实际沿用用户组，因此只统计 group='default' 会遗漏这些真实 default 调用。另 2 个空组令牌继承 EMOXIA，894 个已删除历史令牌仍保留 default。

## Decision

将这 51 个实际继承 default 的令牌明确设为福利，仅更新 tokens.group；保留 routing_config 的 NULL/空串差别，不添加多组配置，不修改用户组。全部当前状态为启用且未过期，其中 4 个有模型限制；保留密钥、余额、用量、有效期、状态及权限限制。两个继承其他组的令牌和已删除历史记录不动，已有显式 default 迁移不重复执行。

复用已验证的同引擎临时表演练、旧值条件事务列更新与 Redis HMAC 定向失效。本人令牌 API 无跨用户管理员局部更新能力，且会全量写回额度字段；不借本人 API 覆盖并发消费。实际 UPDATE 同时核对令牌空组、空路由和所属用户仍为 default，冲突中止回滚，不扩大到用户后来改变的分组。

## Existing notes audit

- [Gemini 迁入福利](../../implemented/process/2026-11-01-gemini-groups-merge-welfare.md)、[vertex 迁入优选](../../implemented/process/2026-11-01-vertex-group-merge-preferred.md)：复用事务保护字段、同 MySQL schema 演练和 Redis 定向失效，本次无渠道或全局分组变更。
- [单 Key 多模型分组](../../implemented/feature/2026-10-27-token-model-groups.md)：本次均无 routing_config，不创建或扩大其路由；显式及多组 default 引用仍核对为零。
- 功能地图的当前分组倍率与 middleware/auth.go 共同证实空组继承用户组，不能当作不属于任何组。

## Alternatives considered

- 仅报告显式 default 为零可避免写入，但遗漏 51 个真实继承 default 的令牌，不满足全部切换。
- 修改用户组会同时影响其他令牌、折扣和权限，用户要求的是令牌分组；只显式设置目标令牌。
- 把所有空组都改福利更简单，但会误改 2 个继承 EMOXIA 的令牌；限定当前所有者组精确为 default。
- 通过本人令牌 API 更新无法跨用户且有写回配额风险；使用已演练的带旧值条件的单列事务。

## Verification

生产同 MySQL 引擎/schema/排序规则临时表演练通过正常更新、旧值冲突和用户组冲突拒绝，保护列一致。事务完成 51 行 group 更新。密钥等保护字段、47 个用户分组、两个 EMOXIA 继承令牌、894 个已删除 default 历史令牌，以及价格/分组选项全部核对一致。未改写并发用量/余额字段。

两个真实迁移令牌只读访问 `/v1/models` 均返回 200，覆盖无模型限制（49 个可见模型）与有模型限制（1 个可见模型）；重建 Redis 的 Group 均为福利，受影响缓存残留旧组为零。最终显式、多组及空组继承 default 的未删除令牌为零。操作回执见[审计](../../../../docs/channel/default-token-welfare-audit-2026-11-01.json)，生产保持 v1.11.14，无程序更新或重启。

## Consequences

明确福利后不再随用户组未来变化自动切换，这是当前迁移的目的。实际使用福利按既有 0.6 倍率及模型价计费；保留模型限制不等于保证所有原模型当前都有福利渠道。历史账单不重算；并发新建空组/default 令牌由最后检查发现，不修改未来创建默认值。
