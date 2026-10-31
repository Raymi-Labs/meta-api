# Agent Note: Kimi 官方价格和介绍译文补齐

Status: implemented

## Problem

线上 v1.4.3 的 Kimi K3.1 Pro official_pricing 为空，description_translations 缺失；介绍仍为历史模型库英文。服务器翻译任务2026-10-31 20:01收到429，持久退避至20:31。既有翻译能力并不等于全部内容已生成，随版本校对词条此前仅包含GPT-6 Astra。

Kimi /docs/models.md 有精确模型介绍，链接的 /docs/pricing/chat.md 只输出空表格；/docs/pricing/chat HTML 中价格位于 DocTable 的静态 MDX 数据，不是可见服务端表格。现有流水线未请求和解析该价格源。

## Decision

沿用固定官方来源与现有自动所有权，增加 Kimi HTML 价格读取：只解码 Next flight JSON 字符串和明确的 DocTable 字面量，不执行脚本，不追随页面提供的任意URL。精确模型ID、列顺序、USD / MTok、缓存5m/1h和无税说明校验后生成独立档位；未知布局/重复模型/单位错误拒绝并保留旧价，加入来源异常计数。不改本站计费。

复用内容哈希词条，为线上Kimi现有原文、官方Kimi新原文和此前反馈的Terra原文补齐17语言。保留人工优先、源文变化失效、固定免费服务与持久退避；不重启生产任务或重置冷却。补齐已知原文不宣称全目录已经翻译。

## Existing notes audit

- [模型介绍全站语言](2026-10-31-model-description-locales.md)：部分重叠，延用缓存/人工覆盖/免费退避；增加已核对原文的随版本译文及验收。
- [官方资料默认值](../feature/2026-10-31-model-official-defaults.md)/[价格同步](../feature/2026-10-31-official-pricing-sync.md)：部分重叠，扩展Kimi来源解析，保留人工保护及失败保留。
- 官方价格/分组折扣：沿用原有独立币种和档位，不改计算或UI结构。

## Alternatives considered

- 手填当前Kimi价格和中文可见效，但缺乏后续官方更新及其余语言覆盖；扩展既有同步链路。
- 仅等待免费翻译恢复符合既定服务约束，但已知短原文可以沿用随版本校对词条，减少当前受影响页面等待。
- 执行官方网页脚本或新增浏览器后台可渲染动态表，但扩大服务器运行依赖和外部代码执行面；只提取经过结构验证的静态字面量。

## Testing

- 2026-10-31实际官方HTML：Pro为USD / MTok 输入2、输出10、缓存读取0.2、5m写入2/1h写入4；Highspeed及K2精确ID独立验证，不混入Batch或近似ID。
- Go专项验证错页、单位/币种变化、列顺序、TTL变化、重复模型、动态表达式、残缺行和空Markdown保守失败；SQLite完整同步到公开pricing返回价格和17语言，503后旧价保留。复用既有人工保护回归，无新增schema。
- Chrome真实组件验收Kimi历史/官方原文和Terra原文，各17语言卡片/详情切换、中文搜索、390px无横向溢出、人工编辑/恢复自动；Kimi两种缓存档位与来源链接可见，外部请求0。验收API为隔离夹具，官方价格夹具由本轮实际HTML解析结果产生。
- Go全量、介绍及币种前端专项、主站/文档/Go构建、决策笔记及发布策略检查通过；Astra既有17语言浏览器流程回归通过。本机预渲染脚本因无/usr/bin/chromium按既有规则跳过，浏览器验收使用已安装Chrome。免费翻译服务未重试，生产未写入。

## Consequences

官方页面结构可能变化，解析必须保守失败。免费服务仍可能限流，其他尚无译文的模型继续原文回退；版本发布不代表生产已升级或全部译文完成。
