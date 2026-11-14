# 元衡 API · Seedance 2.5 应用联调文档

文档日期：2026-11-15。适用服务端：v1.31.1 及包含相同视频协议的后续版本。

本文面向第三方应用的服务端开发者。接入目标为元衡 API，本文的统一字段不能直接作为火山方舟或其他供应商原生请求发送。示例不含真实密钥、用户任务 ID 或私有素材。

## 1. 接入信息与验收范围

| 项目 | 值 |
| --- | --- |
| API 根地址 | `https://meta-api.com` |
| 推荐模型 ID | `doubao-seedance-2-5-260628` |
| 鉴权 | `Authorization: Bearer <META_API_KEY>` |
| 创建任务 | `POST /v1/videos`，JSON 请求 |
| 查询任务 | `GET /v1/videos/{task_id}` |
| 下载成片 | `GET /v1/videos/{task_id}/content`，仍需鉴权 |
| 检查可见模型 | `GET /v1/models` |
| 推荐首次联调参数 | 4 秒、720p、9:16、单张参考图 |

Key 必须是元衡平台的有效 API 令牌，并具备对应模型、分组与足够额度。上游供应商 Key 不能替代元衡 Key。模型出现在 `/v1/models` 表示该令牌可见，不保证每次生成成功或所有参数组合可用。

本文的照片角色映射适用于豆包视频/火山渠道实际映射到 `doubao-seedance-2-5-260628` 或 `dreamina-seedance-2-5-260628` 的请求。对外调用仍使用该账号获准的模型 ID；国际区型号及自定义别名需要管理员配置，不能仅自行替换名称就假定可调用。

| 能力 | 当前实现与验证 |
| --- | --- |
| 单张参考图 | **已线上实测**：2026-11-15，WebP data URI、4 秒、720p、9:16；生成并鉴权下载成功，实际 4.074 秒、720×1280，含一条音轨 |
| 参考图效果 | 抽帧检查保留银白长发、蓝白服饰、几何徽章，完成抬手挥动与眨眼；不构成所有素材或动作的质量保证 |
| 多张参考图 | 服务端已实现、数量/角色/互斥通过本地测试；尚未做本轮线上生成验收 |
| 首帧、首尾帧 | 服务端及独立视频脚本已实现、通过本地测试；尚未做本轮线上生成验收 |
| 参考视频/音频、编辑、延长 | 本文对应的照片适配链路尚未实现，不要在产品界面宣称已接通 |
| 真人照片、其他时长/分辨率 | 需根据实际渠道权限、素材规则与定价进行单独验收；没有实测不等于模型不支持 |

## 2. 推荐调用流程

1. 由业务后端保管 Key，建立本地作业记录与唯一业务编号。
2. 校验照片和参数，调用一次 `POST /v1/videos`。
3. 收到有效 `id` 后立即持久化；创建成功只表示受理，不能显示“视频完成”。
4. 使用同一个 Key 和任务 ID，每 10–15 秒查询状态。
5. `completed` 后调用鉴权内容接口，检查下载响应并保存成片。
6. 生成成功和下载/归档成功分别记录；下载失败只重试下载，不重新生成。

**创建接口没有本文承诺的幂等键机制。** 禁止在 HTTP 客户端、SDK、队列或反向代理中自动重试生成 POST。提交超时、断线或没有拿到有效 ID 时，记录为“提交结果不确定”，先核对任务/使用日志，不能直接再创建一个任务。

## 3. 创建任务

### 3.1 请求头

```http
Authorization: Bearer <META_API_KEY>
Content-Type: application/json
```

API Key 仅置于业务后端环境变量或密钥管理服务。不要写入浏览器、移动端包、URL、源代码、前端 localStorage 或请求日志。

### 3.2 推荐字段

