# Agent Note: Gemini 官方价格文档迁移

Status: implemented

## Problem

Carbon 等型号缺官方参考价。旧 ai.google.dev 地址跨域跳转至 aistudio.google.com，抓取器按安全规则拒绝；新版文档使用 Markdoc，不再含旧 HTML 价格结构。

## Decision

价格抓取直接使用固定新官方地址，厂家白名单加入该精确域名。将 Markdoc 的精确模型链接、Standard 标签和三列价格表转换为既有解析器的结构，复用日期选价及条件档位。不放宽重定向策略，不改销售倍率。隐式缓存说明保留在备注，数值解析单独处理。

## Existing notes audit

[覆盖审计](../../implemented/bug-fix/2026-11-01-official-pricing-coverage.md)与[元数据覆盖](../../implemented/bug-fix/2026-11-13-model-metadata-coverage.md)部分重叠；延续厂家证据和未知别名拒绝，补齐其中 Google 迁移缺口。

## Alternatives considered

手填 Carbon 可立刻补一个型号，但不能修同步且促销会过期。放开任意跨域重定向不能解决格式变化且扩大访问边界，因此采用固定地址与格式适配。

## Verification

真实 Carbon Standard 表解析输入、输出、缓存价；验证促销切换、非 Standard 隔离、相邻型号不串价、未知别名拒绝、厂家公开校验。实际下载完整价格文档及离线 HTTP 抓取回归通过；旧 HTML 解析继续兼容。`go test ./controller ./model`、针对真实文档及 HTTP 抓取的回归、`go build ./controller ./model`、`make verify-notes` 和发布策略检查通过。本地根程序构建因 web/dist 中 iCloud dataless 文件阻塞而中断，完整应用构建交由 Release CI 验证。

## Consequences

官网再次改变格式时仍保留未知或旧有效值；发布不等于生产升级或数据同步完成。

## Coverage audit

[11月15日全目录复查](../../../../docs/audits/2026-11-15-official-price-coverage.md)确认线上210项中40项缺价；本实现可解析 Carbon 与 Hydrogen，但不覆盖 Omni 的文本/视频分项价格与通用详情路径。线上仍为 v1.32.1，不能将发布视为落库生效。
