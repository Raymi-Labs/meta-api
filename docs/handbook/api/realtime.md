# Realtime 实时接口

Realtime 使用 WebSocket 建立持续双向连接，适用于模型支持的实时交互。本站入口为 `wss://meta-api.vip/v1/realtime`。先在[模型广场](https://meta-api.vip/pricing)确认模型和渠道支持实时协议，再将 `MODEL_ID` 替换为实际 ID。普通 Chat、音频合成和 Realtime 是不同协议，不能混用请求结构。

## 最小连接检查

在受信任的服务端或本地开发环境安装 Python `websocket-client` 包。以下代码建立连接并读取首个事件，不发送生成任务；它用于检查握手和事件通道，不能代替完整音频对话验收。

```python
import os
from websocket import create_connection

ws = create_connection(
    "wss://meta-api.vip/v1/realtime?model=MODEL_ID",
    header=[
        f"Authorization: Bearer {os.environ['API_KEY']}",
        "OpenAI-Beta: realtime=v1",
    ],
    timeout=30,
)
try:
    print(ws.recv())
finally:
    ws.close()
```

这里使用兼容协议常见的握手头；模型或上游协议版本有差异时，应按其要求调整。超时参数只是客户端示例设置，不表示本站承诺的会话时长或响应时限。不要把长期 API Key 嵌入网页、移动应用安装包或公开仓库。

## 会话中的数据

连接后通过 JSON 事件交换状态。常见事件包括配置会话的 `session.update`、创建输入项的 `conversation.item.create` 和请求生成的 `response.create`，服务端返回相应确认、输出增量或错误事件。字段结构、音频格式、语音活动检测和可用模态由实时模型及协议版本决定，不能直接照搬 Chat 的 `messages` 请求。

音频应用还需处理采集、编码、分片、播放缓冲和打断。应根据事件类型分别处理文本、音频、工具参数与完成状态；函数工具仍由应用验证并执行。保持一条连接并不保证永远有效，应用应监听关闭和错误，保存必要的业务状态，并设计可控的重连流程。

## 认证和错误边界

本站有 WebSocket 鉴权入口，但没有可据此承诺的通用临时凭证签发接口。面向浏览器的产品宜由自己的服务端管理凭据与授权，避免把长期 Key 放进 URL、前端日志或浏览器可读配置。

握手失败时依次检查模型、Key、分组权限以及代理是否允许 WebSocket 升级；401 表示鉴权问题，429 应降低并发并退避。收到错误事件不能当作普通对话文本显示。断线后不要无条件重放可能已执行的工具动作，也不要把尚未确认完成的音频片段标记为完整回答。

关联：[语音接口](/docs/reference/audio)、[流式输出](/docs/reference/streaming)、[工具调用](/docs/reference/tools)、[认证与 API Key](/docs/reference/authentication)。
