# Agent Note: 官方参考价统一展示原价

Status: implemented

## Problem

官方参考价当前混入 Gemini 按日期生效的优惠、Inception/MiniMax 划线替换价、LongCat 限时折扣列、阿里云原价乘限时折扣。用户要求比较基准展示厂家原价，不展示促销价；详情与分组比较必须同源。

## Decision

复用官方抓取、自动所有权、持久化与公开 pricing 数据链路，不在前端硬编码模型价格。Gemini 仅在完整连续的双日期且后续价格更高时取后续常规价格；不按最高价猜测任意条件。显式划线原价、独立原价列和中文“原价”取来源原值。单位、地区、上下文和服务档位保持；本站销售倍率与采购限价独立。未知或不完整文档拒绝解析，保留人工维护保护。

## Existing notes audit

[Gemini 文档迁移](../../implemented/bug-fix/2026-11-15-gemini-pricing-migration.md)的当前日期选价被部分取代，保留其格式和来源边界；[价格覆盖](../../implemented/bug-fix/2026-11-01-official-pricing-coverage.md)的促销解析部分被取代，其他条件完整性继续适用；[缺口修复](../../implemented/bug-fix/2026-11-15-official-price-gaps.md)与本次部分重叠，保留身份核验与未知空态。

## Alternatives considered

只改 Carbon 数字无法覆盖 Argon 和其他厂家，且下次同步会覆盖。前端乘二无法区分真正原价、永久降价和不同服务档位，导致管理元数据与公开比较不一致。保留当前日期价不符合用户指定的原价比较口径。

## Verification

`go test ./controller ./model` 通过，包括下载的 Google、Inception、MiniMax、LongCat 真实文档，原价/缓存/零值、日期不连续/倒序/缺失/重复、上下文和服务等级、原自动优惠价更新、人工零值保护及公开 pricing 投影。15项前端官方比较和缓存语义回归、`go build ./controller ./model` 通过。阿里云使用原价及折扣样本回归；未声称覆盖所有在线厂家文档。无数据库结构或 SQL 方言修改。

## Consequences

当前公开快照明确受到影响的包括 Carbon、Argon、Mercury 3、LongCat；MiniMax 和阿里云解析路径一并修复。MiniMax 官方“Permanent 50% off”仍按用户要求取明确划线原价，保留 Standard/Priority 与上下文条件，不展示折扣标签。没有明确原价、只有折扣的数值不反推；历史自动值需重新同步才更新，人工值不强制覆盖。完整应用构建与发布产物由 Release CI 验证，发布不代表生产已升级。
