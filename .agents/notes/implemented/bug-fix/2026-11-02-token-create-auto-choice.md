# Agent Note: 创建令牌显式选择 Auto 与费用提示

Status: implemented

## Problem

新建令牌默认读取 DefaultUseAutoGroup，但 Auto 开关藏在折叠的高级设置中。用户可能在未注意到备用分组价格不同的情况下创建自动切组令牌。用户要求创建时询问是否启用，并提醒故障切组与实际计费。

## Decision

复用 auto_group_fallback 字段，在新建主区的分组选择下方显示必选的开启/关闭选项，不预选。说明首选模型不可用时按后台顺序尝试有权限的备用组、按实际分组计费且费用可能更高。选择前目录按首选组查询，选择后复用既有目录刷新、校验和创建接口。

编辑已有令牌保留原值与开关；高级多分组配置继续覆盖 Auto，并在主区显示其范围及实际计费提示，不要求选择一个不生效的开关。单组/多组切换保留已作选择，但新建会话与身份变化不能继承上次同意。批量创建沿用本次选择；未选择不能发送写请求。

后续由[多选分组选择器](../feature/2026-11-07-token-group-multi-picker.md)改为可选勾选框（默认不勾），创建与编辑都可修改，多分组令牌另存为 `routing_config.auto_fallback`。

## Existing capability review

EditTokenModal → POST /api/token/ → controller/token.go → model.Token 已支持 false 持久化；service/token_group_fallback.go 与 service/token_routing.go 已限定备用范围并按实际组计费，无需后端、价格或 schema 改动。

检索活跃笔记 Auto、默认、创建、自动切组：本决定部分取代[首选分组回退](../../implemented/feature/2026-10-24-preferred-group-fallback.md)的网页创建默认值，以及[简洁创建](../../implemented/feature/2026-11-01-token-simple-create.md)把新建 Auto 置于高级区的决定；保留其服务端与编辑契约。[多模型分组](../../implemented/feature/2026-10-27-token-model-groups.md)的范围优先规则保持。模型排序、健康过滤、价格折扣笔记仅相邻，不替代。

## Alternatives considered

- 只把默认开启的开关移到主区：可见性更好，但用户仍能未作选择就提交，未满足显式询问。
- 点击创建后再弹确认框：能取得选择，但改变开关后必须重新加载模型/渠道范围，容易让确认载荷与目录校验不一致；在同一表单先选择能复用现有流程。
- 全局关闭默认或批量改旧令牌：配置简单，但改变其他客户端或历史令牌行为，超出本次网页创建提醒范围。

## Testing

后台默认开/关均无预选；未选时阻止写入并定位问题；明确 true/false 正确保存；切组、目录刷新、失败重试、批量创建、身份/会话重开、多组切换、旧固定/auto 与可售令牌保持预期。桌面与手机、浅色与深色可见无溢出，17 种语言有提示。真实表单与合成接口由 `token-create-selector.browser.mjs`、`token-routing.browser.mjs` 验证；截图检查桌面、390px 浅色与深色，包含错误焦点和键盘提交。相关 ESLint、Prettier、语言资源检查、完整前端构建与 `make verify-notes` 通过。

## Consequences

网页新建增加一次必选操作，不调整后台默认设置或外部 API 行为。Auto 不保证供应商成功或价格更低；测试使用合成接口，无真实扣费或生产部署。
