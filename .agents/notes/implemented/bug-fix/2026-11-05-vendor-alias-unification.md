# Agent Note: 厂商身份别名归并与写入防重

Status: implemented

## Problem

2026-11-05只读公开目录中Moonshot/Moonshot AI/MoonshotAI为三个ID，公开模型分别4/6/0；ChatGLM/Z.AI/智谱为三个ID，公开模型2/2/0。前端按vendor_name分组使同一作者拆成多个筛选按钮。后台CRUD仅按原始名称查重，默认/同步归一缺少这两组。Xiaomi与小米MiMo、Tencent TokenHub与腾讯涉及公司/系列/接入平台，不可无条件合并。

## Decision

复用CanonicalVendorName、FindVendorByCanonicalName与既有初始化元数据归一入口。显式身份白名单统一Moonshot/Moonshot AI/MoonshotAI为Moonshot AI，ChatGLM/Z.AI/Zhipu/Zhipu AI/智谱为智谱；已有MiMo、阿里、Inception规则保持。默认推断、内存厂商查找、能力同步与后台CRUD共用规范名称和别名查重，精确匹配不按包含关系归并，不处理模型名别名。

启动事务归并明确同一身份的Moonshot、智谱、Inception记录：优先现有精确规范名称，否则最小ID别名，保持目标有效图标/描述及状态；缺失图标填默认，不复制渠道来源描述为作者介绍。模型仅变更vendor_id和原已由自动同步持有的vendor_id快照，保持人工归属、来源与未知JSON字段。这里是同一作者记录身份去重，不推断模型作者：精确/前缀/包含规则及软删除模型均保持规则、状态和其他字段，只迁移引用，避免恢复后指向已删除厂商。确定所有模型引用清零后软删除旧记录，保留描述等元数据；重复执行无变更；错误全部回滚。

## Alternatives considered

- 前端只合并按钮：能暂时消除重复，但后台ID和同步仍分裂，详情和统计易不一致。
- 模糊相似度/公司归属自动合并：覆盖广但可能将Moondream、接入商、套餐或不同作者合并，不采用。
- 自动合并Xiaomi/Tencent TokenHub整个目录：当前空目录不证明将来只承载自家模型，保留既有按模型识别的能力。

活跃笔记：已有阿里/MiMo/Inception归一是复用机制，保持模型系列特定约束；OpenRouter厂商图标、能力同步快照部分重叠，本次仅补身份别名。不改上游受保护署名。

## Verification

公开目录盘点见[审计](../../../../docs/audits/2026-11-05-vendor-alias-audit.md)。本地 `go test -p 2 ./model ./controller -count=1 -timeout=240s` 两包完整回归通过；新增测试覆盖三名称并存、标准名/最小ID优先、元数据保留、软删除模型与前缀规则引用、自动/人工/未知/损坏JSON快照、事务回滚、幂等、CRUD冲突、默认推断/同步查找与定价模型库存一致。定价响应的派生PricingVersion会在刷新后填充，比较时排除该缓存版本字段，其他价格/分组/模型字段逐项保持。

GitHub Actions `Vendor identity compatibility` 运行41869912999通过：隔离MySQL5.7（utf8mb4_unicode_ci）、PostgreSQL9.6与SQLite实际运行迁移、失败回滚、幂等及查重。数据库服务随CI作业结束清理，无生产访问。前端10项目录筛选回归和厂商排序/别名匹配脚本通过；现有真实Chrome目录筛选浏览器回归通过（17语言、管理员保存及筛选），无新布局；主站Vite构建通过，完整主站语言/文档36项及生图805源key覆盖通过。`make verify-notes`通过。本轮本地测试/浏览器/构建进程为零，CI数据库容器清理步骤成功。

普通Git网络超时后使用GitHub Git数据API上传，核对初始远端树与本地提交树完全一致；不包含本机数据或密钥。

## Consequences

并发创建沿用现有表约束，应用别名检查不是新的跨实例唯一约束；规范化缩小名称差异，不宣称解决所有同名并发插入问题。数据库初始化执行数据事务，部署前需备份；版本回退不能自动拆回已合并引用，恢复依赖备份。管理员自定义别名描述保留在软删除记录中，不自动拼接到主介绍。上线前重复盘点，不硬编码生产ID。
