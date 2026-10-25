# 图片生成与编辑

本站提供图片生成和图片编辑入口。先在[模型广场](https://meta-api.vip/pricing)确认模型支持的图片协议、输入方式与计费单位，将 `MODEL_ID` 替换为实际 ID。支持生成不代表同时支持编辑、透明背景、蒙版或流式图片；这些能力需要逐个模型确认。

## 文生图

`API_KEY` 从环境变量读取。先用一张图片的最小请求验证，再增加尺寸、质量等模型专属参数。

```bash
curl --fail-with-body -sS 'https://meta-api.vip/v1/images/generations' \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "MODEL_ID",
    "prompt": "一座安静的海边灯塔，水彩插画，柔和晨光。",
    "n": 1
  }'
```

`prompt` 描述内容，`n` 表示请求数量；允许的数量仍受模型限制。`size`、`quality`、`background`、`output_format` 和 `response_format` 的取值按所选模型填写，不能把其他厂商的参数枚举直接复用。

## 编辑现有图片

以下示例适用于支持 multipart 编辑协议的模型，需要本地存在 `input.png`。不要手动设置 Content-Type 的 multipart boundary，cURL 会自动生成。

```bash
curl --fail-with-body -sS 'https://meta-api.vip/v1/images/edits' \
  -H "Authorization: Bearer $API_KEY" \
  -F 'model=MODEL_ID' \
  -F 'prompt=保留灯塔主体，将天空改为柔和的晚霞。' \
  -F 'image=@input.png' \
  -F 'n=1'
```

蒙版、多参考图、保真度和透明区域规则依模型而异。需要 `mask` 时，应按该模型要求准备与原图兼容的文件；JSON 编辑接口则使用其对应字段，不能套用 multipart 的文件语法。`/v1/images/variations` 当前未实现。

## 保存结果与处理失败

兼容响应通常在 `data[]` 中提供 `url` 或 `b64_json`。前者是结果地址，应及时下载保存；不能假设永久有效。后者是编码后的图像数据，需要解码为二进制文件，扩展名应与实际输出格式一致。部分模型有其他响应结构或生成过程事件，应按模型说明处理。

400 常见于不支持的尺寸、格式、数量或缺少图片；413 表示上传体积超限；内容策略拒绝需要修改请求内容。图像生成可能耗时较长，连接超时并不自动证明任务未执行。先查使用日志和已有结果，避免立即重复提交造成重复生成与消耗。

关联：[生图接入指南](/docs/image-generation)、[图片理解](/docs/reference/vision)、[认证与 API Key](/docs/reference/authentication)、[故障排查](/docs/troubleshooting)。
