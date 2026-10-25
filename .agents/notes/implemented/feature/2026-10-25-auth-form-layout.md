# Agent Note: 认证同屏表单与独立协议确认

Status: implemented

## Problem

登录与注册先显示 OAuth 选项，账号表单还需进入下一层；合并的协议句子在外语下换行，难以区分确认项。用户要求紧凑同屏表单、两项独立确认，并隐藏这两个页面的底部页脚。

## Decision

复用现有认证接口和配置驱动的 OAuth 提供商，使用共享 AuthCard 排版和 AuthConsent 状态。每个已启用协议独立勾选，未启用的协议不阻塞；密码、OAuth、微信、Telegram 和 Passkey 在发起认证前检查完整同意状态。保持注册 username/password/password2 与条件邮箱验证契约，保留邀请码、Turnstile、2FA、next 安全回跳。页脚只按 login/register 路由隐藏，其他归属信息及源文件不删除。

## Existing decisions

检索结果：[邀请品牌角色](2026-10-22-invite-brand-character.md)部分重叠，保留邀请专用展示并适配新卡片；[语言同步](2026-10-25-site-language.md)继续适用，复用已有 17 语言词条；[企业导航](2026-10-25-enterprise-navigation.md)继续适用，认证页使用现有共享顶栏。其他主题、计费、SSH 和管理会话笔记与认证表单无关。

## Login transition

React Router 的 startTransition 会出现 window.location 已更新、旧登录树仍重渲染的窗口。认证重定向与表单从 useLocation 的同一渲染快照解析 next 和邀请码，防止成功认证后被旧树按空 search 回退到 /console；安全路径与后端 SSO 规则不变。

## Alternatives considered

- 仅默认打开邮箱表单：改动小，但仍把 OAuth 藏到另一层，不能满足同时可见的布局目标。
- 照参考图仅保留邮箱和密码：最简洁，但现有注册依赖用户名、密码确认和可选邮箱验证，会改变后端契约；本轮只统一呈现，不删必要字段。
- 仅靠按钮 disabled 检查同意：直观，但 OAuth/键盘提交也需门禁，采用共用判定并在 handler 再校验。

## Verification

- 独立同意纯函数覆盖启用/勾选组合与撤回；浏览器验证未勾、单勾、双勾、单项和无政策配置，未同意的 OAuth 入口给出既有本地化提示。
- Chrome 验证桌面、320px 英文、暗色和邀请注册；OAuth 与输入同屏，窄屏无横向溢出，邀请深蓝品牌布局保留。
- 隔离本地接口验证登录/注册失败、邀请码携带、注册后返回登录与条件邮箱验证；2FA 模拟完成后确实返回 /enterprise；路由组件回归覆盖全局地址先变而旧路由仍渲染；Turnstile 生命周期保留，不发起真实注册或外部 OAuth。
- 验证同意、回跳、邀请、导航、全站语言资源测试及格式/静态检查、完整前端构建和五页预渲染；发布与生产部署分开。

## Consequences

注册现有 OAuth handler 没有同意校验，补齐会使用户必须先在卡片下方确认再点击第三方按钮；使用原有本地化提示说明。窄屏或长语言允许每项内部自然换行并保持复选框顶部对齐，不强制不换行造成溢出。
