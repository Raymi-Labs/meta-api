# Agent Note: 七云反代入口迁移与 DNS 切换边界

Status: implemented

## Problem

旧七云入口被供应商限流，用户已购买新机并授权迁移全部反代站点；DNS 由用户在 Cloudflare 修改。迁移必须保留每个站点的源站、Host/SNI、TLS 校验、长连接和 ACME 行为，不能仅凭首页响应判断所有入口迁移完成。

## Decision

复用现场 Nginx 配置、证书及续期配置，在新机独立启动并通过指定 IP 的 HTTPS 请求验收。旧机继续服务，DNS 切换与旧机退役是独立步骤；本轮不改 DNS、不改源站应用和数据库。

配置与证书经旧机到新机的 SSH 管道传送到 root 专用暂存目录，不将私钥、密码、ACME 账户材料写入仓库或本地报告。新机原为空机，安装所需软件包后先备份默认配置，再复制源配置、校验并启用服务。

复用 DNS-01、nginx 与 webroot 三种既有续期方式。主域名的 Cloudflare 凭据随续期配置经 SSH 迁移，文件保持 root:600；不执行 DNS 切换或提前签发证书。具体站点、验证步骤及回退边界见 [运维说明](../../../../docs/REVERSE_PROXY_MIGRATION.md)。

## Existing capability and scope

- 现场启用七个反代虚拟主机，覆盖八个域名：meta-api.vip、cn.meta-api.vip、edu.meta-api.vip、edu.gewu.uk、emoxia.meta-api.vip、chatshare.tv、metar.uk、www.metar.uk；edu.gewu.uk 另有 8443 入口。
- 元衡 API 沿用 `/api/status` 公开状态入口和 `/v1/models` 既有鉴权。反代迁移不修改 router、controller、service、model 或计费。
- 活跃笔记检索关键词“七云、reverse.proxy、反代、跳板、metar.uk、chatshare.tv”无命中，无需替换已有决定。

## Alternatives considered

- 重建精简反代配置：结构更统一，但会增加遗漏重定向、WebSocket、SSE、可信源站 CA 与独立 ACME 回源规则的风险；本轮复用已经运行的配置。
- 先改 DNS 再逐站安装：可以立即转移流量，但未备齐证书或依赖时会产生用户可见故障；采用预先指定 IP 验收后切换。

## Testing

- `nginx -t` 成功，Nginx 和 Certbot timer 已启用且 active；80/443/8443 保留。新旧 `/etc/nginx` 普通文件集合 SHA-256 一致。
- 外部指定新 IP 访问八个域名，TLS 校验全部成功；七个站点首页返回 200，www.metar.uk 返回预期 301；首页哈希与旧机一致，edu.gewu.uk:8443 返回 200。
- meta-api.vip 与 cn.meta-api.vip 的 `/api/status` 返回 200 且 success=true，无凭据 `/v1/models` 返回 401。
- HTTP 跳转符合原行为；edu.gewu.uk 的本机 HTTP 探测出现空响应，但从另一台服务器访问新机返回 301。该域名公开解析为 NXDOMAIN，需要用户按使用意图添加记录。
- 六个 webroot 域名使用新机临时标记文件验证路由，完成后删除标记。七组证书均有效；普通 renew 读取全部配置后因未到期而跳过，未进行实际 ACME 签发。
- metar.uk 的 HTTP/HTTPS 验证文件路由返回 200，缺失 ACME 挑战返回 404，与旧机一致；新机错误日志无 error/crit/alert/emerg。
- 同一客户端交替下载同一个 1,666,733 字节静态资源，新机两次完整响应分别为 2.489 秒、2.755 秒且哈希一致；旧机两次均在 20 秒超时，详见运维说明。不将这个客户端样本换算为供应商标称带宽或稳定性保证。

## Consequences

全部站点可以在保留原有协议行为的前提下切换入口，故障回退只需恢复对应 DNS 目标。代价是过渡期需维护两套入口与相同续期凭据，源配置暂存副本必须保持 root 专用权限。

DNS 仍由用户切换；切换后真实公网访问和 HTTP-01 续期仍需复验。edu.gewu.uk 没有公开 DNS 时无法完成 HTTP-01 续期。复制证书仅改变 TLS 入口位置，不消除源站已有故障；用户流量在 DNS 缓存消退前可能分散。短时 HTTP/静态资源检查不构成长期稳定性或真实带凭据 SSE/WebSocket 会话的证明。本轮仅运维与文档变更，不发布应用新版本。
