# Seedance 2.5 参考照片

豆包视频/火山渠道映射到 `doubao-seedance-2-5-260628` 或 `dreamina-seedance-2-5-260628` 时，统一视频接口支持参考照片：

```json
{
  "model": "doubao-seedance-2-5-260628",
  "prompt": "以参考图1中的人物和参考图2中的场景生成视频",
  "mode": "reference",
  "images": ["https://example.com/character.png", "https://example.com/scene.jpg"],
  "seconds": "5",
  "resolution": "720p",
  "aspect_ratio": "16:9"
}
```

示例 URL 必须替换为上游可读取的实际照片；也接受 PNG/JPEG/WebP 的 data URI。已有视频 Skill 使用 `--image` 或 `--images` 即可编码本地照片。不要发送本机路径或浏览器 blob URL。

`mode` 省略时，照片按参考素材提交（最多30张），一张也不会被自动当作首帧。单张别名 `image`、`input_reference` 同样可用，但不要与 `images` 混用。明确首帧使用 `mode=first_frame` 和一张图片；首尾帧使用 `mode=first_last_frame` 和按顺序排列的两张图片，两种帧模式使用 `aspect_ratio=adaptive`。

已有 `metadata.content` 的显式角色继续支持，但不能同时传统一照片字段，首尾帧也不能混合参考照片。限制在提交收费任务前检查，实际供应商素材权限、分组和地域仍以渠道响应为准；本次不代表参考视频/音频、编辑和延长已接通，也不改变其他型号及第三方 Sora 兼容通道。
