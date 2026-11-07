# Agent Note: 撤回路由方案，保留按原顺序的智能路由总开关

Status: implemented

## Problem

2026-11-06 上线的路由方案（`route_plans`、令牌 `route_plan_id`、方案价锁定与方案跨组排序）把线路池、售价和授权打包成一套与分组并行的产品概念。用户评估后认为它让项目变复杂：散户按分组选价格，企业要的是稳定再便宜，两类需求用现有能力已能满足——企业统一价格用后台已有的「用户组专属倍率」，一个令牌调多个模型用已有的分组多选（`routing_config`），真正需要加强的只是线路观测和智能路由本身。上线后线上没有任何方案或绑定方案的令牌，撤回不影响用户。

## Decision

- 删除路由方案的全部代码与界面：方案表模型、CRUD 与 `/api/route_plan/*` 接口、鉴权时的方案快照、`HandleGroupRatio`/`service/quota.go` 的方案计费、异步任务方案价、日志方案字段、令牌编辑页「路由方式」、令牌列表方案显示、`pkg/route_insight/tier.go`，以及只供方案使用的 40 条界面文案。
- 计费恢复为原契约：按实际使用分组倍率（含用户组专属倍率与时段倍率）结算。
- 智能路由对所有令牌按原分组顺序生效：管理员页只保留「线路观测」，原「健康选路」更名为「智能路由总开关」（选项键仍为 `route_routing_setting.health_routing_enabled`，默认关）。开启后，每个令牌保持原有分组顺序、优先级和权重，跳过不健康渠道（`service/route_health.go`），unknown 不跳过；具体组内与跨组规则、可调阈值见[有序健康选路](../feature/2026-11-07-ordered-health-routing.md)。会话亲和遇到 unhealthy 渠道时释放给正常选择。`probe_percent`（默认 2，0–20）按每次尝试抽签，中签时不跳过，用于恢复后的线路重新积累数据。
- 不增加令牌级选路策略（价格优先/稳定优先）：所有令牌都按顺序。
- 逐次尝试遥测、影子决策、采购成本倍率与故障域字段、渠道健康快照保持不变，见[遥测与影子决策](../feature/2026-11-06-route-attempt-telemetry-shadow.md)。

### 线上遗留

生产 MySQL 已建的 `route_plans` 空表和 `tokens.route_plan_id` 列保留不删：GORM 读写忽略多余列，删表/删列需要另行迁移且无收益。options 表中的 `route_routing_setting.plan_routing_enabled` 行被配置加载器忽略。

## Alternatives considered

- 保留方案代码、仅在界面隐藏：回退成本最低，但保留一整套未使用的计费分支和权限旁路，后续维护者仍要理解它；线上无数据，删除比隐藏干净。
- 令牌级「价格优先 / 稳定优先」策略：能按客户偏好跨组动态排序，但会让同一令牌的实际计费分组随健康变化，并增加一项客户需要理解的设置；用户决定全部按顺序，先把路由本身做强。
- 按厂商/模型规则定价的方案：解决了「200 多个模型无法逐个定价」的问题，但前提是保留方案；改用用户组专属倍率后，按分组粒度定价，厂商单一的分组即可按厂商打折。
- 被撤回的方案设计及其备选（固定倍率专用分组、跨组按健康/成本选路、时段倍率不叠加等）见[已否决的托管路由提案](../../rejected/architecture/2026-11-05-managed-token-routing-and-pricing.md)。

## Consequences

收益：产品概念回到「分组 + 倍率 + 有序回退」一套；计费路径与上线前一致；智能路由开关语义单一，可对所有令牌生效。

代价与限制：企业「GPT 三折、DeepSeek 八折」这类按厂商折扣，只能在厂商单一的分组上用用户组专属倍率实现，混合厂商的分组（如企业级、优选、福利）同一分组只能一个倍率。撤回当时总开关只在组内避开故障、影子决策仍按成本口径推荐；两者随后由[有序健康选路](../feature/2026-11-07-ordered-health-routing.md)改为整组不健康时提前换组、影子与实际路由同一算法。

## Verification

`go test ./controller/ ./service/ ./model/ ./middleware/ ./pkg/... ./router/ ./dto/ ./relay/... ./types/ ./setting/...` 在 SQLite 下全部通过；`model`、`pkg/group_health`、`controller`、`service`、`middleware` 在全新初始化的 PostgreSQL 16 上通过（复用旧测试库时，其他测试遗留的同名索引会导致 group health 迁移测试多出建索引语句，与本改动无关）。`service/route_health_test.go` 覆盖总开关关闭时不变、组内跳过有替代的 unhealthy、只剩故障渠道仍可用、探测中签、亲和释放。前端 17 种语言资源检查、Prettier 与 Vite 生产构建通过，完整二进制构建通过。未部署。

[历史快照：路由方案独立售价与健康选路](../../archived/feature/2026-11-06-route-plans-health-routing.md)
