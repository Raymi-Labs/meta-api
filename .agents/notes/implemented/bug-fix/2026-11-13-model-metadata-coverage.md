# Agent Note: 扩展模型官方资料覆盖

Status: implemented

## Problem

240 个线上模型中 40 个缺官方价格、44 个缺模态。Seedance 仅接入一个型号；百炼独立详情页的能力矩阵未读取。已有全目录审计显示旧解析不能补齐缺价条目。

## Decision

扩展 Seedance 官方型号表的六个明确版本及分型号价格，按来源去重抓取。百炼复用独立官方模型详情页的精确标题和能力矩阵补齐模态、三态能力、token 限制，不从模型名推断能力，不从简单价格表丢失区域条件。复用自动同步、人工保护及公开来源投影。

## Existing notes audit

[Seedance](../../implemented/bug-fix/2026-11-12-seedance-official-metadata.md)部分扩展，精确身份原则保留；[覆盖审计](../../implemented/bug-fix/2026-11-01-official-pricing-coverage.md)继续负责价格条件和未知项。本篇不更改本站结算。

## Alternatives considered

逐个写固定属性容易过期，采用统一详情矩阵解析。按型号前缀套用模态会混淆语音识别、合成和多模态模型，不采用。代理别名不当作厂家正式 ID。

## Verification

六个 Seedance 真实表格解析及独立价格断言通过；共享来源只发两次请求。百炼两列/四列、中英文模态、未知能力、错标题回归通过，24 个真实详情页中 19 个成功。相关 controller/model 回归、race 检查和 controller 构建通过；结构未变更。全目录缺失项见[覆盖审计](../../../../docs/audits/2026-11-13-model-metadata-coverage.md)。

## Consequences

官网结构或重定向变化仍会拒绝解析；没有证据的项目保持未知，发布不等于线上全部补齐。
