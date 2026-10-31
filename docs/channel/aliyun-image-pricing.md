# 阿里云图片模型分类与定价

渠道选择阿里云（DashScope），请求入口为 `/v1/images/generations` 或 `/v1/images/edits`。Qwen Image 3.0、Wan 2.7 Image 与 Z-Image 使用图片测试；真正的视频型号、映射为视频的别名或显式视频端点仍只做免费配置预检。图片测试会调用上游，可能产生阿里云费用，本站缺价时会先阻止请求。

## 配置本站售价

在“系统设置 → 分组与模型定价设置 → 模型定价设置”选择模型。

- **qwen-image-3.0-pro / qwen-image-3.0**：选择“图片输入 / 输出计费”，分别填入 1K、2K 的输入图与输出图单价，单位 **USD/张**。四项都需填写；免费项目明确填写 `0`。点击“应用更改”后生效，重新打开仍可编辑。
- **wan2.7-image-pro**：使用按次配置，输入框按“输出图片单价 / USD/张”标注。阿里渠道按成功返回的图片数结算。
- **z-image-turbo**：填写标准模式的输出图片单价；阿里渠道 `parameters.prompt_extend=true` 时按 2 倍单价计费。官方仅支持文生图，不提供输入参考图计费项。

参考价格不会自动写入本站售价，也不会自动换汇。现有固定价不会自动迁移。需要输入和输出分别计费时，管理员须显式切换模式并保存；分组及时段倍率仍在分项费用之和上应用。

## 北京地域官方参考价

2026-10-31 核对，单位为人民币元/张，不含限时优惠；其他地域应查看官方文档。

| 模型 | 档位 | 输入图 | 输出图 |
| --- | --- | --- | --- |
| qwen-image-3.0-pro | 1K | 0.02 | 0.25 |
| qwen-image-3.0-pro | 2K | 0.02 | 0.50 |
| wan2.7-image-pro | 1K / 2K / 4K | 不单独计费 | 0.50 |
| z-image-turbo | 关闭提示词改写 | 不支持参考图 | 0.10 |
| z-image-turbo | 开启提示词改写 | 不支持参考图 | 0.20 |

来源：[Qwen Pro](https://help.aliyun.com/zh/model-studio/qwen-image-3-0-pro)、[Wan Pro](https://help.aliyun.com/zh/model-studio/wan2-7-image-pro)、[Z-Image](https://help.aliyun.com/zh/model-studio/z-image-turbo)、[计费说明](https://help.aliyun.com/zh/model-studio/model-pricing)。4K 并不适用于 Wan 的所有任务；以[官方参数约束](https://help.aliyun.com/zh/model-studio/wan-image-generation-and-editing-api-reference)为准。

## 请求与结算

Qwen 输入/输出按张模板首期限定 **阿里云渠道的 Qwen Image 3.0**，不适用于 OpenAI 通用渠道、聊天端点或其他模型。JSON 的 `image` 支持 URL/Base64 字符串及字符串数组，也支持原生 `input.messages[].content[].image`，二者不要同时传入。图片编辑的 multipart 文件输入沿用原入口。

费用为 `输入图数 × 输入单价 + 成功输出图数 × 输出单价`，输入图费用不再次乘输出数量。1K/2K 按**输出像素面积**划分：不超过 2,250,000 像素为 1K，其余为 2K；不是按参考图自身尺寸判定。

数量与尺寸从转换、模型映射及渠道参数覆盖后的实际出站请求计算。未指定尺寸或 `auto` 时，先按两个档位中较大的总费用预留，成功后用上游图片计量字段和有效图片结果结算并退回差额。计量缺失、计量与图片结果矛盾、没有图片结果均不能报成功；上游超时、鉴权或参数失败也不代表模型已验收。

价格表单复用现有表达式配置；服务端私有 `_image_billing` 视图不能通过请求伪造。自定义表达式可继续使用原编辑器。模型广场按图片展示四项价格及分组倍率，不把配置预览的零值展示为免费 Token 价格。

[Qwen 协议与计量定义](https://help.aliyun.com/zh/model-studio/qwen-image-generation-and-editing-api-reference)。这些功能发布后仍需部署才能在线使用；本次不修改任何线上售价，也不代表已经完成真实阿里云付费生图验收。
