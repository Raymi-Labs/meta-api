# Agent Note: 模型详情当前位置导航

Status: implemented

## Problem

模型详情目录只有点击滚动，没有当前位置反馈。用户要求滚动时将当前板块对应的左侧栏目加粗，方便识别阅读位置。

## Decision

复用现有 sections 与滚动按钮，在局部导航组件中按页面实际位置设置唯一 aria-current="location"，以加粗和主题正文色呈现。当前板块取固定顶栏下方阅读线之前的最后一个有效板块；在页面底部选最后一个可见板块，让较短的 FAQ 也能高亮。兼容窗口和应用内部滚动容器。被动滚动事件经 requestAnimationFrame 合并，每帧最多测量现有少量板块；ResizeObserver 跟进 FAQ、分组和图表的高度变化。模型切换、加载/错误态及卸载清理旧状态和监听。

导航保持现有按钮与返回目录历史状态，不新增滚动时的 hash/history 写入。窄屏沿用横向目录，同样标记当前位置，不引入另一套导航。

## Existing notes audit

- [模型独立详情](../../implemented/feature/2026-10-25-model-detail-page.md)：扩展当前位置反馈，保留滚动按钮及目录返回状态边界。
- [接入信息合并](../../implemented/simplification/2026-10-30-model-detail-quickstart-only.md)：继续兼容 #quick-start/#endpoints 跳转，不恢复重复端点栏目。
- [模型属性](../../implemented/feature/2026-10-30-model-attributes-official-sources.md)：相邻布局，现有栏目顺序及属性数据不变。

## Alternatives considered

- 只在点击时加粗实现最少，但不能响应滚轮、触控、键盘滚动和历史锚点，无法满足用户要求。
- IntersectionObserver 可以避免逐帧测量，但仍需补充阅读线、短末尾板块和动态尺寸处理；当前仅几个板块，使用有界的帧合并测量更直接。

## Verification

- model-official-pricing.browser.mjs 在桌面亮色、390px 暗色与 320px 布局下通过，覆盖上下滚动、点击/键盘导航、末尾 FAQ、展开内容、分组过滤、SPA 切换模型以及唯一当前位置和字重；URL 与 history 长度不因目录滚动改变。
- 本地预览手动从模型属性向下滚动，确认分组价格自动加粗；截图 model-active-section.png。
- 导航状态白名单及路由权限回归、ESLint/Prettier、完整前端构建和公开页面预渲染通过。事件与 ResizeObserver 卸载清理经代码核对；未执行跨浏览器或真实生产验收。

## Consequences

每个活动滚动帧会测量现有少量板块，以换取动态布局下的一致位置反馈。位置计算使用现有 scroll-margin-top，避免固定头部遮挡导致错位。ResizeObserver 与滚动共享一次帧任务，状态不变不重复渲染，不增加后端请求或修改权限与计费。

相关扩展：[固定模型栏与跨模型对比](2026-10-31-model-detail-sticky-compare.md)复用本能力，并通过权限校验后的模型体验深链接连接详情、对比和测试。
