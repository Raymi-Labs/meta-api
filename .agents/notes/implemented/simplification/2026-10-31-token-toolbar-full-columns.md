# Agent Note: 令牌页固定完整列与单行工具栏

Status: implemented

## Problem

用户要求默认全部列、移除显示全部列复选框、标题总计和紧凑列表按钮，并把添加令牌与查询重置放到同一行。现有摘要列表与两行工具栏需要多一步操作且占据垂直空间。

## Decision

复用 TokensColumnDefs 和 CardTable 的完整列，移除摘要/完整切换及令牌页紧凑偏好读取；桌面横向滚动并固定操作列，手机继续使用完整字段卡片并保留批量选择。TokensActions 与 TokensFilters 同行，批量操作选中后另起一行，窄屏自然换行且保留次级筛选折叠。分页总数仍由底部分页展示。

列表/搜索沿用 UserAuth → controller.GetAllTokens/SearchTokens → model.GetAllUserTokens/SearchUserTokens 的当前用户数据范围；不改鉴权、请求、计费、完整密钥按需读取或写入流程。

## Related decisions

- [令牌管理](../../implemented/feature/2026-10-27-token-management-experience.md)：部分取代其默认摘要与详情切换决定，保留身份/选择/复制/保存隔离。
- [创建选择器](../../implemented/feature/2026-10-20-token-create-selector.md)：部分取代顶部两行布局，保留创建、批量与分组契约。
- [手机筛选](../../implemented/feature/2026-10-28-ordinary-user-ui-hierarchy.md)：部分重叠，保留折叠与草稿字段持续挂载。倍率、多模型路由、分组收藏等笔记与本次展示调整无关，保留。

## Alternatives considered

- 仅把 fullView 默认值改为 true 最小，但会留下用户要求移除的切换入口与摘要渲染，且旧紧凑偏好会影响完整列固定/滚动；直接固定完整表格。
- 将所有按钮塞入筛选表单能实现单行，但会耦合创建和提交语义；保留原组件，以局部弹性布局排列。

## Verification

既有 token-list-ratio 浏览器验收使用真实组件与合成 API，通过默认完整字段、旧紧凑存储不干扰、标题/开关移除、1600px 常用操作同排及批量操作另排、分页/查询/刷新/空结果/重置、390px 完整字段与选择、高级筛选打开、320px 深色无页面横向溢出。未访问外部 API、读取完整密钥或产生写请求。

21 项复制/分组/收藏回归、相关 ESLint/Prettier、完整 Bun 构建与五个公开页预渲染通过。浏览器脚本既有两行/手机操作隐藏断言已按新界面调整，Semi 复选框点击外层控件验证实际交互。

## Consequences

顶部空间收紧，所有字段立即可用；完整字段增加横向宽度与手机卡片高度，桌面表格内部滚动、工具栏允许换行。旧紧凑存储值保留但不再影响令牌页，不改其他页面偏好。如后续需要按字段定制，应以明确用户需求重新设计，不能自动恢复已移除的摘要开关。

本次仅发布源码和产物，不等于生产部署或真实客户验收。
