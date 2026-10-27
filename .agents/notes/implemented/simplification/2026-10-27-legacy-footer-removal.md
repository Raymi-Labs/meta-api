# Agent Note: 统一页面布局停用旧页脚

Status: implemented

## Problem

控制台、登录注册和模型广场已隐藏旧页脚，关于、文档及其他使用 PageLayout 的页面仍显示整块页脚。用户核对展示位置后明确要求把这一套页脚全部去掉，需要统一展示范围并清除其占位。

## Decision

移除 PageLayout 中 FooterBar 的唯一挂载点、Layout.Footer 容器及仅用于该页脚的路由判断。当前请求明确针对旧 Footer.jsx 的整块展示；公开首页、下载、企业及政策页使用独立 PublicFooter，独立额度页也有自己的布局，均不扩展本次范围。保留 Footer.jsx 源文件、后台 Footer 配置和其他项目归属信息；不更改接口、权限、数据库或生产配置。

## Existing decisions

- [认证同屏表单](../../implemented/feature/2026-10-25-auth-form-layout.md)部分重叠：仅扩展旧页脚的停用范围，认证与协议规则继续适用。
- [首页卡片与页脚](../../implemented/feature/2026-10-24-home-capability-motion.md)、[公共政策](../../implemented/feature/2026-10-25-public-policies.md)、[企业导航](../../implemented/feature/2026-10-25-enterprise-navigation.md)属于独立公开壳，本次不改变其页脚或导航。
- 活跃笔记中没有独立的控制台页脚归属篇；既有范围由功能地图记录，本次同步更新。

## Alternatives considered

- 继续扩充隐藏路由清单：可以逐页选择，但用户要求这套页脚全部停用，清单容易漏掉新页面或尾斜杠路径。
- 在 Footer.jsx 返回空内容或使用 CSS 隐藏：改动集中，但留下无效挂载及可能的容器占位；直接移除唯一挂载点更完整。

## Verification

- Chrome 对生产构建执行 34 组页面检查，覆盖 1440px 与 390px 下的关于、文档、密码重置、403/404、登录注册、模型广场、额度查询、首页、下载和企业页；没有旧页脚容器、署名行或未捕获异常。关于页截图检查正文、导航及无横向溢出。
- 自定义 Footer 与演示站模式同时启用后，关于页（含尾斜杠）、文档和密码重置仍不显示旧页脚。首页、下载、企业的 PublicFooter 和额度页独立页脚保持渲染。浏览器使用隔离模拟接口，没有生产写入。
- 16 项导航/公开页脚/企业示例测试、认证回跳脚本与 5 项路由回归通过；ESLint、Prettier、完整前端构建和五页预渲染、独立文档构建、发布策略及 notes 检查通过。

## Consequences

后台 Footer 配置不再用于 PageLayout 展示，已有配置保留但在该布局不生效。若将来需要恢复站点联系或政策入口，应明确选择展示位置，不能直接恢复整套旧页脚。版本发布不代表生产升级。
