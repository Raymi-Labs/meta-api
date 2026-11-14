# Agent Note: 参数覆盖草稿试运行

Status: implemented

## Problem

渠道参数覆盖编辑器能校验 JSON 和规则格式，但管理员必须保存并调用模型才能确认条件、正文和请求头的结果。仅在浏览器重写规则会与 Go 转发引擎产生语义分歧。

## Existing capabilities and notes audit

复用 ParamOverrideEditorModal 的草稿序列化、管理员 channel 路由和 relay/common.ApplyParamOverride。现有日志只记录模型等少量关键字段，不能充当完整试运行结果。检索活跃笔记后，[请求 Schema 保真](../../implemented/bug-fix/2026-10-24-schema-order-preservation.md)部分重叠，继续持有原始数字/顺序约束；[渠道推理配置](../../implemented/feature/2026-10-25-channel-reasoning-profiles.md)持有预设配置，与本次草稿执行入口分开。无既有试运行提案需要取代。

## Decision

管理员 POST /api/channel/param_override/preview 提交未保存的覆盖配置、样例正文与模拟上下文；复用 Go 引擎返回修改后的原始 JSON、请求头覆盖、规则条件命中序号和拒绝信息。只接受有界 JSON，不读渠道、凭据或真实 HTTP 请求头，不调用供应商、不结算、不持久化、不生成消费日志。样例输入和返回正文以字符串传递，避免浏览器将大整数改写；上下文使用现有 BuildParamOverrideContext 规范。界面明确只预览覆盖阶段，不能证明供应商接受最终请求；草稿变化后清除旧结果。

## Alternatives considered

- 浏览器本地实现规则：无需新接口，但复杂的路径、条件和 header 规则容易偏离线上 Go 引擎，故复用服务端实现。
- 保存后复用渠道测试：能验证供应商，但有调用费用和真实副作用，也无法预览未保存草稿，故保留为独立功能。
- 引入上游完整插件框架：扩展性更强，但本地已有覆盖引擎，新增运行时和前端迁移超出本次需求。

## Verification

- Go 回归覆盖旧格式、操作列表、条件跳过、header、return_error 和无效正则；与真实 ApplyParamOverrideWithRelayInfo 比较结果，return_error 后的操作不执行。未登录/普通用户被拒，管理员通过，真实 Authorization 不作为样例，超大输入与尾随 JSON 被拒；测试无需初始化数据库或外部客户端。
- Chrome 本地隔离接口验证当前草稿预览、不保存渠道、大整数保真、无效正文后草稿保留、自定义拒绝码与条件命中展示；375px 预览对话框无横向溢出，可滚动查看结果。原始编辑器和预览入口均保留。
- 完整 relay/controller/service/model/middleware/dto/setting/types/router 测试与相关定向 race 通过；17 语言资源检查、主站/图片工作台构建及系统 Chrome 五页预渲染通过。浏览器现有 Semi findDOMNode 警告和预期的 400 错误不作为业务失败。

## Consequences

管理员可以在保存和模型调用前核对草稿，代价是新增管理员接口和模拟上下文维护。预览只代表管理员输入的上下文和参数覆盖阶段；真实映射、适配器和供应商验证仍可能不同。请求限制为 256 KiB、100 操作，并在操作后检查 1 MiB 输出上限，不能当作任意脚本沙箱。示例由管理员自填，不自动复制真实请求中的秘密。

条件路径与线上一致：先读取正文，缺失时直接读取上下文字段（如 upstream_model），不添加 context 前缀；复制缺失请求头时按既有规则跳过。正文通过字符串跨浏览器传递，不额外引入 JavaScript 数字精度损失；旧式覆盖与复杂清理的既有 Go 解码行为仍按线上引擎执行，不把预览标为通用原始 JSON 保真器。
