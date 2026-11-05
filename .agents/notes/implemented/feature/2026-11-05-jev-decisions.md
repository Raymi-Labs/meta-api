# Agent Note: Jev 决策模型接入

Status: implemented

## Problem

OpenRouter 的 Jev 1.14/1.13 使用 Decisions 请求与实际 input_tokens/output_tokens；现有 Meta API 仅有聊天、Responses 等协议，广场数据源来自启用渠道。单独添加模型名会错误声明聊天能力，不能形成可调用入口。

## Decision

复用 OpenRouter 渠道类型、令牌鉴权、限流、模型/分组授权、模型映射、预扣/退款与消费日志，新增非流式 POST /v1/decisions。原生 state/questions/assets 保真，响应保留 answers 与概率，实际 usage 转为统一计费；缺失或非法 usage 拒绝且不重发已成功执行的请求。新协议只支持 OpenRouter，已核实 Jev 具体版本声明 decisions，Router/latest 不自动纳入。

广场/详情协议和多语言调用示例、后台测试共用 Decisions 契约。沿用 models 元数据与启用 abilities，不新建目录表。生产先部署兼容代码，再通过管理 API 配置价格、厂商/模型资料，最后启用渠道；凭据不出服务器。

TypeSafe 原厂参考价沿用既有来源校验，明确认可 typesafe.ai 及其子域，仍拒绝 OpenRouter 转售价和伪装域名。Decisions 参数校验错误返回 400，不继承通用解析错误的默认 500；保持过大请求的 413。

## Existing notes audit

- [OpenRouter 厂商接入](../../implemented/feature/2026-10-31-openrouter-vendor-onboarding.md)：部分重叠，复用分厂商渠道、完整模型 ID 与管理 API 写入；原自动路由/latest 排除继续保留。
- [OpenRouter 原厂限价](../../implemented/feature/2026-11-01-openrouter-price-routing.md)：部分重叠，复用渠道 provider 覆盖，独立核实 Decisions 对策略的支持。
- [协议与示例](../../implemented/feature/2026-10-28-model-protocol-display.md)：部分重叠，扩展共用协议列表，决策模型不补聊天示例。

## Alternatives considered

- 只补广场展示最简单，但不能调用，且现有默认协议会误导用户，因此同步实现协议与计费。
- 将 Jev 包装为 Chat Completions 可以复用聊天界面，但会丢失多问题、概率和 typed answers 契约，因此保留原生 Decisions。
- 同时加入 Jev Router 可提供自动选模，但其动态价格与本次固定模型计费不同，且已有决策明确排除自动路由；本次不纳入。

## Validation

Go common/dto/relay/controller/middleware/router/model/ratio_setting 回归与 Go 构建通过；新增边界覆盖未带令牌、非 OpenRouter、模型映射、覆盖策略、stream/null 拒绝、实际272输入/22输出只扣272 quota，以及缺失usage/503各一次上游调用后退款。SQLite 与 MySQL 8.0（生产 channels/abilities/options/models 空结构、utf8mb4_unicode_ci）均通过。后台测试选择 Decisions 而非 Chat。

前端28项相关测试、完整构建及桌面/手机深色浏览器回归通过；七语言示例保留模型 ID、state/questions，不生成聊天模板。前端 prerender 因默认 Chromium 路径缺失跳过，实际浏览器测试使用本机 Chrome。现有 OpenRouter 凭据直连 1.14/1.13 均返回合法答案、272输入/22输出和 $0.000011424 成本，未将直连验收当作网关上线。

v1.20.0 生产文本调用两个版本均成功，分别 272 输入/22 输出，实际各扣 190 quota，临时令牌已删除。线上验收发现首次目录读取仍处于一分钟缓存窗口，以及原厂域名尚未登记导致参考价隐藏；缓存过期后两个型号正确出现，来源规则补充 TypeSafe。流式请求已被拒绝且未扣费，但默认错误分类为 500，因此新增精确 400 断言补上缺口。

v1.20.2 经受保护入口部署后，两个公网域名分别调用 1.14/1.13 成功，两条消费日志及令牌扣费合计 380 quota。流式、越权和低价限制分别返回 400/403/404，失败未扣费；官方参考价和桌面/手机原生示例显示通过。临时令牌均删除，后台两型号测试通过。价格采用 ECB 2026-11-05 快照；生产记录与多模态验收边界见接入说明。

发布运行 42068369096 首轮发生 Ubuntu 镜像下载延迟和 Windows 附件等待超时；Linux 与原上传作业分别重试后全部成功，正式 Release 九项附件及双架构镜像齐全。生产一直固定在已验证的 `sha-aaa02ae`，没有为附件问题重复切换应用。

## Consequences

Decisions 为上游 alpha API，格式可变。新增协议不改变任何既有模型价格或数据库 schema。模型广场、后台测试、分组健康及令牌额度失败观察共用现有机制；详情按钮指向原生示例，聊天体验不伪装成决策调试器。接入说明见[使用文档](../../../../docs/channel/jev.md)。

多模态真实推理及所有 question 类型需要独立验收；本地估算只用于预扣，最终严格按上游用量。缺少 usage 会退款，可能形成本站采购损失，但不凭空收费或重复提交。生产按先部署兼容程序、再管理 API 配置价格和资料、最后启用渠道的顺序验收，版本发布与生产完成分别报告。
