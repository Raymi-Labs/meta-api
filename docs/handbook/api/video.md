# 视频生成与异步任务

视频通常采用先提交、再查询、最后取结果的异步流程。先在[模型广场](https://meta-api.vip/pricing)确认模型支持的视频协议和分组，把 `MODEL_ID` 替换为实际 ID。本站有 OpenAI 兼容视频入口及部分厂商接口，参数和响应不完全相同；不能将所有视频模型统一套入同一请求。

## 一句话生成视频

安装[元衡视频 Skill](https://meta-api.vip/downloads/meta-api-video-skill.zip)到客户端 Skills 目录，配置自己的 `META_API_KEY`，直接描述想要的视频。需要 Python 3.9+；支持 Grok 文生视频（默认 1 秒、480p、16:9）和 Muse 文生/图生视频（默认请求 4 秒、720p），生成按模型和分组计费。

脚本只提交一次并自动查询。中断后用 `--resume` 继续原任务；没有任务 ID 时先查日志，不自动重发。

## 提交与查询

下面是 Grok 的 JSON 示例。其他模型须按各自协议选择 JSON 或 multipart，不能通用。

```bash
curl --fail-with-body -sS 'https://meta-api.vip/v1/videos' \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "grok-imagine-video",
    "prompt": "海边灯塔在晨光中，镜头缓慢向前移动。",
    "duration": 1,
    "resolution": "480p",
    "aspect_ratio": "16:9"
  }'
```

保存返回的任务 `id`，将下方 `TASK_ID` 替换为该值。任务 ID 表示提交已被接受，不代表视频已经生成成功。

```bash
curl --fail-with-body -sS 'https://meta-api.vip/v1/videos/TASK_ID' \
  -H "Authorization: Bearer $API_KEY"
```

该协议的结果包含 `status`，并可能提供 `progress`、`created_at`、`completed_at` 和 `error`。按实际返回判断排队、运行、完成或失败。轮询应使用间隔与退避，切勿通过重复提交新任务来“刷新进度”。

## 获取成品

Grok 完成后可从 `metadata.url` 获取视频链接。

确认任务完成后，再调用内容接口。结果获取需要鉴权，下载地址与任务结果也不应假设永久保留。

```bash
curl --fail-with-body -sS 'https://meta-api.vip/v1/videos/TASK_ID/content' \
  -H "Authorization: Bearer $API_KEY" \
  --output result.mp4
```

下载前检查 HTTP 状态，失败响应可能是 JSON，而非视频文件。参考图、尺寸、时长、重混等能力由模型决定；重混路由为 `/v1/videos/:video_id/remix`，不能据此推断所有视频模型都可使用。

## 其他协议与失败边界

本站也提供 `/v1/video/generations` 及对应查询接口，部分渠道还有独立的厂商路径。它们的 `duration`、`seconds`、宽高和参考图字段不可互换。不要直接拼接上游厂商地址，凭据始终只发往本站授权接口。

400 通常是模型、参数或输入文件不匹配；任务失败要读取 `error` 并查询使用日志；查询不到结果应核对任务 ID 和所用协议。超时不等于提交未发生，应用应保存已返回的 ID 并继续查询。费用按实际任务与日志记录处理，不能承诺任意失败都不会产生消耗。

关联：[认证与 API Key](/docs/reference/authentication)、[图片生成](/docs/reference/images)、[模型与协议](/docs/reference/models)、[故障排查](/docs/troubleshooting)。


## Muse 图生视频

`muse-video` 已通过元衡线上PNG图生验收。令牌使用“生图和视频”分组，POST `/v1/videos`：

```json
{
  "model": "muse-video",
  "prompt": "保持画面主体，让镜头缓慢向前推进",
  "seconds": "4",
  "size": "1280x720",
  "input_reference": "data:image/png;base64,图片Base64内容"
}
```

参数中不能填本地路径；程序读取图片后编码。保存任务ID，沿用上文查询和鉴权下载步骤。Skill支持 `--model muse-video --image reference.png`，自动提交和查询。请求时长支持4–30秒整数，720p；实际时长可能调整，本次4秒请求产出5秒、24fps视频。PNG已线上验证，JPEG参数编码已离线验证。

## Grok 首帧参数兼容

服务端修复支持官方 `"image":{"url":"https://example.com/frame.png"}`，也支持data URI；字符串 `image` 或 `input_reference` 会转换为官方格式。只接受一个明确首帧，冲突、空值、错误对象会在提交前拒绝，其他厂商格式不变。图片决定Grok输出比例。

此兼容需要部署对应版本：2026-10-31线上测试时两个Grok型号仍返回image对象解析错误；本地修复不代表线上已验收。Grok的Skill `--image` 同样依赖此修复，不会失败后自动改成文生视频或换厂商。
