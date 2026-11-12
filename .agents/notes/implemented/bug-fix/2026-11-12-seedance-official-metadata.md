# Agent Note: Seedance 2.5 官方资料补齐

Status: implemented

## Problem

线上 doubao-seedance-2-5-260628 官方参考价与模态为空；本站按秒价格非零。官方同步没有该型号入口。

## Decision

复用官方文档抓取、来源和人工覆盖保护，精确读取官方能力表型号列及价格表人民币四档，不改变本站计费。未公开的 token 限制和文本工具能力保持未知。

## Existing notes audit

official-pricing-coverage、manufacturer-official-pricing 与本篇部分重叠，继续负责通用同步与来源保护；video-price-display 负责本站按秒价格，public-model-provenance 负责公开资料边界，均不被替换。

## Alternatives considered

手写生产数据库资料缺少后续同步；以本站秒价代替官方价混淆单位和输入条件，因此扩展既有官方同步。

## Verification

真实官网 HTML 和缩减夹具均通过解析；错型号、错单位、缺档拒绝。隔离 SQLite 的完整同步→保存→公开投影验证四档价格与模态，503 保留旧值、人工模态编辑不覆盖。相关官方价格、来源与自动同步回归通过；controller 构建通过。源站压缩下载官方页面通过。

本地完整程序构建读取既有 web/dist/assets 中云盘重复文件时阻塞，停止该构建；完整服务端与前端构建由既有 Release 工作流验证。

## Consequences

官网变更拒绝解析并保留原快照；官方能力不等于每个渠道的支持承诺。

型号覆盖及共享抓取扩展见[全目录补齐](2026-11-13-model-metadata-coverage.md)，保持精确型号与厂家来源原则。
