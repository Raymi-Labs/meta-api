# Agent Note: 令牌候选隐藏零 Uptime 分组

Status: implemented

## Problem

创建令牌的分组候选只检查权限，最近健康统计全部失败的分组仍然可选，容易创建无法使用的令牌。

## Decision

复用 EditTokenModal 已订阅的 /api/health/groups 最近24个小时段数据，按同组全部授权模型汇总的 uptime_sample_count > 0 且 uptime_rate === 0 隐藏未选候选。旧响应缺少 Uptime 字段时回退 request_count / success_rate，使用共享 getUptimeRate；不按四舍五入的百分比判断。无样本、接口失败、统计关闭或采集不完整均保留候选。

普通单选、厂商/搜索/收藏与高级多选共用过滤后的权限目录。首次健康请求未完成时等待，防止全失败组短暂可选；已有缓存刷新不阻塞。当前已选分组保留，避免刷新或编辑静默清除配置，用户主动移除后重新应用过滤；保留只在当前授权候选内生效。不更改保存接口、路由、权限、模型目录或计费。

## Existing capability review

- EditTokenModal → UserAuth /api/health/groups → GetGroupHealth → grouphealth.Query → 小时指标与测试补充，接口先校验当前用户分组和模型权限。
- [创建选择器](../../implemented/feature/2026-10-20-token-create-selector.md)、[多模型范围](../../implemented/feature/2026-10-27-token-model-groups.md)部分重叠，沿用原保存/编辑规则；[低样本补充](../../implemented/bug-fix/2026-10-31-uptime-low-sample-probes.md)、[72小时展示](../../implemented/bug-fix/2026-10-31-uptime-72h-measurement-reference.md)属于数据来源，模型详情72小时展示保持，选组复用原24小时接口。健康颜色/小时详情不改变。

## Alternatives considered

- 在 GetUserGroups 删除健康失败组可供所有消费者使用，但会改变列表、权限和其他目录的语义，因此只过滤令牌编辑器候选。
- 根据组内模型列表为空过滤能发现静态配置缺失，却无法识别仍有配置但全部失败的组；本轮使用已有 Uptime 证据。
- 仅按真实请求成功率过滤最简单，但会忽略成功测试补充，和现有 Uptime 不一致。

## Verification

25项分组工具/收藏单元测试通过；真实 Chrome 组件脚本 token-create-selector.browser.mjs 和 token-routing.browser.mjs 通过，覆盖全部失败、部分成功、未知/无样本、测试补充、失败/采集缺失、首次加载等待、搜索、高级多选、健康恢复、原账号/权限隔离及单组/多组编辑保存。初次新增测试误用不存在的高级区CSS定位，修正定位后整套通过。

主站与生图应用生产构建、语言资源检查、Chrome五条公开路由预渲染、相关ESLint和Prettier通过。make verify-notes 通过；本地构建沿用已有大chunk、Browserslist和lottie eval警告。浏览器使用合成API，不请求生产或付费上游。

## Consequences

统计是有限窗口的历史证据，不保证上游当前状态；保留已选组以保护用户配置，接口不可用时保持可选。无数据库迁移，发布不代表生产部署。
