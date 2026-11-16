# 渠道响应头超时

## 管理入口

管理员编辑渠道 → 高级设置，分别配置 **非流式响应头超时（秒）** 与 **流式响应头超时（秒）**。

| 字段 | 对应请求 |
| --- | --- |
| `nonstream_response_header_timeout_sec` | 原有非流式文本与视频任务提交 |
| `stream_response_header_timeout_sec` | 流式 Chat Completions、Completions、Responses 和 Gemini 生成 |

两个字段独立，旧非流式设置不会改变流式请求的等待。以下边界对两个字段均适用。

- `0` / 留空 / 历史配置缺省：继承全局默认，不表示无限等待。
- `1`—`600` 的整数：覆盖该渠道对应请求等待上游响应头的时限。
- 负数、超上限、非整数配置后端拒绝。
- 存储在已有 `channels.setting` JSON，无需迁移数据库。

针对 Grok 的慢非流式请求，可在部署后仅对目标渠道的非流式等待试设 `120`；针对 Gemini 原生视频理解 SSE，可将目标渠道的流式等待试设 `180`，其余渠道保持 `0`。这些是人工试验值，不是新默认。

```json
{"nonstream_response_header_timeout_sec":0,"stream_response_header_timeout_sec":180}
```

必须先部署支持流式字段的应用，再在管理员渠道界面保存；旧版本无法应用新字段。将流式值恢复 `0` 即继承全局。代码不会硬编码生产渠道 ID 或自动修改线上渠道。管理员新增/更新接口的鉴权不变。

## 生效范围

非流式字段保持原范围：使用公共 HTTP 转发器的非流式 Chat Completions、Completions、Responses 文本请求和视频任务提交。

流式字段作用于公共 HTTP 转发器的流式 Chat Completions、Completions、Responses 和 Gemini 生成入口，包括 `/v1beta/models/{model}:streamGenerateContent` 中的视频理解；它不依赖模型前缀或供应商名字。Images API、Responses 图片工具、音频接口、WebSocket、后台任务轮询不采用流式覆盖；原生 Gemini 非流式请求也不因此扩大旧字段的范围。

流式/非流式按 relay 当前请求状态识别；协议桥接可能内部采用流式，选择对应的配置。覆盖仅改变等待响应头阶段，不改变协议。

不是模型执行速度、首字时限或响应体总时限。上游返回响应头后，本配置不限制继续读取响应体。`RELAY_TIMEOUT`、客户端context、Nginx/CDN超时仍可能先结束请求。不会为了延长等待强制修改它们。

代理/TLS/重定向校验、客户端总超时保持。SOCKS握手使用上下文拨号，以便取消时及时释放连接。覆盖客户端不修改共享transport；有界LRU缓存保留连接复用，最多32组基础客户端/时限组合。淘汰与重载关闭空闲连接，不打断在途请求。

## 日志和风险

消费与最终失败日志管理员 `admin_info.relay_http_timeout` 包含：

- `response_header_timeout_seconds`：该次客户端的响应头时限；
- `client_total_timeout_seconds`：原有客户端总时限，0表示客户端未设总时限；
- `channel_nonstream_override`：本次是否使用非流式渠道覆盖。
- `channel_stream_override`：本次是否使用流式渠道覆盖；两个覆盖标记不会同时为 true。

这不是一条跨所有重试的时序记录，只反映该次记录关联的最近HTTP客户端选择。普通用户日志仍清除管理员字段。重试分类、预扣/退款、价格、路由和并发额度不变。本选项不扩大重试范围；已支持的 Chat/Messages 一次安全切换仍遵循原门槛，Gemini 原生请求不因延长等待而新增自动重试。

延长会占用连接与并发名额更久，不保证上游能在新时限内完成；上线后按请求ID观察超时率、耗时及并发压力。出现问题可将该字段恢复0，不需回滚其他渠道。

## 验证边界

本地模拟验证默认短时限失败、覆盖后成功、慢响应体不会按响应头时限截断、取消、总时限、HTTP代理/重定向、SOCKS取消、缓存有界与并发隔离。浏览器模拟管理员编辑完成无修改提交、改值再打开、清空继承，原分组及未知setting保留。无收费上游测试；MySQL/PostgreSQL未实机验证，但没有新增SQL或schema迁移。

流式扩展的本地验证包括原生 Gemini SSE 默认短时限失败、覆盖后完整终态成功、并发未配置渠道仍按短时限失败、两个覆盖字段互不替代及诊断标记。真实管理员表单验证保存、重开、清空为0和未知 setting 保留；这些不代表供应商请求已验收或生产已部署。
