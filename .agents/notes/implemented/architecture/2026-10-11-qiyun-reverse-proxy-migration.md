# Agent Note: 七云反代入口迁移与 DNS 切换边界

Status: implemented

## Problem

旧七云入口被供应商限流，用户已购买新机并授权迁移全部反代站点；DNS 由用户在 Cloudflare 修改。迁移必须保留每个站点的源站、Host/SNI、TLS 校验、长连接和 ACME 行为，不能仅凭首页响应判断所有入口迁移完成。

新增同用途域名时，必须延续已验证的入口规则，并明确证书、对外 Host 和续期归属；`edu.xlrdf.cn` 对应主 API 站点，不能仅凭 `edu` 前缀选择既有教育站点配置。

## Decision

复用现场 Nginx 配置、证书及续期配置，在新机独立启动并通过指定 IP 的 HTTPS 请求验收。旧机继续服务，DNS 切换与旧机退役是独立步骤；本轮不改 DNS、不改源站应用和数据库。

配置与证书经旧机到新机的 SSH 管道传送到 root 专用暂存目录，不将私钥、密码、ACME 账户材料写入仓库或本地报告。新机原为空机，安装所需软件包后先备份默认配置，再复制源配置、校验并启用服务。

复用 DNS-01、nginx 与 webroot 三种既有续期方式。主域名的 Cloudflare 凭据随续期配置经 SSH 迁移，文件保持 root:600；不执行 DNS 切换或提前签发证书。具体站点、验证步骤及回退边界见 [运维说明](../../../../docs/REVERSE_PROXY_MIGRATION.md)。

同源新增入口 `edu.xlrdf.cn` 使用独立 Nginx 虚拟主机、日志和 HTTP-01 证书，复用主域名定义的 upstream/map；仅将外部 Host、重定向和 Cookie 域改写为新域名，源站 Host/SNI、证书校验及流式参数保留。新增入口沿用已有 certbot timer/deploy hook，并实际签发证书及演练续期；不扩展主域名证书，不改变应用默认服务器地址。

## Existing capability and scope

- 原迁移（2026-10-11）启用七个反代虚拟主机，覆盖八个域名：meta-api.vip、cn.meta-api.vip、edu.meta-api.vip、edu.gewu.uk、emoxia.meta-api.vip、chatshare.tv、metar.uk、www.metar.uk；edu.gewu.uk 另有 8443 入口。
- 元衡 API 沿用 `/api/status` 公开状态入口和 `/v1/models` 既有鉴权。反代迁移不修改 router、controller、service、model 或计费。
- 活跃笔记检索关键词“七云、reverse.proxy、反代、跳板、metar.uk、chatshare.tv”无命中，无需替换已有决定。
- 2026-10-13 新域名接入检索命中本篇，归属为现有入口复用决定的扩展，直接更新；原迁移证据保留当日范围。

## Flawless 入口替换（2026-10-17）

同一迁移决定继续适用于 Flawless：复制旧跳板当前九个业务虚拟主机、十个域名、九组证书与续期依赖，保持站点文件和上游安全语义；仅保留新机全局 TLS 配置并调整连接容量、TLS 会话复用，保留已有测速服务。新旧服务器的业务源站与 DNS 不在本轮修改范围。旧站点、共享依赖和证书核对结果见运维说明。

本次独立外部探测中主 API 状态/鉴权及多数站点通过，ChatShare 源站拒绝连接导致新旧入口同为 502，单独列为未通过；当前本机代理路径也有部分 TLS 失败，不能宣称全部客户端验收完成。Cloudflare 控制面未核实，切换时需检查历史负载均衡池；源站域名 nan/ai 不属于本次旧跳板迁移清单。旧机保留回退，续期 dry-run 和真实长时 API 会话在切换后复验。

## Alternatives considered

