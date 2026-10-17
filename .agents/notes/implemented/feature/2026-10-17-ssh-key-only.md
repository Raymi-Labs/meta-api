# Agent Note: 源站关闭 SSH 密码认证

Status: implemented

## Problem

源站 SSH 暴露密码猜测入口。用户已完成 XTerminal 专用密钥导入并确认登录成功；服务端 2026-10-17 14:55:32 和 14:55:45（北京时间）记录其既有跳板来源的新密钥成功认证，独立终端也通过同一密钥登录。关闭密码的客户端前置条件现已满足。

## Decision

复用既有公钥和 root 运维身份，通过早于 cloud-init 的 00-newapi-key-only.conf 固定 AuthenticationMethods publickey，关闭密码和 keyboard-interactive，保留 root 公钥登录及 PAM 会话管理。只 reload SSH，不重启主机或业务容器。

保留一个管理连接；变更前备份有效配置和所涉及的原配置，先启动 15 分钟自动回退，再原子安装片段。回退只撤销本轮片段，使用锁和提交标记避免与验收提交竞态。语法及实际连接上下文的有效配置验证后 reload；全新密钥连接、服务器认证方法探测和 XTerminal 重连通过后取消回退，并再次确认配置没有回滚。

## Existing capability and note audit

功能地图已有 Fail2ban 和 XTerminal 专用密钥接入。检索 PasswordAuthentication、key-only、密钥迁移和 XTerminal，[Fail2ban](../../implemented/feature/2026-10-17-ssh-fail2ban-protection.md) 与 [客户端密钥](../../implemented/feature/2026-10-17-xterminal-ssh-key.md) 为部分重叠：保留其原有防护及客户端密钥取舍，本篇承接完成客户端迁移后的服务器认证收紧。Flawless 已禁止密码认证，无需重复修改。

## Alternatives considered

- 只设置 PermitRootLogin prohibit-password：能关闭 root 密码入口，但不能表达所有 SSH 登录仅公钥的目标；同时关闭密码和 keyboard-interactive，并固定认证方法。
- 将配置放在 99- 前缀：便于辨认本地覆盖，但 OpenSSH 采用首次值，50-cloud-init.conf 的 PasswordAuthentication yes 会优先生效，因此使用 00- 前缀并验证有效配置。
- 同时创建新 sudo 用户并禁止 root：可进一步分离日常运维，但会改变现有脚本、客户端和权限流程；本次保持已验证的 root 公钥访问。

## Testing

sshd -t 通过；默认、两个实际 root 来源和普通用户上下文有效值均为仅公钥，root 显示同义值 without-password。旧 NAN 密钥和 XTerminal 新密钥均可建立独立连接；服务端 15:04:00（北京时间）确认来自既有跳板的 XTerminal 新密钥认证，用户也确认新会话成功。禁用公钥的客户端只收到 publickey 并以 Permission denied 拒绝，没有提交实际密码；首次探测因本机网络路径未取得认证方法，绑定物理接口的复测通过。

15:05 提交事务并停止临时回退 timer；systemd 回收后 timer/service 均 inactive、LoadState not-found，备份无 rolled-back 标记。15:06 全新密钥连接再次通过，SSH 和 Fail2ban active，应用与源站 HTTPS 状态接口 200。部署片段与仓库 SHA-256 一致，原三把授权公钥保持。备份位置及逐步恢复边界见 [运维说明](../../../../scripts/server-security/SSH_KEYS.md)。

## Consequences

密码 SSH 客户端后续需要迁移密钥。密钥文件丢失或设备故障时需备份密钥或云控制台恢复。仅密钥认证能去掉密码猜测入口，不能排除应用漏洞、密钥泄露或证明主机从未被入侵。
