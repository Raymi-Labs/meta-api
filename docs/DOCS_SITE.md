# 独立文档站

域名：`https://docs.meta-api.vip`。复用 `web/src/pages/Docs`、65 篇本站 Markdown 和十七种正文语言。参考站的 181 个主题在 `docs/handbook/reference-coverage.json` 中逐项映射；正文按本站路由与功能独立编写，不镜像其专有接口、活动或业务承诺。

## 内容维护

原有十三篇教程保持路由兼容；新增文章按指南、控制台、API 与集成四个 catalog 放在 `docs/handbook/`。目录元数据和 Markdown 都由正文语言采集器收集，桌面及手机顶部显示当前语言。搜索、目录、前后篇与正文使用同一语言词典。

修改 catalog 后运行 `bun run --cwd web sync:handbook` 生成显式原文导入；提交时由 `--check` 防止索引漂移。`test:docs-site` 检查目录唯一性、内链、代码围栏、文档主机误用和完整主题映射；`build:docs` 先检查十七语言内容覆盖与变量，再构建。新增正文须补全全部目标语言，不能以缺失译文的包发布。翻译由维护阶段生成，读者浏览时不向翻译服务发送内容。

内容分为实际改写、合并主题、尚不支持和仅作主题参考四种映射状态；参考站新闻只提示对应迁移主题，不成为本站发布证据。更新协议、支付或企业能力前，重新核对当前 UI、路由、鉴权与数据源。

## 构建与附件

在仓库根目录执行 `bun run --cwd web test:docs-site`、`bun run --cwd web test:site-language`、`bun run --cwd web test:runtime-docs`，然后 `bun run --cwd web build:docs`。产物为 `web/docs-dist/`，`build-info.json` 记录源版本和 Git revision。

应用现有 Linux Release 流程同时提供 `new-api-docs-<版本>.tar.gz` 与 `.sha256`，仍保留两架构应用附件与独立镜像流程。文档静态产物不会嵌入登录路由；主站原 `/docs/*` 继续可用。

## 独立部署

只使用已授权的站点发布窗口。静态文档与业务应用分别验收，不调用 Docker 更新或重启。部署之前保存当前站点软链、Nginx 配置及业务容器 ID/健康状态。

1. 下载同一 Release 的文档包和校验文件，执行 `sha256sum -c`，检查 `build-info.json`。解压到 `/var/www/newapi-docs/releases/<版本>/`，不要覆盖旧版本。
2. 确认 DNS 的 `docs` 无冲突后，新增指向文档服务器的记录。首次申请证书时仅开放独立 HTTP 虚拟主机的 `/.well-known/acme-challenge/`，目录为 `/var/www/newapi-docs/acme`；其他路径返回 503。沿用服务器既有 ACME 账户和续期流程申请该域名证书；安装 `deploy/docs-site/renew-hook.sh` 为 `/etc/letsencrypt/renewal-hooks/deploy/50-newapi-docs-reload`（0755），仅该证书续期后校验并重载 Nginx。
3. 安装 `deploy/docs-site/public-api.conf` 到 `/etc/nginx/snippets/newapi-docs-public-api.conf`，另安装 `docs.meta-api.vip.conf` 为独立站点。其公开接口固定 `127.0.0.1:3000`，部署前核实本机业务监听地址；不允许将整个应用代理到该域名。
4. 原子切换 `current` 软链，执行 `nginx -t` 后 graceful reload。失败时恢复原软链/配置，不停止正在服务的业务容器。
5. 验证 HTTPS 证书、主页、深层链接、静态资源、`build-info.json`；`/api/status` 和 `/api/pricing` 只能 GET/HEAD 且不传身份。其他 API、控制台和非只读方法应拒绝。核对业务容器 ID/健康状态未变。
6. 通过平台现有设置将 `DocsLink` 配置为 `https://docs.meta-api.vip/`；该设置只切换文档入口，不代表业务程序升级。新版主站导航、首页阅读按钮与页脚会携带 `lang`；文档消费合法语言参数并移除，随后以用户手动选择为准。

## 运行边界

公开定价目录仅生成公开模型样例，不保证某个账户/令牌的可用模型。请求失败保留教程，并提示重试；API 基地址回退 `https://meta-api.vip`，绝不使用文档域名调用模型。跨域仅传语言偏好，不共享账户、令牌或用户缓存。教程内主站操作链接回到主站，代码块中的请求样例保持原格式。

回退文档只需将 `current` 指回已校验的旧目录；必要时将 `DocsLink` 改回 `/docs`。业务应用发布继续走项目受保护入口，不能借此文档流程部署应用。
