# Agent Note: 富文本与 Mermaid 统一 DOMPurify 修复版本

Status: implemented

## Problem

主站直接锁定 DOMPurify 3.4.11，Mermaid 另带3.2.6，图片工作台另有独立锁文件，Streamdown→Mermaid 带3.4.4。单改主站直接依赖不能覆盖这两条间接渲染链。上游77436271f升级到3.4.17。GHSA-cg2g-xh64-v4q7影响3.4.4至3.4.16，要求 SAFE_FOR_TEMPLATES、DOM返回/原地模式、带shadow root的DOM输入和后续模板执行同时满足。本地 richContent 只使用字符串清洗与 WHOLE_DOCUMENT，未发现这些组合；这是预防性依赖修复，不能称为已确认的线上XSS。

## Decision

直接依赖和 overrides 同时固定3.4.17，由 Bun 更新锁文件，使主站 Mermaid 也使用该版本；图片工作台独立 overrides 同步固定3.4.17，保留原有其他 override。复用 richContent/DocumentRenderer 和既有严格 Mermaid 清洗，不放宽配置、不改文案和组件。用真实 Chromium 验证安全回归与正常渲染，公开文档和主站都构建。

## Existing decisions

检索活跃笔记没有 DOMPurify 版本治理归属；文档覆盖/独立文档站笔记部分重叠，只复用渲染器，不修改文档内容和语言语义。Schema JSON 清洗是后端协议转换，与浏览器 HTML 清洗无关。无待否决提案。

## Alternatives considered

- 只升级直接依赖改动最小，但 Mermaid 仍使用另一份旧清洗库，不能覆盖完整依赖树。
- 整体升级 Mermaid/VChart 能获得更多修复，但涉及图形API和主题兼容，超出本轮清洗依赖修复；定向 override 并验证已有图表更可控。

## Testing

确认实际直接/间接解析均为3.4.17；真实浏览器重现旧版shadow root分片模板表达式保留，升级后移除；字符串富文本清除脚本/事件/危险链接，保留安全链接、Markdown表格、文档样式和Mermaid图。主站、文档构建成功。

## Consequences

新版清洗规则可能移除过去显示的危险或边缘HTML。保留现有样式能力不等于增加CSS隔离保证；不证明所有XSS形态不存在。不进行线上攻击测试或生产升级。

## Validation evidence

The direct and Mermaid-resolved main-app sanitizer and the independent playground sanitizer resolve to 3.4.17. Bun left an obsolete nested 3.2.6 package in the existing node_modules; it was removed after verifying it no longer occurs in the lockfile. A frozen-lockfile install passes, and the browser regression now rejects stale nested resolutions.

The old sanitizer reproduces the split shadow-root template expression; 3.4.17 removes it. Chromium checks rich HTML, Markdown tables, whole-document styles, both Mermaid dependency trees, and HTML sanitization after diagram rendering. These are local synthetic tests, not production exploitation or a complete image-generation flow.

Main web including the playground TypeScript/Vite build, standalone docs, static-asset checks and 13 release-policy tests pass. Existing large-chunk and Browserslist notices remain. Default local prerender skips missing /usr/bin/chromium; the separate regression actually runs installed Chrome. Unrelated regenerated playground snapshots are excluded; release builds regenerate assets from the committed locks.

The benefit is consistent patched dependency resolution; the cost is maintaining two overrides and browser coverage. No production deployment occurs.