| 字段 | JSON 类型 | 要求与说明 |
| --- | --- | --- |
| `model` | string | 必填；使用账号获准的完整模型 ID |
| `prompt` | string | 必填且非空；描述动作、场景与需要保持的特征 |
| `mode` | string | 建议显式提供：`text_to_video`、`reference`、`first_frame`、`first_last_frame` |
| `images` | string[] | 图像 URL 或完整 data URI 数组；顺序对应提示词里的参考图编号或首尾帧顺序 |
| `seconds` | **string** | 建议显式提供，例如 `"4"`；不是 JSON 数字 |
| `resolution` | string | 建议首次用 `"720p"`；使用小写 p；其他档位需确认上游与本站定价 |
| `aspect_ratio` | string | 参考模式可从 `"9:16"` / `"16:9"` 开始；首帧/首尾帧用 `"adaptive"` |
| `metadata.generate_audio` | boolean | 可选，例如 `true`；表示请求生成声音，不是上传参考音频 |
| `metadata.watermark` | boolean | 可选；透传给渠道，实际效果由上游决定 |

兼容字段：`duration` 为整数秒，可替代 `seconds`；不要同时发送两者。单张图片可用字符串 `image` 或 `input_reference` 替代 `images`，不要混用。新集成建议统一使用上表字段。

本协议不用 `size: "720x1280"` 表达输出档位，使用 `resolution` 与 `aspect_ratio`。不要把其他视频厂商的 `image_url`、`input.media`、`first_frame_url` 等字段直接放到此统一请求顶层。

### 3.3 模式与图片语义

| `mode` | `images` | 比例要求 |
| --- | --- | --- |
| `text_to_video` | 不传照片 | 按已确认的渠道参数选择 |
| `reference` | 1–30 张参考照片 | 首次联调建议 9:16 或 16:9 |
| `first_frame` | 恰好 1 张，指定起始画面 | adaptive；省略时服务端补充 |
| `first_last_frame` | 恰好 2 张，第一张首帧、第二张尾帧 | adaptive；省略时服务端补充 |

有照片而省略 `mode` 时，统一照片默认按参考素材处理，**一张图也不自动变成首帧，两张图也不自动变成首尾帧**。

参考模式借用人物、物体、服饰或风格特征，不保证生成视频的第一帧逐像素复制原图。需要固定第一帧时应明确选择 `first_frame`。

首尾帧不能与参考素材混用。已有高级集成的 `metadata.content` 显式角色仍可使用，但不可同时传统一照片字段；新应用优先使用本文的 `mode + images`。

### 3.4 单张参考图：已实测路径

```json
{
  "model": "doubao-seedance-2-5-260628",
  "prompt": "以参考图中的角色为唯一人物参考，保留发色、服装配色和胸前饰品。面向镜头轻轻抬起右手挥手一次，镜头固定，不添加其他人物或文字。",
  "mode": "reference",
  "images": ["https://your-cdn.example/reference.webp"],
  "seconds": "4",
  "resolution": "720p",
  "aspect_ratio": "9:16",
  "metadata": {"generate_audio": true}
}
```

`your-cdn.example` 是占位域名，必须换成上游能读取的真实素材地址。此次线上实测使用的是本地 WebP 编码后的 data URI，而不是示例中的远程 URL。

### 3.5 多张参考图

```json
{
  "model": "doubao-seedance-2-5-260628",
  "prompt": "保留参考图1中的人物设计，让人物在参考图2中的场景里缓慢转身。",
  "mode": "reference",
  "images": ["https://your-cdn.example/character.png", "https://your-cdn.example/scene.jpg"],
  "seconds": "4",
  "resolution": "720p",
  "aspect_ratio": "16:9"
}
```

### 3.6 首帧与首尾帧

首尾帧请求：

```json
{
  "model": "doubao-seedance-2-5-260628",
  "prompt": "从起始画面自然过渡到结束画面，保持人物与服装一致。",
  "mode": "first_last_frame",
  "images": ["https://your-cdn.example/start.png", "https://your-cdn.example/end.png"],
  "seconds": "4",
  "resolution": "720p",
  "aspect_ratio": "adaptive"
}
```

仅首帧时，把 `mode` 改为 `first_frame`，数组只保留首图。`return_last_frame` 表示要求返回成片尾帧，不是传入尾帧，也不是本指南承诺的公开输出字段。

### 3.7 创建响应

成功通常为 HTTP 200。以下仅摘录可用于继续查询的字段：

```json
{
  "id": "task_EXAMPLE",
  "task_id": "task_EXAMPLE",
  "object": "video",
  "model": "doubao-seedance-2-5-260628"
}
```

