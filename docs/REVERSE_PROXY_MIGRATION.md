# 反代入口迁移与切换验收

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
| edu.gewu.uk | `/etc/nginx/sites-available/edu.gewu.uk`，含 443/8443 | 核对时为 NXDOMAIN；需要启用时添加 A 记录 |

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
