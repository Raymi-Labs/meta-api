# Agent Note: OpenRouter 账户累计金额展示手续费

Status: implemented

## Problem

管理员希望按含 5% 手续费的美元成本看累计购入和消费，同时用官方余额判断还能调用多少额度。现有三张卡片都直接显示 credits 数据。

## Decision

在两张累计卡片格式化前乘以 1.05。余额取原始额度差，区间说明收进点击图标，保留固定人民币换算。

## Alternatives considered

- 修改后台 credits 会把运营展示的费率传给原始额度的消费者。
- 修改通用美元格式化会把手续费也计入余额，导致显示的可用额度失真。

## Existing notes audit

[账户财务](../../implemented/feature/2026-11-02-openrouter-account-finance.md)记录原始来源，[人民币对照](../../implemented/feature/2026-11-15-openrouter-cny-comparison.md)定义区间展示，本次只接续两个累计字段的显示。统一看板和快照笔记继续约束筛选、归属与缓存。

## Verification

金额回归覆盖 400、400.200081、零和未知值。真实 Chrome 的报表回归验证含费累计卡、原始余额、注释展开和收起，并在切换账户后检查新金额。日期筛选、人民币金额和 CSV 回归通过。

## Consequences

两张卡片显示按固定费率估算的成本。余额仍用于判断可用额度，因此不能用两个页面累计值相减来还原官方余额。核对逐笔实付可使用现有官方账单链接。
