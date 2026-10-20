# Agent Note: 令牌列表展示当前分组倍率

Status: implemented

## Problem

用户无法在令牌管理列表直接比较各令牌的分组倍率，必须打开编辑表单。列表中的倍率必须属于当前用户和令牌实际使用的分组，不能从名称猜测，也不能把未知倍率显示为 1x。

## Existing capabilities and impact

- `TokensColumnDefs.jsx` 定义列表列，`CardTable` 同时复用到移动端卡片。
- `/api/token/` 与 `/api/token/search` 经 UserAuth 后按当前用户 ID 查询、分页、脱敏；详情与编辑接口独立。
- `GetUserGroups` 和 `service.GetUserGroupRatio` 已包含用户分组的专属倍率覆盖。`TokenAuth` 中空分组继承用户分组，auto 的实际分组需请求时确定。
- 复用以上查询、脱敏与倍率服务，只给两个列表接口追加只读展示字段，不写 Token 模型、数据库或编辑参数，不改变计费和路由。
- 活跃笔记检索：[个人收藏分组](2026-10-04-token-group-favorites.md) 部分重叠于令牌入口，但持有的是收藏/权限刷新决策，保留原决定并互链；生图分组与 Kimi 计费笔记与本次无关。

## Decision

在分组后增加“倍率”列。列表与搜索共用 `buildTokenListItems` 响应构造器，每页至多读取一次当前用户分组，复用可用分组与专属倍率计算。新增 `group_ratio`（数值或 null）、`group_ratio_status`（fixed、auto、unavailable）响应字段；空令牌分组继承实际用户分组。无权/失效分组、用户查询失败、非法倍率都返回 unavailable，不阻塞原列表或回退 1x。零倍率有效。auto 不伪造固定值。

列展示 0.45x / 自动 / —；说明是当前分组倍率（含专属倍率），不等于模型单价、不含时间倍率，最终以使用日志为准。旧后端未返回展示字段时显示 —。不增加前端权限/倍率请求或持久化缓存。

## Alternatives considered

- 前端追加 `/api/user/self/groups` 请求可以完全不改服务端，但空分组还需要可靠的用户分组信息，并需要维护刷新、账号切换与过期响应失效；选择随列表返回当前展示值，避免新增异步错配。
- 把倍率写入 Token 模型能让前端直接读取，但会造成配置变化后旧值过期，并混淆只读展示与实际计费；选择仅在响应 DTO 计算，不新增数据库字段。

## Testing

- `go test ./controller -count=1 -timeout=180s`：整个 controller 包通过，原密钥脱敏与编辑回归保持通过。
- `go test -race ./controller -run 'TestTokenListGroupRatio' -count=1 -timeout=120s`：通过。隔离 SQLite 覆盖普通/专属倍率、基础零倍率/专属零覆盖、继承用户组（包括 auto）、失效/无权组、负数/NaN/Infinity、用户读取失败、配置刷新、列表/搜索分页和用户隔离。未连接 MySQL/PostgreSQL 实例；业务查询沿用现有 GORM，无新增 SQL/迁移。
- `node --test web/src/components/table/tokens/tokenGroupUtils.test.js web/src/components/table/tokens/tokenGroupFavorites.test.js`：原分组选择/收藏 16 项通过。
- `web/scripts/token-list-ratio.browser.mjs`：真实 TokensPage、hook、表格与移动卡片，拦截 API 为合成数据；列顺序、普通/专属/零/继承/auto/未知/旧后端/非法数据、提示、分页、搜索、刷新变价、空结果、重置、390px 窄屏无横向溢出均通过，无浏览器异常、写 API 或完整密钥读取。
- 主站 `bun run vite build --outDir <临时目录>` 通过，不重建生图或覆盖现有产物。既有 Browserslist、依赖 eval 和大 chunk 警告保留。新增文案在 7 种语言资源中齐全。
- 浏览器测试排除会独立监听端口的开发 inspector 插件，在 finally 关闭浏览器、Vite、esbuild 并删除临时入口；正常结束退出码 0，不保留测试服务。
- `make verify-notes`、相关 Prettier 与 `git diff --check` 通过；收尾进程/监听检查确认本轮测试服务与工作进程为零。

## Consequences

[创建窗口分组选择](2026-10-20-token-create-selector.md)复用既有用户分组接口的专属倍率，只调整表单展示，不修改本篇列表响应契约。

每个非空列表增加一次用户分组读取与逐条常量级倍率查询。倍率是列表加载时的快照，不自动保证管理员随后改价、时间倍率或实际请求选组的最终扣费；刷新列表可更新。DTO 为兼容性追加字段，旧前端忽略；新前端遇到旧后端不猜测。生产部署需要单独进行，提交或合并不等于线上生效。

第一性原理目标是用户在列表直接比较“该令牌对我适用的分组倍率”，不是新增定价配置。对抗性审查拒绝名称解析、缺失按 1x、auto 按固定数值、零值被当作缺失、专属倍率与基础倍率重复相乘、展示字段写回令牌，以及用前端缓存替代服务端权限。尚未部署，合成浏览器验证不替代部署后的真实账号验收。
