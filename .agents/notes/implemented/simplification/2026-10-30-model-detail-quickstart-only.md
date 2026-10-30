# Agent Note: 模型详情合并接入信息

Status: implemented

## Problem

模型详情同时展示快速开始和 API 端点，协议、路径与说明重复。用户要求移除下方 API 端点；模型广场的 Responses 示例与历史链接仍指向 #endpoints，需要一并迁移。

## Decision

详情只保留快速开始，删除独立 API 端点区及导航。广场 Responses 示例改到 #quick-start，并通过导航状态选择 Responses；普通进入仍默认已声明协议。旧 #endpoints 链接滚动到快速开始，避免失效。复用既有示例资格、未声明提示和七语言代码生成，不修改目录元数据、筛选、权限、后端接口或旧侧栏组件。

## Existing notes audit

- [协议分层展示](../../implemented/feature/2026-10-28-model-protocol-display.md)：部分取代详情端点区及链接目标，保留声明标签、筛选计数与文本模板资格。
- [协议选择](../../implemented/feature/2026-10-26-model-protocol-examples.md)：保留默认已声明、手动选择与复制；仅显式 Responses 示例入口预选该模板。
- [模型 FAQ](../../implemented/feature/2026-10-30-model-faq.md)：相邻区块，仍在详情底部，数据与编辑流程不变。

## Alternatives considered

- 仅删除端点 JSX 改动最少，但会让广场和历史锚点落空；同步迁移入口并兼容旧锚点。
- 保留独立端点区可继续集中列出声明路径，但与已有快速开始重复且不符合用户要求；使用快速开始统一承载接入示例。

## Verification

23 项协议、快速开始与文档回归通过。既有浏览器流程更新后在 1440px 桌面和 390px 手机暗色通过：无重复区块及导航，Responses 入口预选、语言切换、复制和已声明筛选保持正确，旧锚点滚动到快速开始并默认已声明协议，无页面溢出或写请求。旧锚点检查以标题在视口上半部可见为准，兼容短页面到达滚动底部的情况。验收使用独立 Vite 服务并显式验证主题，避免演示服务固定浅色覆盖测试设置。完整前端构建通过，指定本机 Chrome 后五条公开路由预渲染通过。本地预览确认快速开始直接衔接 FAQ。

## Consequences

示例入口预选不代表新增协议支持；未声明提示继续保留。仅调整展示，不进行真实模型调用或生产部署。
