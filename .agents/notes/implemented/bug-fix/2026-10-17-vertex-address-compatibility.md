# Agent Note: Vertex 多区域与可选项目地址

Status: implemented

## Problem

旧适配器统一拼接 region-aiplatform 域名，不适用于 us/eu；API Key 缺少可选项目路径，无法表达需要 project 的官方端点。

## Decision

适配 718a4e7f9/a0bf6a8a4：共用官方地址生成，区分 global、us/eu、普通区域；服务账号沿用凭据项目，API Key 使用 settings.vertex_project_id，留空保留 Express 路径。旧 Semi 渠道编辑器增加可选项目，沿用管理员接口和 settings JSON，无数据库迁移。

## Alternatives considered

- 所有 API Key 强制项目可简化地址，但破坏现有 Express 使用者。
- 只靠自定义 URL 规避 404 不需要 UI，但不能清晰保存项目/区域语义；采用可选设置，不引入上游新前端。

## Testing

Vertex/dto 回归、Vertex 竞态及构建通过，覆盖两种凭据、google/anthropic、global/us/eu/普通区域、流式查询参数、API Key 特殊字符及空项目兼容。旧 UI ESLint、Bun 全构建和五个公开页预渲染通过。浏览器在真实项目壳和模拟管理员 API 中验证编辑回填、保存/清空、未知 settings 保留、保存失败保留输入、JSON 切换以及 390px 暗色布局；7 种语言补齐新字段。浏览器既有 React/Semi 开发警告未扩展修复；不将模拟保存等同真实供应商调用。

## Consequences

项目权限与 API Key 资格仍由供应商校验，不能保证配置即调用成功。不改服务账号获取令牌、模型映射或计费；不支持的新模式仍拒绝。活跃笔记无 Vertex 决策归属，保留 OpenCode/超时等相邻出站规则。
