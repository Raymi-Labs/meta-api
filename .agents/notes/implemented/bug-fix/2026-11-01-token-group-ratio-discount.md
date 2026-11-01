# Agent Note: 令牌分组折扣按当前倍率换算

Status: implemented

## Problem

令牌创建界面根据企业级、优选、福利名称写死同价/低20%/低40%。用户明确纠正这里以当前倍率为官方价格比例，例如0.38应显示比官方价格低62%；名称映射会在配置变化时显示错误折扣。

## Decision

单选分组行和高级多组选项共用换算函数，读取当前用户目录的 ratio；小于1按(1-ratio)×100展示降幅，等于1显示同价，大于1显示涨幅。所有真实组统一计算，不依赖组名。百分数最多两位小数，清除末尾零；零倍率为低100%。未知、非法、auto、溢出或差异小于显示精度时不显示价格比较，保留原倍率状态。接口重新加载后提示随值更新，不写回配置。

## Existing capability review

EditTokenModal.loadGroups → UserAuth保护的/api/user/self/groups → controller.GetUserGroups → service.GetUserGroupRatio 已返回当前用户权限内组及适用倍率；TokenGroupPicker与TokenRoutingEditor共用同一目录。无新增接口或后端修改。

- [默认单组创建](../../implemented/feature/2026-11-01-token-simple-create.md)部分取代：撤销名称映射的固定价格说明及不按倍率换算的决定，保留单选默认与高级设置布局。
- [通用组置顶](../../implemented/bug-fix/2026-11-01-token-group-priority.md)、[健康过滤](../../implemented/bug-fix/2026-11-01-token-group-zero-uptime.md)继续适用，不改候选过滤、排序或已选值。
- [模型分组官方价比较](../../implemented/feature/2026-10-30-model-group-official-discount.md)是不同入口：继续按具体模型实际价格与官方价格逐项比较。本次按用户指定规则仅调整令牌分组选项，不替代模型价格计算和计费。

## Alternatives considered

- 固定三组折扣文案实现简单，但配置变化后仍会错误，不能满足自动换算要求。
- 接入逐模型官方价格比较可以给出模型费用差异，但令牌选择的是分组、尚无具体模型；按用户明确指定的倍率规则展示，不引入不明确的聚合折扣。

## Verification

- 25项分组/收藏/健康过滤单测通过，覆盖0.38、0.6、0.8、1、0、1.2、小数舍入和未知/auto/非法/溢出边界；不依赖分组名称。
- 21项语言加载/17语言资源测试通过；删除固定20%/40%键，改用percent插值。
- 真实组件Chrome合成接口回归通过：福利0.38显示低62%，重新打开后0.6显示低40%；单选与高级候选一致，旧多组/auto保存、失败重试和迟到响应隔离保持。目视桌面与390px深色截图，无横向溢出。
- 修改源码ESLint、Prettier、完整前端构建（含五页Chrome预渲染）及make verify-notes通过；保留原有Browserslist和大包提示。

## Consequences

分组配置变化无需改价格提示，统一函数同时供单选和高级配置使用；新增高于官方价格的显示分支并维护17语言插值。该口径限于用户明确指定的令牌分组倍率说明，不作为单个模型最终结算证明；模型详情继续采用实际单价比较。舍入仅用于显示。发布与生产部署分开验收。
