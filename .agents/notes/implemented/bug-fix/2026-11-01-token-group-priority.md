# Agent Note: 令牌可用分组优先展示三个通用分组

Status: implemented

## Problem

创建默认多模型模式的可用分组直接沿用 `/api/user/self/groups` 对象枚举顺序，旧单组模式按倍率降序。企业级、优选、福利三个通用分组因此无法固定出现在最前面。

## Decision

复用 tokenGroupUtils，按原始分组名称精确匹配置顶企业级、优选、福利。多组候选的其余分组保留接口目录顺序；单组候选先执行既有倍率排序，再应用同一置顶规则。只处理已授权候选，不添加缺失组，不改变倍率或已选 routing_config.groups / model_groups 的实际路由顺序。筛选和收藏继续限制显示范围。

## Existing capability review

- EditTokenModal → UserAuth `/api/user/self/groups` → controller.GetUserGroups → service.GetUserUsableGroups / GetUserGroupRatio 已提供权限交集和本人倍率，无需新增后端配置或修改数据库。
- [单 Key 多模型分组](../../implemented/feature/2026-10-27-token-model-groups.md)部分重叠：继续保留选择顺序、模型映射和元数据展示，只补候选排序。
- [创建选择器](../../implemented/feature/2026-10-20-token-create-selector.md)部分重叠：本决策部分取代全部按倍率排序的规则，其余倍率边界、过滤与选组交互保留。
- 个人收藏、列表倍率、供应商排序及计费笔记不拥有本次候选置顶规则，不修改其决定。

## Alternatives considered

- 直接调整后端 map 输出顺序可减少前端处理，但 JSON 对象并无业务排序契约，旧单组倍率排序仍会覆盖它；无需为局部展示改造 API。
- 将所有候选一律按倍率排序可统一两个界面，但扩大了其他分组的顺序变化；采用共同置顶函数，保留各入口原有剩余排序。
- 对组名包含“企业”或“福利”的条目模糊匹配能涵盖别名，但会将企业专属、厂商福利组一起提前；按用户指定的三个完整组名匹配。

## Testing

20 项分组/收藏单测通过，覆盖精确组名、不同倍率、缺失权限候选、不可变输入和其余排序。`token-routing.browser.mjs` 验证多选候选顺序、旧单组排序、说明搜索/模型范围，以及选择福利→企业级后创建和编辑保存仍保持该路由顺序。`token-create-selector.browser.mjs` 原有创建/编辑、收藏/搜索/厂商、权限失败、异步隔离及窄屏流程通过。完整 `bun run build`（含本地 Chrome 预渲染）通过。

首次并行执行构建和浏览器测试时，构建写入 public/image-playground 触发 Vite 页面重载，多组套件初始挂载超时；构建完成后单独复跑通过。生成的非本任务静态资源已还原，未纳入提交。外部文档读取遇到重定向及 429，本次因果判断依据当前仓库实际表单、API 与服务代码。

## Consequences

候选前三项遵循指定顺序，倍率不影响置顶；缺失组不出现，筛选和收藏仍限制展示范围。共同函数只重排副本，实际路由数组、权限、倍率和计费保持原行为。三个组名是明确的展示约定，未来改名需同步该规则。

浏览器使用合成接口，不代表生产部署或真实客户端验收；生产升级另行进行。
