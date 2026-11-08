# Agent Note: 移除 Meta API 的 Pulse 展示板块

Status: implemented

## Problem

消费贡献退役后，Meta API 的券余额页与结算运营页重复社区展示，用户要求删除两个板块并保留账号和奖励对接。

## Existing capabilities and impact

核对 App、SiderBar、菜单可见性设置、router/api-router.go、Pulse BFF 与配置消费者：三个只读代理仅供这两个页面调用，内网地址和两组 BFF 密钥没有其他运行消费者。既有[退役记录](2026-11-08-retire-pulse-consumption-api.md)部分重叠，本次替代其保留展示页的决定；钱包与私网迁移笔记边界不变。

## Decision

删除页面、菜单、默认配置和三个只读 BFF。生成密钥迁入超级管理员设置接口。旧 BFF 配置在读取时忽略，更新时清理废弃键但保留奖励/SSO密钥；新提交废弃字段拒绝。保留 Internal Benefit、论坛登录绑定与幂等恢复。无数据迁移或生产权限变更。

## Alternatives considered

- 仅隐藏菜单：可保留旧书签，但页面/API/密钥仍维护无用路径，因此删除。
- 关闭整套 Pulse：可减少配置，但会破坏社区登录和奖励到账，因此保留必需接口。

## Result

旧菜单配置不能恢复入口；BFF 不再注册，生成密钥只在 RootAuth 后调用；旧配置不阻断奖励密钥加载和保存；奖励与SSO测试、前端构建和笔记校验通过。

## Consequences

外部自建客户端若调用被移除的 BFF 需迁到社区；发布说明列出不兼容范围。生产部署和日志库授权回收独立处理。

## Verification

`go test ./common ./controller ./model ./middleware ./router`、`go build ./...` 通过，覆盖配置兼容、奖励与 SSO 回归以及生成密钥权限。前端 8 项配置与菜单测试、登录跳转测试、相关文件 ESLint、`bun run build` 通过。本地缺少 `/usr/bin/chromium`，预渲染按脚本跳过，由发布工作流执行。未配置外部数据库 DSN 的集成测试按既有规则跳过；本次无数据库 schema 或 SQL 变更。生产部署、真实社区端到端验收和日志库权限回收尚未执行。
