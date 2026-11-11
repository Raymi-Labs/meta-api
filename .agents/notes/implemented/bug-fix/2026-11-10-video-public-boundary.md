# Agent Note: 视频任务公开响应隔离上游地址

Status: implemented

## Problem

视频适配器将上游成片 URL 写入 metadata；通用查询与用户列表还返回原始 Data、旧版 FailReason 和映射后模型。下载代理复制上游响应头，取片成功并不表示上游身份已经隐藏。

## Decision

复用 `/v1/videos/{公开任务ID}/content`，公开输出使用字段白名单，成功时只返回基于管理员配置 ServerAddress 的公开绝对地址：元衡自有HTTPS域名复用已有精确白名单归一到 `https://meta-api.com`，其他部署保留配置域名；不信任请求 Host/Forwarded，配置非法时仅返回站内相对路径，不改变数据库原始数据或全局 ServerAddress/支付回调配置。覆盖统一/兼容查询、普通用户列表、Sora/Muse 提交及视频上游错误。管理员列表保留排障信息。取片限任务所属用户或管理员，响应私有禁缓存，仅输出必要媒体头。

## Alternatives considered

- 只替换豆包 url 不能覆盖嵌套字段、其他厂商、旧数据与响应头，拒绝。
- 删除数据库原始响应会破坏轮询、取片和排障，拒绝。
- 匿名短链引入额外授权机制；现有 Key/会话下载已满足 Codex，不采用。

## Testing

真实查询夹具先复现豆包/Sora统一及兼容端点的域名泄露，再验证白名单投影。新增本地测试覆盖 Gemini 实时查询、Muse/Sora提交、失败消息、普通列表、HTTP响应头、HTTP200 JSON错误体拒绝、跨用户取片拒绝，以及管理员取片仍按原任务身份向上游鉴权。原始 Data/PrivateData 不变。relay/controller/dto/service/全部视频适配器测试及39项视频客户端测试通过；未发付费请求。域名修正补测四个元衡别名均归一到 `.com`，自定义域名、非标准端口和伪后缀不被归一，且全局配置不变。

## Consequences

依赖额外供应商字段的客户端需迁移到标准字段；保持正常站点的完整HTTPS地址，现有Skill可继续校验和自动下载；下载仍带原密钥。历史错误日志已有独立用户白名单；本次不删除管理日志或修改视频画面水印。代码修复需部署才生效。

## Related notes

[任务共同契约](../../implemented/bug-fix/2026-10-26-video-task-common-contract.md)与[成品下载](../../implemented/feature/2026-10-31-video-artifact-delivery.md)部分重叠；保留其轮询、资金和客户端下载决定，本篇增加公开输出边界。
