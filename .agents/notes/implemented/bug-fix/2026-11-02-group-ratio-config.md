# Agent Note: 空特殊倍率与后台测试缺省倍率

Status: implemented

## Problem

分组设置页清空最后一条特殊倍率时提交空字符串；Option 先落库后解析，RWMap 在解析前清空。生产每分钟读取同一无效值并报错，旧有效规则无法保留。管理员属于 default，但该分组已不在公开倍率表中，后台渠道测试仍调用普通倍率缺失告警。

## Existing capabilities and impact

复用 GroupGroupRatioRules → 超级管理员 PUT /api/option/ → model.UpdateOption → ratio_setting；保留传统 GroupGroupRatio 及 group_ratio_setting.group_group_ratio 入口。兼容空白只适用于特殊倍率，不扩展到其他配置；普通倍率表、分组权限、用户和令牌保持。

复用 HandleGroupRatio 的 IsChannelTest 标记；仅在后台渠道测试没有专属倍率且基础倍率缺失时使用现有默认值 1。正常调用仍走原诊断；有效基础/专属倍率及零倍率优先级保持。

活跃笔记检索：令牌列表分组倍率、令牌分组选择笔记部分关联，但持有的是展示/权限决定，保留；定价及视频测试笔记与本次配置写入契约无关。

## Decision

- 前端空特殊规则序列化为 {}；后端兼容旧客户端空白输入，校验嵌套对象及有限非负数值，拒绝 null 和非法结构。
- 写数据库和 OptionMap 前校验，解析到临时映射后用已有 ReplaceAll 发布；加载失败保留最后有效运行规则和 OptionMap，不改变通用 RWMap。
- 传统和命名空间键共用校验/加载，避免别名绕过保护。
- 不补回公开 default 分组，后台测试使用已有测试标记处理缺省倍率。

## Alternatives considered

- 只把线上空字符串改成 {} 可消除当前错误，但清空规则仍会复发，因此同时修复保存和加载。
- 给 GroupRatio 补 default:1 会影响分组展示和鉴权认可，因此只处理后台测试缺省倍率。
- 改通用 RWMap 会扩大到其他配置契约，本次仅复用 ReplaceAll 修特殊倍率入口。

## Testing

- Go 完整包回归：setting/ratio_setting、model、relay/helper、controller 通过；相关新增用例 race 通过；后端 go build 通过。
- SQLite 与隔离 MySQL 8.0.46（utf8mb4_unicode_ci）执行同一持久化测试，覆盖非法输入拒绝、创建/替换写失败回滚、空白规范化、重载及两类键；生产 options 引擎及排序规则已只读核对一致。未运行 PostgreSQL 实例；仅使用原有 GORM 与事务，无 schema 迁移或数据库专属 SQL。
- 真实 React/Semi 浏览器组件验证删除最后一条规则、确认并保存 {}，零倍率/四位小数与未完成规则序列化保持；仅合成数据，无生产请求。
- 前端完整构建通过；显式指定本机 Chrome 后公共页预渲染通过；既有 Browserslist 和大 chunk 警告保留。

## Consequences

本修复不重建历史已丢失特殊规则，不能从旧消费记录反推应有优惠。别名同时保存冲突配置的既有加载优先级不在本次范围。非法配置保留最后有效内存，首次启动则保留初始化规则；历史空白明确表示没有特殊规则。生产配置修正与应用部署独立验收，继续走受保护发布入口。
