# Agent Note: 阿里云双向流式 TTS 与音色复用

Status: implemented

## Problem

阿里渠道的 ConvertAudioRequest 未实现，OpenAI Realtime 将所有消息视为 JSON，普通音频非 SSE 返回路径收齐响应后才写出。用户需要使用百炼复刻音色进行低延迟语音聊天，并要求提供配置与调用文档。

## Decision

扩展现有阿里渠道，支持 Qwen-Audio-TTS 与 CosyVoice 的 DashScope 任务协议。新增 GET /v1/audio/speech/stream?model=…，复用令牌鉴权、分组、模型权限、渠道选择和并发控制；查询模型锁定连接，run-task 校验模型并应用渠道映射。保留 POST /v1/audio/speech 的 OpenAI 请求格式，通过同一上游 WebSocket 协议逐块输出二进制或 SSE。

任务控制和二进制帧分开处理。一个连接顺序执行任务，支持 cancel 后新 task_id 复用，限制消息大小、任务数、文本量、空闲时间与连接寿命。断开后有界取消并收尾计量，不重放已经提交的文本。保持单一读写所有者并回收 goroutine。

用量只取上游累计快照，各任务去重后累加，一条连接对应一次结算与使用日志；字符计费映射为文本输入单位，Flash Token 计费拆为文本输入/音频输出。实时会话关闭信任预扣旁路，发送文本前按文本量保守预留额度，并随已确认用量补充预扣，额度不足停止任务。首版仅支持倍率计费，按次、时长与阶梯表达式明确拒绝，避免长连接内多任务含义不明确。音色预先在百炼创建，保存 voice ID；本次不新增声音训练、账号间音色共享或浏览器录音聊天界面。

## Existing capabilities and note audit

功能地图中的渠道管理、音频 DTO、Relay/controller、BillingSession、音频日志为复用入口。检索 WebSocket/TTS/MiMo：[响应请求 ID](../bug-fix/2026-10-20-response-request-id-integrity.md)部分重叠，继续保留本站 ID；[运行时文档](2026-10-12-runtime-documentation.md)与[文档内容覆盖](2026-10-25-docs-content-coverage.md)部分重叠，沿用原目录和语言门禁；反代迁移、厂商图标笔记无关。没有现成阿里 TTS 决策被取代。

## Alternatives considered

- 仅填写自定义地址可快速连通兼容接口，但无法转换 DashScope 事件和二进制帧，也不能正确结算，拒绝采用。
- 全部统一成 OpenAI Realtime 对客户端友好，但 TTS 分段输入并不是完整语音对话协议，会引入虚假的 VAD/ASR 支持。采用独立明确的语音流入口。
- 独立桥接服务可绕开现有适配器，但会重复密钥、权限与结算基础设施；在现有阿里渠道中扩展。

## Testing

测试使用本地 WebSocket 模拟上游验证分段音频提前到达、任务序列/模型锁定、取消复用、HTTP 二进制与 SSE、异常/断开收尾、累计用量去重、计费与退款边界。18个控制器场景覆盖 HTTP/双向流与钱包/订阅/独立令牌的完整、部分和未知用量；检查日志状态和资金实际变化。Go 相关包回归、专项 race、前端/文档构建通过；公开文档 Chrome 中英切换与指南链接可见。本地独立文档的远端动态模型列表不可用，静态教程仍正常展示。

## Consequences

官方用量字段可能随模型变化；只支持明确模型与协议，缺少终态/用量时报错并保留已确认消耗，不推算未知上游费用。真实百炼首包延迟、音色权限与账单仍需实测，模拟上游不能证明生产性能。

预留按每字符2个输入单位、Flash另加32个音频输出Token及请求窗口计算，最终只收上游报告值。低余额可能提前拒绝长文本，不能把保守预留当售价。连接最多100任务、10分钟，单条128KiB，单任务20万字符；断开最多2秒收尾。无新增SQL或表结构；本次结算测试使用SQLite，MySQL/PostgreSQL与真实付费供应商未实测。

扩展的是已有阿里渠道而非新增渠道类型；该 TTS 协议通过事件报告用量，不使用 Chat StreamOptions。模型协议元数据包含带 model 参数的 GET 地址，避免误示聊天模板。新增发送额度不足测试确认文本到达上游前就被取消，空 input 对象保留官方任务契约。