- 重建精简反代配置：结构更统一，但会增加遗漏重定向、WebSocket、SSE、可信源站 CA 与独立 ACME 回源规则的风险；本轮复用已经运行的配置。
- 先改 DNS 再逐站安装：可以立即转移流量，但未备齐证书或依赖时会产生用户可见故障；采用预先指定 IP 验收后切换。
- 将新域名直接加入主站 `server_name` 和证书 SAN：配置更少，但会耦合两个不同 DNS 区的验证、证书续期及回退。新入口采用独立证书和虚拟主机，共享已定义的 upstream/map，保留主要转发行为。

## Testing

- `nginx -t` 成功，Nginx 和 Certbot timer 已启用且 active；80/443/8443 保留。新旧 `/etc/nginx` 普通文件集合 SHA-256 一致。
- 外部指定新 IP 访问八个域名，TLS 校验全部成功；七个站点首页返回 200，www.metar.uk 返回预期 301；首页哈希与旧机一致，edu.gewu.uk:8443 返回 200。
- meta-api.vip 与 cn.meta-api.vip 的 `/api/status` 返回 200 且 success=true，无凭据 `/v1/models` 返回 401。
- HTTP 跳转符合原行为；edu.gewu.uk 的本机 HTTP 探测出现空响应，但从另一台服务器访问新机返回 301。该域名公开解析为 NXDOMAIN，需要用户按使用意图添加记录。
- 六个 webroot 域名使用新机临时标记文件验证路由，完成后删除标记。七组证书均有效；普通 renew 读取全部配置后因未到期而跳过，未进行实际 ACME 签发。
- metar.uk 的 HTTP/HTTPS 验证文件路由返回 200，缺失 ACME 挑战返回 404，与旧机一致；新机错误日志无 error/crit/alert/emerg。
- 同一客户端交替下载同一个 1,666,733 字节静态资源，新机两次完整响应分别为 2.489 秒、2.755 秒且哈希一致；旧机两次均在 20 秒超时，详见运维说明。不将这个客户端样本换算为供应商标称带宽或稳定性保证。
- 2026-10-13 新域名公开 DNS 指向当前跳板，无 AAAA；HTTP-01 临时挑战的公网路径通过，实际证书签发及 `renew --dry-run --run-deploy-hooks` 成功。Nginx 检查通过，既有配置哈希不变，服务和续期 timer 均 enabled/active。
- 新域名首页、静态脚本与主域名哈希一致，状态接口返回 200/success=true，无凭据 API 返回 401；浏览器显示同域首页和登录页。初次探测两次超时后复测正常，新域名有一条客户端中断 499 且错误日志为空，不推定超时根因或长期稳定性。

## Consequences

全部站点可以在保留原有协议行为的前提下切换入口，故障回退只需恢复对应 DNS 目标。代价是过渡期需维护两套入口与相同续期凭据，源配置暂存副本必须保持 root 专用权限。

原迁移由用户独立切换 DNS，迁移安装验收不代替切换后的公网访问和 HTTP-01 续期复验。edu.gewu.uk 没有公开 DNS 时无法完成 HTTP-01 续期。复制证书仅改变 TLS 入口位置，不消除源站已有故障；用户流量在 DNS 缓存消退前可能分散。短时 HTTP/静态资源检查不构成长期稳定性或真实带凭据 SSE/WebSocket 会话的证明。入口运维与文档变更不发布应用新版本。

`edu.xlrdf.cn` 的 DNS 已由用户设置且现场验证成功，独立证书续期也已验证；其生命周期不依赖主域名证书，但仍依赖主配置中的 upstream/map。后续停用主配置时须先迁出共享定义。新增域名保留同一账户和 API Key 体系；浏览器登录状态按域隔离，第三方回调及长时流式会话仍需按使用场景单独验收。新增入口仅为服务器配置与运维文档更新，不发布应用版本。

2026-10-17 用户明确停用闲置的 `edu.xlrdf.cn`、`edu.gewu.uk`：新旧机的独立站点和自动续期配置移入 root 专用恢复目录，不改共享 upstream/map；保留 `edu.meta-api.vip`。现存七个业务虚拟主机、八个域名；历史新增与迁移验收仍表示对应时点，不代表这两个入口当前启用。DNS 删除仍由用户完成。