保存 `id`；`task_id` 是兼容别名。不要依赖创建响应中 `status` 或 `progress` 已完整填充，应立即转入查询流程。任务 ID 是不透明字符串，不解析、不自行拼接、不换成上游任务编号。

## 4. 照片输入与限制

支持 PNG、JPEG、WebP。data URI 格式为：

```text
data:image/png;base64,<PNG二进制的标准Base64>
data:image/jpeg;base64,<JPEG二进制的标准Base64>
data:image/webp;base64,<WebP二进制的标准Base64>
```

不要只发送裸 Base64，不要把 Windows/macOS 本地路径或浏览器 `blob:` URL 发给上游。MIME 类型必须与图片真实内容一致。

| 校验项 | 当前网关规则 |
| --- | --- |
| 参考照片数量 | 最多 30 张；首帧/首尾帧另按模式限制 |
| 单张内嵌图片 | 解码后最多 20 MiB |
| 内嵌图片总量 | 解码后合计最多 64 MiB |
| 单边尺寸 | 不超过 8192 像素 |
| 单张总像素 | 不超过 16×1024×1024 |
| 内容验证 | 严格 Base64、实际格式、尺寸检查、完整图像解码 |

这些是网关资源预算，不代表供应商能力上限。Base64 请求体比原始文件更大，仍受部署侧请求体大小限制；30 张不是任何文件大小组合都一定能提交。

HTTP(S)/asset 地址只做格式检查，本适配器不主动抓取远程素材。远程文件大小、格式、尺寸、有效期和素材权限由供应商验证。通用联调优先使用 HTTPS 公网素材或 data URI；`asset://` 仅在对应渠道确认素材资源归属与权限后使用。不要给素材下载请求附加元衡 API Key。

## 5. 查询与状态机

```http
GET /v1/videos/{task_id}
Authorization: Bearer <META_API_KEY>
```

| `status` | 应用行为 |
| --- | --- |
| `queued` | 排队；间隔查询 |
| `in_progress` | 生成中；间隔查询 |
| `completed` | 生成完成；进入鉴权下载 |
| `failed` | 终态失败；读取 `error`，停止轮询，不自动重新提交 |
| `unknown` / 未识别状态 | 保留原任务号，有限次数继续查询或提示人工核对；不能当作成功或直接重建任务 |

`progress` 是参考进度，不是精确剩余时间。HTTP 429、暂时网络错误、5xx 可对 **GET 查询**有界退避；如有 `Retry-After` 则遵守。401/403 应停止并检查凭据/权限。404 需核对原任务 ID、账号及保留期限，不可当作需要重新创建的依据。

应用等待超时只表示本地不再等待，不等于上游已取消。当前本文流程不提供取消 API；不要对用户宣称“已取消生成”。

成功响应示意：

```json
{
  "id": "task_EXAMPLE",
  "object": "video",
  "model": "doubao-seedance-2-5-260628",
  "status": "completed",
  "progress": 100,
  "metadata": {
    "url": "https://meta-api.com/v1/videos/task_EXAMPLE/content"
  }
}
```

不要要求结果必须包含 `seconds`、`size`、上游原始 URL 或其他供应商字段；实际时长、画幅、帧率与音轨以下载文件为准。

## 6. 下载与交付

```http
GET /v1/videos/{task_id}/content
Authorization: Bearer <META_API_KEY>
```

使用原任务对应的元衡账号/令牌，保持相同计费身份；服务端会检查任务归属和完成状态。

- 在 `completed` 后调用；检查 HTTP 成功和视频内容类型，流式写入临时文件，完整下载后再交付。
- 下载失败保留“生成完成”状态，重新请求同一内容接口；不能调用生成 POST 来修复下载。
- 不自动携带鉴权头跟随跨域重定向，也不把 Key 发给上游 CDN。
- 地址不是匿名分享链接，直接放进没有 Authorization 的 `<video src>` 通常不能播放。浏览器应用应由自己的后端校验用户后代理内容，或归档到受控存储后签发自己的访问链接。
- 本站内容接口当前主要代理上游成片，不等于长期保存。完成后及时下载/转存；不要承诺永久有效或无限次下载。

## 7. curl 快速联调

