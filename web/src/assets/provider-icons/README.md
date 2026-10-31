# 补充厂商图标

这里补充当前 `@lobehub/icons@2.1.0` 未收录的品牌，不升级或覆盖已有组件。图标只用于识别对应厂商，商标与作者头像归各自权利人所有。

- `components/` 中除 Meituan 外的 11 个组件来自 npm `@lobehub/icons@6.13.0` 的 `es/<品牌>/components/Mono.mjs`。保留原路径数据，内联 TITLE，移除构建注释；MIT 许可见 [LICENSE.lobehub](LICENSE.lobehub)。上游：[lobehub/lobe-icons](https://github.com/lobehub/lobe-icons)。
- Meituan 复用现有 `react-icons/si` 的 Simple Icons 美团图标。
- `images/` 来源、原始下载地址和 SHA256 见 [assets.json](assets.json)，核对日期为 2026-10-31。Particle AI、Recursive、PrismML、XingChen、Nex AGI、Unbiased、TheDrummer 来自 OpenRouter 对应作者目录展示的标志；Elorian、Moondream、Reflection、ScaleDown、inclusionAI、Thinking Machines、Writer 使用该目录指向官网的 favicon 快照。Mancer 使用其官网声明的 favicon。
- Anthracite、Sao10K、Undi95、Gryphe 使用同名 Hugging Face 公开作者/组织资料中的头像，而非 Hugging Face 平台通用图标。CDN 返回 WebP 时保留原始字节并使用正确后缀。

运行时不请求来源网站，所有资源由 Vite 打包。单色矢量使用 `currentColor`，含多色/白色分隔线的图像保留颜色并带白色底板，确保深色主题可辨。数据库只保存白名单名称，不接受远程 URL、任意文件路径或 SVG 字符串。完整厂商配置对照见 [映射清单](../../../../docs/channel/openrouter-vendor-icons.json)。
