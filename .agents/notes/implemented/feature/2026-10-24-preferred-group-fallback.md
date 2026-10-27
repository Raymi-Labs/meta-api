# Agent Note: 首选分组与故障自动切组开关

Status: implemented

## Problem

用户希望先选择分组，再决定该组不可用时是否自动尝试其它分组。现有 auto 是特殊分组，固定分组不使用自动列表，跨分组重试开关只在 auto 下显示。生产默认 auto 已开启，但用户可选目录缺少 auto，创建页面也仅调整 auto 排序而不设置默认值，导致用户找不到入口。

## Decision

新增独立的令牌 auto_group_fallback 布尔字段，固定分组仍是首选。开启后先尝试首选组，再按现有 AutoGroups 顺序尝试用户有权限的其它组，去重并继续遵守模型、渠道、客户端限制。关闭保持固定组路由。复用既有渠道选择、容量准入、失败重试、计费及日志，不修改重试错误分类或扩大实际调用预算，不在已输出后增加重发。

旧令牌缺省 false；旧 auto/cross_group_retry 行为保留，避免把历史固定组中无效的 cross_group_retry=true 误当作授权。创建页面选择真实分组，独立开关默认沿用 DefaultUseAutoGroup；已有令牌编辑保留原值，可售令牌不开放该字段。初始令牌以用户默认组为首选。普通模型目录、令牌模型限制与实际允许路由范围一致；[多模型分组范围](2026-10-27-token-model-groups.md)扩展后，旧固定组测试默认限首选组，并可显式选择获准的备用组；开关旁解释按实际分组计费，列表标明首选倍率与自动切换。

## Existing capability review

- 创建/编辑 → token controller → model.Token；鉴权写入上下文 → distributor → service/channel_select → ability/cache。已有 AutoGroups 和用户分组权限交集可复用，无需新建路由表。
- [创建选择器](../../implemented/feature/2026-10-20-token-create-selector.md)部分重叠：继续使用两栏分组与异步保护，新增独立开关，不再把 auto 当作新建时必须选择的普通组。
- [列表倍率](../../implemented/feature/2026-10-07-token-list-group-ratio.md)部分重叠：首选组倍率沿用原计算，新增实际分组计费说明。
- [安全切换](../../implemented/bug-fix/2026-10-12-upstream-capability-failover.md)继续约束错误分类、次数、已输出和指定渠道边界。本次不放宽重试条件。

## Alternatives considered

- 仅开放 auto 可选无需代码改动，但不能表达用户首选组和是否故障回退这两个独立意图。
- 将 cross_group_retry 直接扩展到固定组可以省字段，但旧固定组可能保留此前不生效的 true，升级会改变这些令牌的路由与费用。
- 前端把选中组直接改为 auto 能复用旧后端，却丢失用户首选，违背本轮目标。

## Testing

- `go test ./service ./middleware ./controller ./model` 通过；整合主线后 `go test ./...`、前端完整构建及 `make verify-notes` 通过。新增服务测试覆盖内存缓存开/关、首选优先、有序去重、用户权限和渠道白名单、停用组、关闭回退、候选读不推进容量等待状态、跨组不重置预算。
- `TestRelayPreferredGroupFallback` 经真实 Distribute/Relay 和本地 HTTP 上游覆盖 8 种情况：首选健康、停用开/关、上游 503 开/关、全部失败、重试关闭、渠道限制。核对最终日志分组与 1x/3x 倍率、仅成功扣一次及失败退款。
- 令牌创建/更新、false 持久化、`/v1/models`、令牌模型及渠道目录、可售令牌鉴权与目录保护通过。旧 auto 保留原路由分支；固定组遗留 cross_group_retry=true 不触发新能力。
- CUA 操作本地完整令牌页面（真实组件、合成接口）：普通用户/管理员、默认开/关、选组不改变开关、开关切换刷新目录、保存载荷与列表 Auto 标记、关闭保存后刷新、旧固定组默认关闭、旧 auto 跨组值回填与保存、可售令牌无开关、失败保留输入、390px iframe 中的暗色弹窗均通过。测试未访问真实模型或修改生产令牌。
- 既有 `token-create-selector.browser.mjs` 更新新建 auto 选项及默认开关断言，并增加旧 auto 回填保存回归；本轮只执行其语法检查，浏览器验证使用 CUA。发现条件卸载 Form.Switch 会重置旧 auto 值，改为保持两个字段注册、仅切换可见性，重开与保存复验通过。
- Bun 前端生产构建通过；本机缺少 `/usr/bin/chromium`，预渲染脚本按既有逻辑跳过，保留后端元信息兜底，发布 CI 继续验证其构建。无额外手动安装或启动生产服务。

## Consequences

新增 GORM 布尔列 `auto_group_fallback`，默认 false，沿用三数据库迁移抽象，不回写既有令牌；本轮数据库执行验证使用 SQLite，不声称完成生产 MySQL 迁移。复用现有重试分类与输出保护；切组可能改变倍率与服务特性，开关和列表明确按实际分组计费。后台自动列表为空时仍使用首选组。普通模型目录、令牌限制和测试范围与路由同步，增加一个独立兼容字段及对应界面状态的维护成本。发布只证明制品完成，生产升级和供应商现场复测单独进行。
