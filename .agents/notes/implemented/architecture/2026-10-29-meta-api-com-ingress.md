# Agent Note: meta-api.com 直连解析与独立 HTTPS 入口

Status: implemented

## Problem

用户已购买 meta-api.com，并要求按 meta-api.vip 准备业务解析。前一阶段已复制 75 个业务子域名，主域名采用指向 meta-api.vip 的 DNS-only CNAME，以跟随旧域名负载均衡。公网 DNS 可用，但跳板没有新域名的 TLS 虚拟主机，HTTPS 握手返回 unrecognized name。用户现在明确要求主域名直接解析当前跳板，并修复访问。

## Decision

主域名使用 DNS-only A，直接指向当前 155.103.118.155；其他子域名不变。[独立站点](../../../../deploy/reverse-proxy/meta-api.com.conf)复用跳板既有 Nginx meta_nan_origin upstream、WebSocket map、源站 Host/SNI 和严格证书校验，使用独立日志及 Let's Encrypt 证书。旧 Cloudflare 凭据查询不到新 zone，因此采用独立 HTTP-01 webroot，沿用现有 Certbot 账户、timer 和 reload hook，不扩大凭据权限。

应用路由、鉴权、计费、默认 ServerAddress 和文档入口不变。仅新增入口配置并平滑 reload，不更新或重启应用。其他子域名的 DNS 准备不表示它们的 HTTPS 和第三方服务绑定也已完成。

## Existing capability and note audit

- [反代迁移规则](../../implemented/architecture/2026-10-11-qiyun-reverse-proxy-migration.md)部分重叠：沿用独立站点/证书与转发参数，保留该规则及历史迁移边界；本篇单独记录用户要求改变 CNAME 方案的决定。
- router/api-router.go 的 /api/status、/api/route-probe 与 /api/route-probe/stream 已可验收匿名状态及无费用短流；/v1/models 与 /api/user/self 可验收鉴权。
- 其余命中“同源”的模型/视频/静态内容笔记与入口 TLS 无关，不修改。

## Alternatives considered

- 保留 CNAME：可跟随旧域名负载均衡且不会引起额外 HTTP 跳转，但继续依赖旧域名；用户明确选择独立 A 记录。
- 只把 CNAME 改成 A：目标 IP 相同，不能修复缺失的 TLS 站点和证书。
- 合并旧站 server_name 和证书 SAN：配置更少，但耦合两个域名的续期与回退；复用独立站点既有模式。

## Verification

Cloudflare 控制面显示主域名 A/IP/仅 DNS/自动 TTL；公共解析返回 155.103.118.155，CNAME 和 AAAA 均无记录。现有 Nginx 普通文件哈希保持不变，新站配置检查和 graceful reload 成功；证书 SAN 为 meta-api.com，到期时间为北京时间 2027-01-27 11:14:51。HTTP-01 实际签发及带 deploy hooks 的续期 dry-run 成功，nginx/certbot.timer active，新站错误日志为空。

用户 Mac 经系统代理及另一台服务器严格校验证书后，新旧域名首页、登录页和状态接口均 200，状态 success=true；无凭据模型列表和用户接口均 401。首页及 241318 字节入口脚本 SHA-256 分别一致。HTTP 保留路径/查询参数并 301 到同域 HTTPS，缺失 ACME 文件为 404。Chrome 实际打开新域名首页成功。

独立服务器的短流探测 nonce/sequence 均匹配，三条消息间隔为 350/350 ms，响应 no-store。Mac 系统代理样本前两条合并到达、第三条延后 192 ms；本机直连和 SSH 曾出现握手超时，切换稳定连接后完成操作，不将该网络现象归因于 Nginx。本次未执行付费模型生成、真实账号登录、第三方回调、长期流式或持续负载验收。

## Consequences

A 记录不再跟随旧域名负载均衡，入口 IP 后续改变时须单独维护。新域名浏览器会话隔离，OAuth/Passkey/支付回调等仍依赖现有平台设置，不能把入口修复描述为正式主域名迁移完成。备份当前配置并验证后平滑 reload；错误时只撤回本次新增站点。

本次仅增加入口配置与运维文档，不发布应用版本。恢复材料位于服务器 /root/meta-api-com-20261029/；凭据和证书私钥不入库。
