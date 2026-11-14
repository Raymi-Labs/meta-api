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


## 本地照片与失败提示

独立视频脚本增加 `--first-frame start.png`、`--last-frame end.jpg`；尾帧必须配首帧，不得与 `--image/--images` 参考照片混用。帧模式缺省采用 adaptive，显式其他比例或像素 size 会提前报错。已有任务恢复、仅下载不需要原照片。

服务端对 data URI 严格解码 Base64，核对 PNG/JPEG/WebP 实际格式、检查尺寸，再完整解码检测损坏。网关资源预算为单张20 MiB、合计64 MiB、单边8192像素及总像素16×1024×1024；超限请先缩图。这些是网关资源预算，不代表厂商能力上限。HTTP(S)/asset 地址只做地址格式检查，不主动下载；远程图片大小、尺寸及权限仍由供应商验证。

豆包异步错误使用官方 error.code 分类，公开查询、用户任务列表和脚本分别展示输入审核未通过、输出审核未通过、参数/照片格式错误、通道权限或可用性问题、取消和过期。提示为固定安全文本，不公开供应商自由文本、请求 ID、密钥或签名地址；管理员仍可在原任务响应中诊断具体上游代码。未知代码保持通用失败，查询错误不被当作任务失败，也不触发新生成。

分类依据：[官方任务查询](https://docs.volcengine.com/docs/ark/get-video-generation-task-api?lang=zh)、[官方错误码](https://ark.volcengine.com/region:cn-beijing/docs/error-codes)。文档及本地模拟验证不等于生产通道已实测。
