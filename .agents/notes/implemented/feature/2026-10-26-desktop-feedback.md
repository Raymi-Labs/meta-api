# Agent Note: 客户端报错在平台后台统一处理

Status: implemented

## Problem

客户端错误提示消失后难以收集复现条件，截图也缺少版本和发生时间。用户要求在客户端直接提交，并在元衡平台后台统一查看。

## Existing capabilities and impact

已核对 `router/api-router.go` 的用户/管理员认证、`middleware/rate-limit.go` 的按用户限流、现有后台侧栏及页面配置。平台目前没有反馈接收接口或工单模型；既有客户端安装包下载只分发文件，不适合存储反馈。活跃笔记中客户端下载、公开通知与额度通知只部分涉及入口，均不拥有反馈数据，继续保留。

## Decision

新增有界反馈记录，复用登录用户身份、GORM 迁移、管理员权限和控制台壳。客户端显式提交描述、版本、系统、固定页面/工具标识以及最多十条脱敏错误，不上传完整日志、请求或附件。服务端再次脱敏并用用户 ID 加 reportId 唯一约束提供幂等。正文最多128 KiB，描述最多4000个字符，错误最多10条、每条2000字符；字段仅显式绑定，账号每分钟最多10次提交。完整JSON递归隐藏敏感键，前缀/损坏JSON保守省略敏感字段后的文本；拒绝不兼容数据库的控制字符。写入使用独立静默GORM会话，避免失败或慢查询将报告正文写入常规SQL日志。管理员可分页筛选、查看详情并调整待处理/处理中/已解决状态。提交响应只在持久化成功后确认。

## Alternatives considered

- GitHub Issues 已有接收设施，但用户明确选择平台内提交，且 GitHub 登录与公开报告增加使用门槛。
- 完整客服工单支持回复和附件，但扩大存储、通知与权限范围；当前只实现问题收集及内部处理状态。
- 自动上传完整客户端日志便于排查，但容易包含账号、密钥与用户内容；采用用户预览过的有限错误摘要并在服务端二次脱敏。

## Verification

- `go test ./controller ./service ./model ./middleware ./router -run DesktopFeedback -count=1` 通过：真实会话鉴权下的未登录/普通用户/管理员边界、伪造身份与状态忽略、相同 UUID 不覆盖原报告、并发幂等与不同账号隔离、字段/正文/错误数上限、控制字符拒绝、多行敏感容器及私钥脱敏、分页筛选、重复及非法状态更新、持久化失败不确认成功、GORM 错误日志不记录正文。
- MySQL 与 PostgreSQL upsert SQL 渲染及字段类型回归通过。错误字段使用 GORM `size:131072`：MySQL 为 mediumtext，PostgreSQL 为 varchar，SQLite 为 text，足以保存十条多字节错误；不是实际 MySQL/PostgreSQL 运行验收。
- `bun run build` 包含 36 项多语言/正文资源回归、17 语言词典检查及生图/主站生产构建，通过。本机缺少默认 `/usr/bin/chromium` 时已有 prerender 脚本跳过，运行时 Meta 兜底保持不变。
- `web/scripts/desktop-feedback.browser.mjs` 使用隔离 API fixture 验收筛选、详情、HTML 转义、状态失败/重试成功、列表失败/重试与普通用户路由保护；浏览器截图覆盖桌面与390px布局。
- `make verify-notes` 与 `git diff --check` 通过。

## Consequences

脱敏是防御措施，无法识别任意自然语言中的秘密；客户端必须让用户预览。版本发布与生产部署分离，未部署接收 API 时客户端需保留草稿并明确失败。新表不改变既有业务数据；暂不自动删除记录或发送通知。
