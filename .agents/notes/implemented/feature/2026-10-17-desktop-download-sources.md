# Agent Note: 官网页面提供安装包直链及备用线路

Status: implemented

## Problem

官网主下载按钮沿用平台镜像地址，经生产兼容跳转最终走 R2，不能利用新 meta-api.vip 入口；页面没有可供用户切换的独立备用下载。

## Decision

元衡客户端仓库迁入 `Raymi-Labs/yuanheng-switch`，GitHub 备用链接使用公司仓库；原个人仓库由 GitHub 转移跳转兼容，主下载域与安装包名称不变。

复用 downloads.json 的 ServerAddress 派生 URL、当前版本及实际包目录。仅对精确匹配 meta-api.vip 官方稳定版 YuanHeng.Desktop 安装包的地址增加 origin=1，使主按钮使用已存在的公开只读直链，同时提供同版同文件的 R2 与 GitHub 链接。其他域名、非标准包、预发布版、任意查询/片段保持后端地址且不推导备用源。

卡片容器由链接改为 article，主下载与备用下载为独立可聚焦链接，避免嵌套链接。沿用现有语言和公开页面入口，不增加后台字段或改 ServerAddress。

## Existing capabilities and scope

检索 active notes 中 desktop/download/R2/客户端下载，没有本下载页面的现行归属笔记；API 线路探测和反代迁移为相邻独立能力，本次不改。涉及 DesktopDownload 页面、规范化 helper、publicLocale 及下载回归。桌面仓库独立发布自动三源下载器，官网仅提供人工选择。

## Alternatives considered

- 新增后端下载域设置与返回契约：可支持更多部署，但当前仅元衡官方发布需要，已有可信固定路径足够，避免扩大配置接口。
- 将所有链接改到硬编码主域：容易实现，却覆盖了后台 ServerAddress 与第三方部署地址，不采用。
- 浏览器先并行测速再下载：可以自动选择，但涉及跨域 Range/CORS、额外流量和浏览器下载限制；官网采用明确直链和手动备用，自动择优由客户端负责。

## Verification

下载 helper 回归覆盖三类安装包的精确版本/文件映射，未知参数、预发布、HTTP/伪域名/自定义域名/端口、错版本和非标准文件保持原址。真实 Chromium 项目壳使用合成目录，分别点击直链、R2、GitHub 并验证实际附件内容、页面不跳走、键盘 Tab 顺序；1440px 中文与 320px 英文/深色系统环境、刷新、503 错误重试、自定义域名无备用源全部通过。截图目视确认布局无水平溢出；公开下载页保留既有浅色主题。

ESLint、Prettier、Bun 完整前端构建及五个公开页 Chromium 预渲染通过；Go service/controller 下载相关回归通过。生产页面以应用部署后的验收为准；浏览器 fixture 不等于已上线。


## Consequences

只有官方固定稳定版路径可以生成备用链接；后端更换 ServerAddress 时回到单链接而非猜测新入口。浏览器原生下载不会自动切源。线上页面需要应用部署，发布版本和生产升级分别验证。

## Release and production boundary

代码提交 d7e585e50，main 已推送。[compat.44 发布](https://github.com/nanashiwang/new-api/releases/tag/v1.0.0-compat.44) 沿用兼容版 prerelease 标记，四个平台二进制齐全且均有 GitHub SHA-256；构建 run 39016645990 全成功。Docker run 39016631660 成功，生产架构镜像 `ghcr.io/nanashiwang/new-api:sha-d7e585e` 的 manifest 为 `sha256:063e407a0ac20399246ea1e4dbdfd29de23af2afc997e4883946256a1dbc80ee`，源站只读查询可见。

用户确认“仅发布，暂不升级官网”。生产仍为 `main-135966d` / `sha-135966d`，运行、Compose 与发布管理状态一致，未执行部署。新直链和备用按钮尚未上线；该决定不阻止独立桌面版本的已授权发布与安装包分发。

安卓公开测试版使用独立版本路径和固定元数据，见 [安卓下载入口](2026-10-19-android-download.md)，不进入本篇的桌面目录契约。
