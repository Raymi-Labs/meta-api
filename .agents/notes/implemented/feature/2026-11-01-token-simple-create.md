# Agent Note: 令牌创建默认单组与价格提示

Status: implemented

## Problem

新建默认启用多模型分组，名称后先出现模式开关与模型白名单，用户必须理解路由配置才能选组。常用通用分组缺少相对官方价格的直观说明。

## Decision

[当前倍率折扣](../bug-fix/2026-11-01-token-group-ratio-discount.md)部分取代下文的固定三组价格说明决定，改为按当前适用倍率统一换算；本篇单组默认与高级设置布局保持。

复用单组选择器作为新建默认入口，名称后直接选组；原有自动回退继续沿用后台 DefaultUseAutoGroup。模型限制、可选多模型分组配置和跨组回退开关放进持续挂载的高级设置。已有多组令牌保留原配置，并在主区展示已选组及高级设置入口。

企业级（兼容企业）、优选（兼容优质）、福利通用组分别展示用户指定的“与官方价格一致”“比官方价格低20%”“比官方价格低40%”。精确匹配通用组名，不套用供应商专属组；实际适用倍率与后台说明继续展示。这是用户指定的产品档位说明，不从倍率推算、不变更价格配置与计费。

## Existing capability review

EditTokenModal → /api/user/self/groups（UserAuth）→ controller.GetUserGroups → service.GetUserUsableGroups/GetUserGroupRatio 已提供权限内组、说明和本人倍率；模型目录与管理员渠道限制保持原接口。无需新增后端或数据库字段。

- [创建选择器](../../implemented/feature/2026-10-20-token-create-selector.md)与[多模型分组](../../implemented/feature/2026-10-27-token-model-groups.md)部分重叠：本篇取代新建默认多组及其主区展示；保留配置、异步与权限契约。
- [通用组置顶](../../implemented/bug-fix/2026-11-01-token-group-priority.md)保留企业级/优选/福利排序，补充档位价格提示。
- [首选组自动回退](../../implemented/feature/2026-10-24-preferred-group-fallback.md)继续持有默认值与实际计费边界，移动开关不修改其语义。

## Alternatives considered

- 删除多模型配置可彻底减少控件，但会丢失已有用户的配置入口；将其保留在高级设置，默认使用已有单组模式。
- 继续默认多组而仅移动开关无需改变默认模式，但用户仍需处理多选路由范围，不能实现名称后直接单选分组。
- 根据倍率换算官方折扣看似通用，但倍率并非官方价格比例，且存在专属倍率；按用户指定的通用产品档位展示，保留实际倍率。

## Testing

`token-create-selector.browser.mjs` 与 `token-routing.browser.mjs` 使用真实组件、合成接口和本地 Chrome 验证默认单选、名称必填、折叠模型限制与模式开关、三档价格提示、创建载荷、旧限制原值、多组配置、失败重试、迟到响应隔离、旧 auto 和可售令牌，以及手机深色布局。高级设置保持挂载，旧多组令牌可以展开继续编辑，折叠保存不丢配置。

分组/收藏/复制单测24项、语言异步/17语言资源校验21项通过；修改源码 ESLint、Prettier、完整前端构建与笔记校验通过。构建保留现有大包和 Browserslist 提示；本地 Chrome 补跑公开页面预渲染。未修改后端和数据库，无真实模型调用或线上令牌写入。

## Consequences

普通用户只需填写名称并选组；高级配置需展开，但字段持续注册以保留原值。将来产品档位价格改变需同步文案；当前不变更实际结算。合成 API 验证与 GitHub 发布不代表生产部署。
