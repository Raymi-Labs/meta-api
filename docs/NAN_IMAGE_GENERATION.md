# NAN AI 生图使用教程

> AI 生图有三种用法：直接使用平台工作台、在本地 Codex 安装技能包，或自行发送 API 请求。新手优先使用平台工作台，需要批量生成或接入程序时再使用后两种方式。

## 快速选择

| 方式         | 适合谁                                                 | 需要准备               | 推荐程度       |
| ------------ | ------------------------------------------------------ | ---------------------- | -------------- |
| 平台 AI 生图 | 不写代码、希望直接上传参考图和下载结果                 | 登录平台               | 最适合新手     |
| 本地技能包   | 使用 Codex Desktop / CLI，希望用自然语言生成或修改图片 | Codex、API Key、技能包 | 最适合日常创作 |
| API 请求     | 开发者、自动化脚本、批量任务                           | API Key、HTTP 客户端   | 最适合集成     |

## 1. 在平台 AI 生图界面使用

### 1.1 进入工作台

1. 登录 `https://meta-api.vip/`。
2. 在左侧菜单找到 `聊天 -> AI 生图`。
3. 或直接打开 [平台 AI 生图](/console/image-playground)。
4. 系统会创建临时访问会话并自动进入生图工作台。

### 1.2 文生图

在输入框中写清楚以下内容：

- 主体：要画什么。
- 场景：时间、地点、环境。
- 风格：摄影、插画、海报、写实、3D 等。
- 构图：横图、竖图、特写、留白位置。
- 文字：需要出现在画面中的准确文案。
- 限制：不要水印、不要多余文字、不要改变产品形状等。

示例：

```text
生成一张 16:9 横版中文宣传海报。
主体是一台放在原木桌面的银色笔记本电脑，窗外是清晨城市天际线。
标题必须准确显示“让 AI 真正进入工作流”，副标题为“统一接入 · 灵活切换 · 按量计费”。
风格现代、克制、商业摄影感，右侧为主体，左侧保留文字区域。
不要水印，不要乱码，不要出现无关英文。
```

### 1.3 图片修改

上传原图后，再写清楚哪些内容必须保留、哪些内容需要修改：

```text
保持商品本体、角度、颜色和包装文字不变，只把背景替换成明亮的白色电商摄影棚。
增加自然柔和的落地阴影，不添加水印，不增加其他物体。
```

生成完成后先放大检查人物手部、商品结构、中文文字和边缘细节，再下载最终图片。

## 2. 在本地 Codex 使用技能包

技能包适合 Codex Desktop 和 Codex CLI。安装后可以直接用自然语言要求 Codex 生成海报、商品图、封面或修改本地图片。

### 2.1 下载技能包

[下载 `meta-api-imagegen-skill.zip`](/downloads/meta-api-imagegen-skill.zip)

文件校验值（SHA-256）：

```text
6458b0812e3791302709054527ac0a15e8f37cfeb8d3906191d08e032d72b454
```

### 2.2 macOS / Linux 安装

把下载的 ZIP 解压到 Codex 技能目录：

```bash
mkdir -p "${CODEX_HOME:-$HOME/.codex}/skills"
unzip ~/Downloads/meta-api-imagegen-skill.zip \
  -d "${CODEX_HOME:-$HOME/.codex}/skills"
```

安装后应存在：

```text
~/.codex/skills/meta-api-imagegen/SKILL.md
```

### 2.3 Windows 安装

在 PowerShell 中执行：

```powershell
New-Item -ItemType Directory -Force "$env:USERPROFILE\.codex\skills" | Out-Null
Expand-Archive `
  -Path "$env:USERPROFILE\Downloads\meta-api-imagegen-skill.zip" `
  -DestinationPath "$env:USERPROFILE\.codex\skills" `
  -Force
```

安装完成后，重新启动 Codex 或新建一个会话，让技能被重新加载。

### 2.4 配置 API Key

如果 Codex 已经通过 NAN 登录并保存了 Key，技能会优先读取现有 Codex 配置。也可以手动设置环境变量：

macOS / Linux：

```bash
export NAN_API_KEY="sk-替换为你的API密钥"
```

Windows PowerShell：

```powershell
$env:NAN_API_KEY="sk-替换为你的API密钥"
```

不要把真实 API Key 写进提示词、截图、仓库或公开文档。

### 2.5 直接对 Codex 提需求

生成图片：

```text
使用 meta-api-imagegen 技能，帮我生成一张 1536x1024 的中文活动海报。
标题是“夏日开发者计划”，整体是明亮的蓝绿色科技风，输出 PNG 到桌面。
```

修改图片：

```text
使用 meta-api-imagegen 技能修改桌面的 product.png。
保持产品本体不变，把背景换成干净的电商摄影棚，并输出 product-final.png。
```

也可以手动调用技能内的脚本：

```bash
python3 "${CODEX_HOME:-$HOME/.codex}/skills/meta-api-imagegen/scripts/meta_api_imagegen.py" \
  --prompt "生成一张现代中文科技产品海报，留出清晰标题区域，不要水印" \
  --size 1536x1024 \
  --quality high \
  --out output/imagegen/poster.png
