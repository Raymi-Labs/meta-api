# Agent Note: 模型广场筛选抽屉与标签选项

Status: implemented

## Problem

现有筛选全屏弹窗和等宽网格使用户难以同时感知页面位置、完整分组名称与倍率。用户要求右侧抽屉、紧凑标签和可折叠分区。

## Decision

复用 Semi SideSheet 和现有五类筛选的数据与回调，共用选项组件增加显式 pills 变体，默认渲染不变。抽屉依次展示分组、厂商、标签、计费和端点，显示设置置后；头部与底部固定，中部滚动。手机占满宽度。即时生效，关闭不撤销，重置沿用统一状态重置；说明明确这一行为。

## Alternatives considered

- 全新生成筛选项组件可完全控制界面，但复制计数/权限/选组规则易分歧，采用现有数据与显式渲染变体。
- 仅 CSS 修改旧弹窗成本低，但不能可靠提供右侧位置、固定区域及折叠可访问性，复用已有 SideSheet。

## Verification

现有 pricing-filter-items 和 pricing-vendor-selection 回归通过，原组件默认模式不变。pricing-filter-browser 和 pricing-wallet-layout.browser 使用合成 API 验证桌面/手机暗色、零倍率/无权限组、厂商切换及价格同步、折叠 Enter 键、Esc 关闭重开、重置、健康与钱包流程。新增长名称换行及抽屉不横向溢出断言。主站 Vite 构建及修改源码 ESLint 检查通过；不调用真实模型或支付。

## Consequences

即时筛选没有取消语义，明确说明并以关闭命名完成按钮；多组标签仍需滚动，固定关闭/重置避免迷失。只改变前端入口，不改变后台权限或计费。

## Related notes

[筛选可见性](../../implemented/feature/2026-10-11-pricing-filter-visibility.md)、[厂商切换](../../implemented/bug-fix/2026-10-21-pricing-vendor-transition.md)继续管理选项与状态规则，本文仅扩展呈现；[布局](../../implemented/feature/2026-10-21-pricing-wallet-layout.md)其余页面保持不变。检索命中的支付/令牌笔记与本次无关。