先通过环境或密钥管理器注入 `META_API_KEY`。以下命令不包含真实 Key；不要开启会回显 Authorization 的 shell 跟踪或 HTTP 调试日志。

**检查可见模型：**

```bash
curl --fail-with-body --silent --show-error --max-time 30 \
  'https://meta-api.com/v1/models' \
  -H "Authorization: Bearer ${META_API_KEY:?请先设置META_API_KEY}"
```

将第 3 节的请求保存为 `request.json`，替换照片地址后，**只执行一次创建**：

```bash
curl --fail-with-body --silent --show-error --max-time 120 --retry 0 \
  'https://meta-api.com/v1/videos' \
  -H "Authorization: Bearer ${META_API_KEY:?请先设置META_API_KEY}" \
  -H 'Content-Type: application/json' \
  --data-binary @request.json \
  --output create-response.json
```

仅在 HTTP 成功且返回有效 `id` 后保存它。提交超时或失败响应不能直接重复执行上面的命令。

**查询原任务：**

```bash
VIDEO_TASK_ID='task_替换为实际任务ID'
curl --fail-with-body --silent --show-error --max-time 30 \
  "https://meta-api.com/v1/videos/${VIDEO_TASK_ID}" \
  -H "Authorization: Bearer ${META_API_KEY:?请先设置META_API_KEY}"
```

**状态为 completed 后下载：**

```bash
curl --fail-with-body --silent --show-error --max-time 120 \
  "https://meta-api.com/v1/videos/${VIDEO_TASK_ID}/content" \
  -H "Authorization: Bearer ${META_API_KEY:?请先设置META_API_KEY}" \
  --dump-header video-download.headers \
  --output result.mp4.part
```

确认命令退出码为 0、响应为视频文件且下载完整后，再将 `.part` 文件改名为 `result.mp4`。下载失败时 `.part` 可能含错误响应，不能直接拿它当成片。

## 8. 本地照片：Python 编码示例

以下代码只生成请求文件，不调用 API、不产生生成费用。Python 3.9+，仅依赖标准库。

```python
import base64
import json
import os
from pathlib import Path

photo_path = Path("reference.webp")
raw = photo_path.read_bytes()
if not raw or len(raw) > 20 * 1024 * 1024:
    raise ValueError("请使用不超过20 MiB的有效图片")
# 示例使用WebP；PNG/JPEG应相应改为image/png或image/jpeg，不能只改后缀冒充格式。
reference = "data:image/webp;base64," + base64.b64encode(raw).decode("ascii")
payload = {
    "model": "doubao-seedance-2-5-260628",
    "prompt": "保持参考图中人物的发色与服装，面向镜头轻轻挥手，镜头固定。",
    "mode": "reference",
    "images": [reference],
    "seconds": "4",
    "resolution": "720p",
    "aspect_ratio": "9:16",
    "metadata": {"generate_audio": True},
}
# 避免覆盖旧请求；文件含照片数据，应按用户素材管理，不记录到公共日志。
fd = os.open("request.json", os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, "w", encoding="utf-8") as stream:
    json.dump(payload, stream, ensure_ascii=False)
```