```

修改本地图片：

```bash
python3 "${CODEX_HOME:-$HOME/.codex}/skills/meta-api-imagegen/scripts/meta_api_imagegen.py" \
  --image input/product.png \
  --prompt "保持产品本体不变，只替换为白色电商摄影棚背景" \
  --out output/imagegen/product-edited.png
```

## 3. 通过 API 请求生图

### 3.1 设置公共变量

```bash
export NAN_BASE_URL="https://meta-api.vip"
export NAN_API_KEY="sk-替换为你的API密钥"
```

### 3.2 推荐：Responses 生图

当前技能包默认使用经过验证的 Responses 流程：外层模型为 `gpt-5.5`，工具为 `image_generation`，并开启流式响应。

```bash
curl -N "$NAN_BASE_URL/v1/responses" \
  -H "Authorization: Bearer $NAN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.5",
    "input": [
      {
        "role": "user",
        "content": [
          {
            "type": "input_text",
            "text": "生成一张现代中文科技产品海报，深蓝背景，青色光效，16:9 横图，不要水印"
          }
        ]
      }
    ],
    "tools": [
      {
        "type": "image_generation",
        "size": "1536x1024",
        "quality": "high",
        "output_format": "png"
      }
    ],
    "tool_choice": {"type": "image_generation"},
    "stream": true
  }'
```

流式结果中应保存最终的 `image_generation_call.result`；如果没有该字段，再使用最后一个 `partial_image_b64`。不要在收到第一张局部预览时提前结束连接。

### 3.3 Responses 图片修改

先把本地图片转成 Base64 数据 URL，再放入 `input_image`：

```bash
IMAGE_B64=$(base64 < input.png | tr -d '\n')

jq -n --arg image "data:image/png;base64,$IMAGE_B64" '{
  model: "gpt-5.5",
  input: [{
    role: "user",
    content: [
      {type: "input_text", text: "保持主体不变，把背景改成白色电商摄影棚"},
      {type: "input_image", image_url: $image}
    ]
  }],
  tools: [{
    type: "image_generation",
    size: "1536x1024",
    quality: "high",
    output_format: "png"
  }],
  tool_choice: {type: "image_generation"},
  stream: true
}' | curl -N "$NAN_BASE_URL/v1/responses" \
  -H "Authorization: Bearer $NAN_API_KEY" \
  -H "Content-Type: application/json" \
  --data-binary @-
```

### 3.4 OpenAI Images 兼容接口

部分图像渠道支持直接调用 `/v1/images/generations`：

```bash
curl -sS "$NAN_BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NAN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "一只在上海雨夜骑自行车的橘猫，电影感，霓虹灯，高细节",
    "size": "1024x1024",
    "quality": "high",
    "n": 1
  }'
```

Imagen 模型使用同一接口，只替换模型名：

```bash
curl -sS "$NAN_BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NAN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "imagen-3.0-generate-002",
    "prompt": "极简东方茶室，清晨自然光，原木与白墙，建筑摄影",
    "size": "1024x1024",
    "n": 1
  }'
```

不同上游可能返回图片 URL，也可能返回 `b64_json`。程序接入时需要同时兼容这两种结果。

### 3.5 Grok 图片模型

当前 `grok-imagine-image` 使用 Responses 协议：

```bash
curl -sS "$NAN_BASE_URL/v1/responses" \
  -H "Authorization: Bearer $NAN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "grok-imagine-image",
    "input": "一座漂浮在云海上的未来图书馆，日出，宽银幕构图"
  }'
```

模型与协议可能随渠道调整。正式接入前，请先查看 [模型请求样例](./NAN_API_EXAMPLES.md)，或通过 `/api/pricing` 检查模型的 `supported_endpoint_types`。

## 4. 常见问题

| 现象                     | 优先检查                                        |
| ------------------------ | ----------------------------------------------- |
| `401` / `unauthorized`   | API Key 是否完整、是否误带空格、令牌是否启用    |
| `model not found`        | 模型名是否完全一致、令牌分组是否包含该模型      |
| `No available channel`   | 当前图像渠道是否在线、模型是否支持所选协议      |
| Responses 一直没有最终图 | 是否使用 `stream: true`，是否持续读取到最终事件 |
| 返回结果没有 URL         | 检查是否返回 `b64_json` 或流式 Base64 图片数据  |
| 中文文字乱码             | 缩短文字、明确逐字文案，生成后人工检查          |
| 本地技能未触发           | 检查技能目录层级，重启 Codex 或新建会话         |

提交问题时请提供发生时间、模型名、请求方式、完整错误文本和请求 ID，不要提供完整 API Key。
