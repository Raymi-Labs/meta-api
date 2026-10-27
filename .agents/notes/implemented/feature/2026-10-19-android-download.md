# Agent Note: 官网增加安卓公开测试版下载

Status: implemented

## Problem

元衡 Android 已有签名 APK，用户要求先在官网开放下载。现有 `/download` 只有桌面稳定版，不能把安卓 alpha 混入桌面更新目录或宣称已完成真机验收。

## Decision

元衡客户端仓库迁入 `Raymi-Labs/yuanheng-switch`，GitHub 备用链接使用公司仓库；原个人仓库由 GitHub 转移跳转兼容，主下载域与安装包名称不变。

扩展现有公开下载页，在桌面区域之前显示独立 Android 卡片，固定为已验证的 0.1.0-alpha.1，显示 Android 10+ / ARM64、版本、大小、测试状态、安装提示和校验值。主链接走现有下载域的移动版本路径，GitHub 为备用。安卓信息与桌面接口加载/失败独立。原桌面三平台动态目录及备用源规则保持；自动更新说明限定为桌面版。复用公开页面的中文/英文词条、响应式样式和导航。

现行 [桌面下载来源笔记](../../implemented/feature/2026-10-17-desktop-download-sources.md) 部分重叠，保留原精确地址边界；新增安卓版本独立由本篇负责。APK 分发由 yuanheng-switch 的移动分发工作流验证包摘要和签名后完成，官网使用已核验的固定版本路径。

## Alternatives considered

- 新建独立下载站容易隔离，但用户已有统一下载入口；复用能保留导航和品牌一致性。
- 把安卓放进桌面 downloads.json 会混淆版本及稳定状态，不改变桌面契约。
- 新增移动后台目录/动态配置适合多个持续版本，但首个已选定 alpha 只需固定经过验证的公开元数据；后续版本需明确更新此入口，不能自动追随未经选择的构建。

## Verification

- 前端 Prettier、ESLint、test:desktop-download、完整 build 和五个公开页预渲染通过。组件命名为 AndroidDownloadCard，避免与 androidDownload.js 在大小写不敏感文件系统上冲突。
- 真实浏览器验收中英文与 320/390px iframe 中的完整公共页面；安装提示、下载按钮及展开校验值布局正常。桌面目录使用现有预渲染 fixture；失败时安卓仍可用，重新加载后桌面目录恢复。
- 移动分发工作流 39192548027 成功，验证原 Release 摘要和签名后上传；公开下载完整 APK 的 16984529 字节及 SHA-256 与页面一致，HTTP Range 返回 206 / 1024 字节。
- 代码验收不代表生产上线；上线仍以版本构建、部署状态和公开页面实测为准。

## Consequences

首版只支持 Android 10+ ARM64，不提供 iOS 下载；真实登录、收费模型、充值、兑换和相册权限仍需真机验收。网页入口发布不代表应用商店审核或稳定版。
