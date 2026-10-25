# 图片与视频理解

视觉输入用于识别图片中的内容、提取文字和分析图表；这与生成图片是不同能力。在[模型广场](https://meta-api.vip/pricing)确认模型支持视觉输入和所需协议，把 `MODEL_ID` 替换为实际 ID。纯文本模型不能因为接收到图片字段就自动获得视觉能力。

## 使用本地图片

准备一张实际的 `input.png`，安装 Python `openai` 包后运行以下示例。Key 从环境变量读取，本地 PNG 被编码为数据 URL，避免把私人图片先上传到其他图床。

```python
import base64
import os
from openai import OpenAI

with open("input.png", "rb") as image:
    encoded = base64.b64encode(image.read()).decode("ascii")

client = OpenAI(
    base_url="https://meta-api.vip/v1",
    api_key=os.environ["API_KEY"],
)
result = client.chat.completions.create(
    model="MODEL_ID",
    messages=[{
        "role": "user",
        "content": [
            {"type": "text", "text": "描述图片内容，并指出无法确定的部分。"},
            {"type": "image_url", "image_url": {
                "url": f"data:image/png;base64,{encoded}"
            }},
        ],
    }],
)
print(result.choices[0].message.content)
```

## 输入与响应

Chat 的 `content` 使用内容块数组，其中 `text` 提供任务要求，`image_url.url` 提供模型支持的图片地址或数据 URL。JPEG 等文件应使用与实际格式匹配的 MIME 类型。远程 URL 必须能被服务端或上游访问，浏览器里可见的登录后地址、电脑本地路径不能直接当公开图片地址使用。

图片数量、尺寸、文件大小、`detail` 选项和费用计算依具体模型而异。多张图片应明确编号和比较目标；不要假设所有渠道都接受相同数量。响应通常仍在 assistant 的文本内容中，并非下载图片。模型可能误识别小字、表格或坐标，需要准确数据时应由应用复核。

## 视频与协议限制

部分接口结构支持视频内容，但实际能否解析视频 URL、接受何种格式和时长取决于模型与适配渠道，不能把图片示例的类型改成视频就视为兼容。本站没有为所有模型提供通用的视频抽帧服务。Messages、Responses 与 Gemini 各有多模态输入结构，应使用对应协议文档，避免混用内容块。

400 常见于模型不支持图片、MIME 与文件不符或请求超限；远程图片下载失败还需检查 URL 有效期和访问权限。先用一张较小、格式明确的图片验证，再增加复杂度。不要把图片中的文字当成可信的业务指令直接执行。

关联：[Chat Completions](/docs/reference/chat-completions)、[Gemini 原生 API](/docs/reference/gemini)、[图片生成与编辑](/docs/reference/images)、[视频生成](/docs/reference/video)。
