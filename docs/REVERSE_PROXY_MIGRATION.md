# 反代入口迁移与切换验收

当前主站新增入口（2026-10-29）：`meta-api.com` 使用 DNS-only A 记录直连 `155.103.118.155`，与 `meta-api.vip` 共用业务源站，独立 HTTPS 站点和证书已启用。详见文末 [meta-api.com 入口](#meta-apicom-独立-https-入口2026-10-29)。旧域名继续服务，平台默认地址尚未切换。

当前状态（2026-10-17）：`edu.xlrdf.cn`、`edu.gewu.uk` 已按用户要求从新旧跳板停用，不再列入 DNS 切换清单。下文此前验收记录保留其历史时点。

核对日期：2026-10-11。本文记录入口层运维边界；服务器密码、私钥、ACME 账户与 Cloudflare 凭据不进入仓库。实际 IP 以当次交付的迁移结果为准。

## 站点与配置归属

| 域名 | 新机配置 | DNS 切换方式 |
|---|---|---|
| meta-api.vip | `/etc/nginx/sites-available/meta-api.vip` | 修改现有 A 记录 |
| cn.meta-api.vip | `/etc/nginx/sites-available/cn.meta-api.vip`，依赖 `snippets/cn-proxy.inc` | 修改现有 A 记录 |
| edu.meta-api.vip | `/etc/nginx/sites-available/edu.meta-api.vip` | 修改现有 A 记录 |
| emoxia.meta-api.vip | `/etc/nginx/sites-available/emoxia.meta-api.vip`，依赖 `ssl/emoxia-origin-ca.pem` | 修改现有 A 记录 |
| chatshare.tv | `/etc/nginx/sites-available/chatshare.tv` | 修改现有 A 记录 |
| metar.uk | `/etc/nginx/sites-available/metar.uk` | 修改现有 A 记录 |
| www.metar.uk | 与 metar.uk 共用虚拟主机和证书 | 保留指向 metar.uk 的 CNAME |
| edu.gewu.uk | 原 443/8443 独立入口，2026-10-17 已停用 | 不再添加或迁移，可删除旧 DNS 记录 |

原有 Host/SNI、可信源站 CA、请求体限制、WebSocket 头、SSE 缓冲开关、超时、Cookie/重定向改写与日志策略均保留。迁移不修改源站程序、数据库、计费或 API 鉴权。

## 切换前验证

从外部机器指定新入口 IP 测试，不能只在新机运行 `curl localhost`，也不能用 `-k` 跳过证书校验：

```sh
curl --noproxy '*' --resolve "meta-api.vip:443:${NEW_PROXY_IP}" \
  --connect-timeout 10 --max-time 30 \
  -o /dev/null -w 'http=%{http_code} tls=%{ssl_verify_result} total=%{time_total}\n' \
  https://meta-api.vip/api/status
```

逐站核对 HTTP 跳转、HTTPS 页面、证书有效期及旧入口的响应；另测 edu.gewu.uk:8443、www.metar.uk 的主域名跳转和 metar.uk 的验证文件路由。元衡 API 的 `/api/status` 应成功，无凭据请求 `/v1/models` 应返回 401。静态文件对比状态、完整字节数和哈希；首页正常不能替代资源下载验证。

2026-10-11 验收：八个域名的 HTTPS 证书检查成功；七个站点首页返回 200，www.metar.uk 返回预期 301；edu.gewu.uk:8443 返回 200。meta-api.vip 与 cn.meta-api.vip 的状态接口成功、无凭据模型列表返回 401。新旧 Nginx 文件集合校验值一致，首页正文哈希一致。edu.gewu.uk 的 HTTP 在本机测试出现空响应，从另一台服务器请求新入口确认返回 301；该域名公开 DNS 仍为 NXDOMAIN。

metar.uk 的百度验证文件在新旧入口的 HTTP/HTTPS 均返回 200，缺失 ACME 挑战均按预期返回 404；新机 Nginx 错误日志未发现 error/crit/alert/emerg。

同一客户端交替下载 `/assets/semi-ui-SyjLil2-.js`（1,666,733 字节，不压缩）：新机两次完整下载耗时 2.489 秒、2.755 秒，SHA-256 均为 `09a29501022bcbe4b35cd4aaaaf07568d10a50ea86bfb925510d170dcd8a3031`；旧机两次均在 20 秒超时，仅接收 204,376 与 216,664 字节。这是当时客户端网络路径下的短时样本，不能作为服务器带宽上限或长期稳定性结论。

不使用付费模型调用进行入口验收；本轮未覆盖带凭据的真实生成、长时间 SSE/WebSocket 会话或持续负载。

## 证书续期

- Nginx 与 Certbot 使用 Ubuntu 软件包；安装 nginx、webroot 和 dns-cloudflare 所需插件，启用 `nginx.service` 与 `certbot.timer`。
- meta-api.vip 使用 DNS-01；现有凭据文件与账户材料只经 SSH 传到新机，凭据文件权限为 root:600。chatshare.tv 使用 nginx 插件；其余站点使用 webroot。
- `/var/www/letsencrypt` 和 `/var/www/metar-acme` 的 HTTP-01 路径保留；metar 的缺失挑战继续回源，保留源站独立续期能力。
- 七组证书在 2026-12-07 至 2026-12-19 到期。普通 `certbot renew --no-random-sleep-on-renew` 可读取全部配置，当前均未到续期时间，因此跳过；这不等于 ACME 实际签发成功。
- DNS 切换后复验实际公网请求与 HTTP-01 dry-run。edu.gewu.uk 如持续无 DNS，无法完成 HTTP-01 续期；不要将它混同为新入口安装失败。

## DNS 与回退

用户在 Cloudflare 将需要迁移的 A 记录替换为新入口 IP，保持已有代理状态；CNAME 不必改成 A。切换前再次核对是否新增 AAAA 或其他指向旧机的记录，避免部分流量仍到旧机。

旧机保持运行，覆盖 DNS 缓存与已有连接的过渡期。新机验收成功表示可以切换，不表示公网 DNS 已经完成切换。发现异常时将受影响 A 记录恢复旧 IP；DNS 回退也需要缓存消退。切换后确认各站点访问、真实 API 流式请求和证书挑战正常，再另行安排旧机退役。

新机 `/root/qiyun-migration-20261011/` 保存 root 专用的安装后默认配置备份与源配置暂存副本。目录权限 700，私钥权限 600；不作为公开下载、Git 附件或支持工单材料。

## edu.xlrdf.cn 同源入口（2026-10-13）

`edu.xlrdf.cn` 的公开 A 记录指向 `154.44.1.123`，核对时没有 AAAA。该入口与 `meta-api.vip` 转发到同一 New API 源站；域名中的 `edu` 不表示复用 `edu.meta-api.vip` 的站点。

- 独立配置：`/etc/nginx/sites-available/edu.xlrdf.cn`，通过同名 `sites-enabled` 链接启用；访问与错误日志使用 `edu.xlrdf.cn` 前缀。
- 复用 `meta-api.vip` 文件定义的 `meta_nan_origin` upstream 和 `meta_connection_upgrade` map。回源 Host/SNI 仍是 `nan.meta-api.vip`，保留源站 TLS 校验、100 MB 请求体上限、WebSocket、关闭 SSE/请求缓冲和 3600 秒读写超时。单独停用主域名配置前必须保留或迁出这两个共享定义。
- 外部 `X-Forwarded-Host`、源站重定向及 Cookie 域改写到 `edu.xlrdf.cn`。HTTP 保留原路径和查询参数跳转到同域 HTTPS；ACME 挑战路径独立处理，缺失文件返回 404。
- 独立 Let's Encrypt ECDSA 证书位于 `/etc/letsencrypt/live/edu.xlrdf.cn/`，到期时间为 **2027-01-11 11:04:03（北京时间）**。HTTP-01 webroot 为 `/var/www/edu-xlrdf-acme`；沿用已启用的 `certbot.timer` 和校验后 reload Nginx 的 deploy hook。`certbot renew --cert-name edu.xlrdf.cn --dry-run --no-random-sleep-on-renew --run-deploy-hooks` 成功。
- 原 Nginx 文件校验值保持不变。服务器 root 专用备份位于 `/root/edu-xlrdf-domain-20261013T040135Z/`；新增域名回退可停用同名 `sites-enabled` 链接，经 `nginx -t` 后 reload，无需恢复整个 Nginx 目录。

公网验收：新域名首页及 `/api/status` 返回 200，状态接口 `success=true`；无凭据 `/v1/models` 和 `/api/user/self` 返回 401。首页与主域名字节一致；`/assets/index-D1ZUhTil.js` 两侧均为 183,355 字节，SHA-256 均为 `8851326492263ebc9ee105e65b83990c28878c6d800e5fe954295a026c96f97a`。所有成功 HTTPS 探测均正常校验证书，浏览器确认首页和同域 `/login` 正常显示。

本机首次并发探测中，主域名模型列表和新域名状态接口各有一次 30 秒超时；随后分别复测为 401 与 200。新域名日志记录一次客户端中断 499，错误日志为空；现有证据不足以确定超时原因，不将短时验收视为长期稳定性证明。补充的异地服务器探测因 SSH 连接被关闭而未执行；本次访问验收来自用户 Mac 的指定 IP、普通域名及浏览器访问，另有 ACME 签发和续期演练的外部挑战验证。

应用公开的默认服务器地址仍是 `https://meta-api.vip`，首页/文档会继续显示这一默认值；客户端可显式使用 `https://edu.xlrdf.cn/v1`。本次仅增加入口，账户、API Key、渠道、计费和应用版本沿用同一后端；跨域浏览器登录状态不会自动共享。未执行付费模型生成、第三方登录回调或长时 SSE/WebSocket 会话验收。


## Flawless 替换旧跳板（2026-10-17）

新入口为 `155.103.118.155`，旧入口为 `154.44.1.123`。本轮已将旧机当前启用的九个业务虚拟主机（十个域名）及九组证书、源站 CA、ACME 配置和必要 webroot 迁到新机。迁移当时新增覆盖 `edu.xlrdf.cn`、`otd.meta-api.vip`；其余域名见上表。随后停用两个闲置教育域名，当前保留七个业务虚拟主机、八个域名及七组自动续期配置。两个 LibreSpeed 入口 8989/8990 保留。DNS 尚由用户切换，旧机继续运行，业务源站和数据库未修改。

### 配置与优化

- 每个站点配置、共享 snippet、源站 CA 与旧机 SHA-256 一致；保留 Host/SNI、源站 TLS 校验、请求体限制、WebSocket/SSE、重定向和 Cookie 改写。没有重建应用、复制业务数据库或开放通用代理。
- 新机已有 BBR，保持启用。Nginx 每 worker 连接上限从 768 提升到 4096，worker 文件句柄上限 65535；配置 10 MB TLS 会话缓存、一天会话超时、关闭 session tickets，隐藏 Nginx 版本。维持新机 TLS 1.2/1.3 全局配置，不覆盖成旧机全局配置。
- Nginx 和 certbot timer 已启用。九组证书有效；普通 renew 成功读取全部配置并因未到期跳过，不等于实际续期成功。HTTP-01 的最终 dry-run 须在 DNS 切换后完成。
- 备份及源配置在新机 `/root/flawless-migration-20261017/`，根目录权限 700，Cloudflare 续期凭据权限 600。配置、证书及续期凭据经 SSH 数据流直接传到新机，没有写入本地仓库或报告。

### 验收与已知例外

- `nginx -t` 通过，80/443/8443 与测速端口正常监听；MetaR 旧式 `listen ... http2` 产生弃用警告但仍有效，本轮保持站点内容一致。
- 从业务服务器作为独立外部客户端指定新 IP，所有十个域名 TLS 校验成功。除 ChatShare 外，首页按原规则返回 200/301/302；`edu.gewu.uk:8443` 返回 200。
- `meta-api.vip`、`cn.meta-api.vip`、`edu.xlrdf.cn` 状态接口均返回 200/success=true；无凭据模型列表和用户接口均为 401。七个 webroot 域名的临时 ACME 文件从外部读取一致，测试文件已清理。
- 主站脚本 `/assets/index-CkAvx-I-.js` 从新入口完整下载 183355 字节，SHA-256 为 `f0dc2e79e803886fabb60c4c34fdef8c6f70b67b023f940593692a16f380eb24`。已与旧入口完整响应的 SHA-256 核对一致。
- ChatShare 初次本机探测为 200，后续新旧入口均为 502；新机日志显示 `198.12.92.210:60403` 拒绝连接，直接请求源站三次同样失败。配置迁移完成，但不能将该站报告为业务验收通过；建议暂缓切换此域名，先恢复源站。
- 本机当前 VPN/代理路径对主域名和 edu.gewu.uk 的指定 IP TLS 探测失败，其他域名成功；异地探测这两个域名成功。该差异未归因，不能用异地成功代替用户网络验收，也不能推定新机配置有误。
- 未使用付费模型生成、未执行真实带凭据长时 SSE/WebSocket 会话和持续负载测试。保留转发配置不等于这些流程已经实测。

### 用户切换清单

1. 对需要切换且已经通过验收的站点，将仍指向旧跳板的 A 记录替换为 `155.103.118.155`；保留现有橙云/灰云状态。`www.metar.uk` 如为 CNAME，继续指向 `metar.uk`。检查是否有遗留 AAAA。
2. 主域名历史上由 Cloudflare Load Balancing 的 `meta-primary` / `cn2-primary` 池控制。如果目前仍启用，应修改实际启用池内的旧源站 IP，并等待原健康检查恢复；不要仅改普通 A 记录。本轮没有读取或修改 Cloudflare 控制面。
3. `nan.meta-api.vip` / `ai.meta-api.vip` 是业务源站入口，不在此次旧跳板虚拟主机集合中，保持现有记录。`download.meta-api.vip` 等其他服务也不随旧跳板批量替换。
4. 切换后核对解析/负载均衡、各域名访问、新机日志、真实 API 流式请求及 HTTP-01 dry-run。发现问题时恢复原 A 记录或池内源站为旧 IP；旧机暂不退役。

本轮为入口运维和文档更新，无应用版本、GitHub Release 或应用容器部署。


## 停用闲置教育域名（2026-10-17）

按用户要求，`edu.xlrdf.cn`、`edu.gewu.uk` 的 `sites-enabled` 链接、`sites-available` 配置及各自的 Certbot renewal 配置均从新旧跳板的活动目录移走，备份在两机 `/root/retired-edu-domains-20261017/`（700）。不再为这两个域名启用独立虚拟主机或自动续期；保留现有证书材料与 webroot 以便恢复。共享 `meta_nan_origin` / `meta_connection_upgrade` 仍由主站提供，不影响其他入口。

两机 `nginx -t` 通过并完成平滑 reload。`edu.meta-api.vip` 继续使用，不在此次停用范围。Cloudflare DNS 未修改，用户可删除这两个闲置域名的记录，也不应将其加入新跳板切换清单。恢复时将对应三个配置文件/链接移回原路径，先 `nginx -t` 再 reload，并确认旧证书仍有效。

## meta-api.com 独立 HTTPS 入口（2026-10-29）

DNS 准备阶段为主域名配置了指向 `meta-api.vip` 的 CNAME，以跟随旧域名负载均衡；其余 75 个业务子域名复制现有 A/CNAME 目标及代理状态，邮件和验证记录没有复制。DNS 成功不代表 TLS 已接入：跳板缺少 `meta-api.com` 站点和证书时，访问会返回 TLS `unrecognized name`。

用户随后要求主域名直接解析跳板并修复 HTTPS。当前配置：

- 主域名为 `A meta-api.com → 155.103.118.155`，仅 DNS、TTL 自动；不再依赖 `.vip` CNAME，也不再跟随其负载均衡切换。其他 75 条记录未修改。
- [站点配置](../deploy/reverse-proxy/meta-api.com.conf)对应 `/etc/nginx/sites-available/meta-api.com` 和同名 `sites-enabled` 链接。复用主站的 `meta_nan_origin` upstream 与 `meta_connection_upgrade` map；这些共享定义仍由 `meta-api.vip` 配置提供。
- 回源到现有业务服务，Host/SNI 仍为 `nan.meta-api.vip`，严格校验证书；保留 100 MB 请求体限制、WebSocket、关闭 SSE/请求缓冲和 3600 秒超时。新站的对外 Host、重定向和 Cookie 域为 `meta-api.com`，日志独立。
- HTTP 除 `/.well-known/acme-challenge/` 外，按原路径/查询参数跳转到同域 HTTPS。独立 HTTP-01 webroot 为 `/var/www/meta-api-com-acme`，缺失挑战返回 404。现有 Cloudflare 凭据无法查询新 zone，因此使用 HTTP-01，不扩展凭据权限。
- 独立 Let's Encrypt ECDSA 证书位于 `/etc/letsencrypt/live/meta-api.com/`，有效期至 **2027-01-27 11:14:51（北京时间）**。复用既有 ACME 账户、`certbot.timer` 和 deploy hooks；实际签发与 `certbot renew --cert-name meta-api.com --dry-run --no-random-sleep-on-renew --run-deploy-hooks` 均成功。
- 备份和激活记录保存在服务器 `/root/meta-api-com-20261029/`（0700）。新增前的 Nginx 普通文件哈希全部保持一致；仅新增站点并在 `nginx -t` 通过后平滑 reload，没有停止或更新应用。

该入口复用现有用户、API Key 和计费。平台 `ServerAddress` 仍是 `https://meta-api.vip`，文档导航仍指向 `docs.meta-api.vip`；第三方登录、Passkey、支付/邮件回调等正式迁移单独处理。其余新子域名的 DNS 记录不表示它们的 HTTPS 或 CloudFront/R2 等服务绑定已完成。

验收：Mac 系统代理路径及独立服务器均通过严格 HTTPS 校验，新旧首页/登录页/状态接口 200，状态 `success=true`，无凭据 `/v1/models` 和 `/api/user/self` 均 401；首页和 241318 字节入口脚本的 SHA-256 一致。Chrome 实际打开 `.com` 首页成功。独立服务器短流探测 nonce/sequence 匹配，消息间隔 350/350 ms，响应 no-store；Mac 代理样本首两条合并到达，不能将独立服务器的时间表现外推到所有客户端。本机直连与 SSH 曾出现握手超时，使用稳定连接完成配置与验收。未执行真实账号登录、付费生成、第三方回调或长时流式测试。

回退新增入口时，只停用 `sites-enabled/meta-api.com` 并在配置检查通过后 reload；不要恢复整个 Nginx 目录或停止应用。保留配置、证书和独立日志用于恢复，停止长期使用时另行停用对应续期配置。主域名 A 记录后续必须随跳板地址单独维护。本次是入口配置与运维文档更新，不发布应用版本。
