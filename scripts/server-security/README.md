# SSH Fail2ban 防护

适用范围：New API 源站和 Flawless 跳板的宿主机 TCP 22。独立运维配置，不修改 New API、数据库、Nginx、SSH 认证方式或业务端口，不发布应用版本。

## 规则

- `sshd` jail 使用发行版自带 `sshd[mode=aggressive]` filter，读取 systemd journal，识别密码失败、无效用户和认证前扫描。
- 10 分钟内累计 6 条失败事件封禁 1 小时；同一地址重复触发按 Fail2ban 默认递增公式延长，最长 24 小时。一次连接可能产生多条失败事件，不能把阈值理解为一定允许尝试 6 次连接。
- 封禁历史保留 7 天，Fail2ban 重启后恢复尚未到期的封禁；服务随系统启动。
- `nftables-multiport` 仅拒绝被封地址到本机 TCP 22 的连接。沿用既有防火墙，不清空其他表/链。
- 只忽略回环和主机自身地址，不把可能变化或共用的公网 VPN 出口长期白名单化。合法用户反复认证失败也可能被临时封禁。

源站的网络对端可能是自有跳板/CDN，且网站登录失败可能返回 HTTP 200；未经可信客户端 IP 和登录结果核对，不对这些 HTTP 日志配置自动封禁。Fail2ban 不是应用防火墙或 DDoS 防护，源站仍允许密码登录。

XTerminal 专用密钥的接入、当前验证状态及关闭密码的门槛见 [SSH 密钥登录](SSH_KEYS.md)。追加公钥不等于关闭密码认证。

## 安装与更新

先核实目标、SSH 端口、有效密钥连接、发行版日志格式与已有 jail。保留一个可用管理连接和云控制台回退入口。已有同名配置时先备份，检查 `/etc/fail2ban/jail.local`、`jail.d` 是否存在更晚覆盖。

在目标主机安装发行版软件包 `fail2ban`，确认 `python3-systemd` 和 `nftables` 可用。复制本目录两个配置到对应位置：

```sh
sudo install -m 0644 99-newapi-sshd.local /etc/fail2ban/jail.d/99-newapi-sshd.local
sudo install -m 0644 99-newapi-retention.local /etc/fail2ban/fail2ban.d/99-newapi-retention.local
sudo fail2ban-client -t
sudo systemctl enable fail2ban
sudo systemctl restart fail2ban
```

配置验证成功后才重启 Fail2ban；无需重启 SSH、Nginx 或 Docker。发行版 filter 的 journal match 会随包版本不同，须以 `get sshd journalmatch` 和实际失败日志匹配为准，不能只检查 active。

## 查询与逐 IP 解封

```sh
sudo systemctl is-enabled fail2ban
sudo systemctl is-active fail2ban
sudo fail2ban-client status sshd
sudo fail2ban-client get sshd journalmatch
sudo fail2ban-client get sshd maxretry
sudo fail2ban-client get sshd findtime
sudo fail2ban-client get sshd bantime
sudo fail2ban-client get sshd bantime.increment
sudo fail2ban-client get sshd bantime.maxtime
sudo fail2ban-client get dbpurgeage
sudo nft list table inet f2b-table
```

确认具体误封来源后，通过仍可用的管理连接或云控制台执行 `sudo fail2ban-client set sshd unbanip <IP>`。不要为一个误封清空整张防火墙，也不要把攻击 IP 列表当作永久配置。日志位于 `/var/log/fail2ban.log`。

完整停用本轮防护使用 `sudo systemctl disable --now fail2ban`，由服务撤销自己的封禁规则。两个配置片段可移入 root 专用备份目录；数据库保留用于审计。未来已有其他 jail 时，先评估整服务停用的影响，或只停用 sshd jail。

## 2026-10-17 现场验收（北京时间）

- 源站 Ubuntu 24.04：Fail2ban `1.0.2-3ubuntu0.1`；跳板 Ubuntu 26.04：`1.1.0-9`。两台配置检查成功、服务 enabled/active，启用 sshd jail；阈值为 6/600 秒、3600 秒封禁、86400 秒上限、604800 秒历史。
- 两台均已自动封禁实际攻击来源。封禁数量为动态值，以现场 `status sshd` 为准。
- 使用临时 Linux network namespace 和 `198.18.255.0/30` 隔离网段发送真实 SSH 失败认证；两台均在 4 次失败连接产生足够失败事件后自动封禁。确认 TCP 22 被阻断，而 80/443 仍连通。
- 在测试地址仍封禁时重启 Fail2ban，确认封禁从数据库恢复并继续阻断 SSH；逐 IP 解封后重新连接成功。测试 namespace、veth、路由及活动封禁已清理；日志/封禁数据库中的验收记录正常保留。
- 两台均通过不复用连接的全新公钥 SSH 登录。成功公钥日志样本分别为39条、64条，filter均忽略且0条记为失败；两台有效SSH配置与备份SHA-256一致，源站数据库防火墙仍保留。部署配置与仓库两个片段SHA-256一致，Fail2ban日志没有ERROR/CRITICAL。
- 源站本机与源站到跳板的独立外部HTTPS检查：状态接口200、无凭据模型接口401，TLS正常验证。本机电脑指定IP检查中两侧状态接口均200；两侧模型接口各出现一次TLS连接超时，均复测为401且TLS验证成功。端口隔离实测和异地请求通过，不将其表述为所有客户端网络均稳定。
- 首次配置快照位于两机 root 专用目录：源站 `/root/fail2ban-setup-20261016T215349Z/`，跳板 `/root/fail2ban-setup-20261016T215503Z/`。目录名采用 UTC，内含包默认配置、原有 nftables 和 SSH 有效配置；不含私钥副本。

运行日志包含访问来源，应留在服务器受限目录，不上传完整日志或认证材料到仓库。