需要自动提交、轮询、恢复和下载，可直接使用项目提供的独立视频客户端：[下载 ZIP](https://meta-api.com/downloads/meta-api-video-skill.zip)。解压后在 `meta-api-video` 目录运行：

```bash
python3 scripts/meta_api_video.py \
  --model doubao-seedance-2-5-260628 \
  --image reference.webp --duration 4 --resolution 720p --aspect-ratio 9:16 \
  --prompt '保持人物设计与服装，让角色轻轻挥手。' \
  --state output/task.json --out output/result.mp4 --wait 900
```

先加 `--dry-run` 可做无网络预检。每个新任务使用独立 state 文件，不能删除旧 state 后重跑来“恢复”。

首尾帧调用：

```bash
python3 scripts/meta_api_video.py \
  --model doubao-seedance-2-5-260628 \
  --first-frame start.png --last-frame end.jpg --duration 4 --resolution 720p \
  --prompt '从起始画面自然过渡到结束画面。' \
  --state output/frames-task.json --out output/frames.mp4 --wait 900
```

首尾帧默认 adaptive，不与 `--image/--images` 混用。恢复原任务使用 `--resume --state output/task.json --out output/result.mp4`；仅重新下载使用 `--download-only`，不会再生成。恢复应保持原 Key、域名和 state，已存在的输出文件不会覆盖。

## 9. 错误处理

### 9.1 HTTP 错误

HTTP 非 2xx 与异步 `failed` 是两个层次，不能只检查其中一个。非 2xx 的响应格式可能因鉴权、网关或代理层不同而变化，不要假设所有错误都有同一 JSON 结构。

| HTTP 状态 | 排查方向 |
| --- | --- |
| 400 | 字段类型、模式/素材冲突、缺参数、格式错误 |
| 401 | Key 无效、过期、禁用或并非此平台令牌；不能据此判定模型不支持参考图 |
| 403 | 账号、分组或访问权限；不要自动换 Key 绕过 |
| 404 | 端点/任务 ID、任务归属、保留期限 |
| 429 | 请求频率或容量；GET 可退避，生成 POST 不自动重发 |
| 5xx / 网络超时 | 服务或上游异常；生成提交结果可能不确定，先核对原任务 |

上表是排查方向，不表示所有参数错误都必定返回 400；实际还应保留 HTTP 状态、可用的错误码和公开任务号用于诊断。

### 9.2 异步失败

```json
{
  "id": "task_EXAMPLE",
  "status": "failed",
  "error": {
    "code": "video_input_rejected",
    "message": "Input content did not pass review; check the prompt and photos"
  }
}
```

| `error.code` | 建议展示与处理 |
| --- | --- |
| `video_input_rejected` | 输入审核未通过；检查提示词、素材及对应素材权限 |
| `video_output_rejected` | 生成内容审核未通过 |
| `video_invalid_input` | 参数或照片格式被拒绝 |
| `video_access_unavailable` | 当前通道权限或可用性有问题，提供任务 ID 联系管理员 |
| `video_cancelled` | 上游确认任务已取消 |
| `video_expired` | 上游任务已过期/超时 |
| `video_generation_failed` | 未分类失败，提供任务 ID 排查 |

公开接口只返回固定安全提示，原始供应商消息不公开。应用按 `code` 做本地化，不依赖英文 `message` 的逐字匹配。已识别错误的分类不等于永久能力判定。

## 10. 计费、恢复与联调清单

生成按实际模型、分组与管理员定价计费，不在客户端硬编码单价。请求秒数、媒体实际时长和最终扣费是不同字段；不能根据本次4.074秒成片推断所有渠道都会按实际秒数结算。任务失败也不能仅凭客户端状态承诺零扣费，应核对对应任务与账单。

业务侧至少保存：本地作业 ID、元衡任务 ID、账号/令牌的非秘密标识、模型、模式、请求参数摘要、提交状态、生成状态、归档状态。不要保存或回显 Key、完整 data URI 或素材签名参数。

验收时检查：

- Key 有效、模型可见；提交后得到任务 ID，页面刷新恢复同一个任务。
- 单图、多图、首帧、首尾帧分别测试，不凭其中一种成功宣称全部可用。
- 切换模式不会带上旧素材；数量超限、角色冲突和坏图在提交前得到提示。
- 只提交一次；网络断开、429、查询失败、下载失败都不会再次创建生成任务。
- `completed` 后鉴权下载成功，播放并检查实际画面、时长、尺寸与声音。
- 成片及时归档，跨用户不可取片；保存的 Key 不出现在前端或日志中。
- 核对该任务的最终计费记录，不以 HTTP 200 作为财务结算证明。

兼容入口 `/v1/video/generations` 仍保留，但查询响应结构不同；新集成统一使用本文的 `/v1/videos` 路径，不混用状态与响应解析。本文未承诺 Webhook、公开永久下载地址、取消接口或幂等重试。

---

维护依据：本仓库视频路由、豆包适配器、公开响应边界和 v1.31.1 单图线上验收。上游参考：[官方任务查询](https://docs.volcengine.com/docs/ark/get-video-generation-task-api?lang=zh)、[官方错误码](https://ark.volcengine.com/region:cn-beijing/docs/error-codes)。上游文档用于解释能力来源；其他应用联调以本文的元衡接口合同为准。
