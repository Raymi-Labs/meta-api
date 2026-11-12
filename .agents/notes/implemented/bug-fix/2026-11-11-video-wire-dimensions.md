# Agent Note: 视频尺寸字段一致性与Muse能力门禁修正

Status: implemented

## Problem

Sora兼容JSON缺少size时无条件补720x1280，覆盖显式resolution/aspect_ratio语义；Muse仍以旧供应商实测范围拦截请求。客户端参数放行不能保证真实出站字段与估价快照一致。

## Decision

共用尺寸规范化：像素size、比例与可解析Np分辨率一致性检查；明确比例及Np按短边等比计算整数像素，不舍入、不推断未知分辨率；仅无任何尺寸参数时保留历史默认。Muse只校验格式、字段冲突和网关资源预算，不按旧时长/720p/4图/4000字能力范围拒绝。使用已存在的同一BuildRequestBody快照计费和发送。

## Alternatives considered

- 只在客户端补size不能修复直接API与旧客户端。
- 遇未知规格回退720p会无声改变用户意图，不采用。
- 去掉所有检查会让矛盾字段及无界内嵌参考图进入请求，保留格式和资源边界。

## Testing

完整适配器Validate/Build路径覆盖Muse和Omni的181秒/1080p/21:9，像素结果2520x1080；未知规格不注入默认，冲突在本地报错。计费表达式读取相同秒数/分辨率/size。GET路由继续隐藏上游地址。无付费生成与生产配置变化。

## Consequences

实际供应商仍可拒绝参数；已有价格表达式可能仅覆盖部分规格，未配置报价应保留拒绝以免漏计，不自动改线上价格。网关保留单图20MiB、合计64MiB、16图及正整数范围的资源/格式检查。

## Verification

新适配器回归在旧实现失败，修复后sora适配器、relay、router完整测试通过。加入尺寸冲突/未知规格、Muse参考图与正整数范围、实际出站与计费快照一致性回归。修正两个旧路由测试的上游URL预期，公开响应仅允许元衡content地址。不改生产价格与分组，不进行收费生成。

## Related notes

替代[Muse长片](../../implemented/feature/2026-10-31-muse-long-reference-video.md)的旧能力门禁；保留其参考图格式、单次提交、计费与任务恢复。客户端同步修复对应yuanheng-switch仓库，不以单端通过代替线上部署。
