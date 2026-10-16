# Agent Note: XTerminal 源站专用 SSH 密钥

Status: implemented

## Problem

源站现有 Ed25519 私钥带口令，Mac 的既有 SSH 路径可用，但用户忘记口令，XTerminal 导入时提示未提供 passphrase。只验证 Mac 的旧连接就关闭密码会让日常客户端失去访问能力。

## Decision

为同一运维用户生成独立的 XTerminal Ed25519 密钥，保存到用户指定的本地认证目录，私钥权限 0600，不设置口令；服务器仅追加公钥并保留原有密钥。先备份 authorized_keys，再通过禁用 agent、密码、配置继承和连接复用的全新连接验证新密钥及服务端成功认证指纹。私钥和运行日志不进入 Git。

XTerminal 的凭据导入、口令字段清空和保存由用户本人完成。服务器密码认证保持开启，直到 XTerminal 独立新连接的公钥认证成功被确认；关闭密码属于下一阶段，不能由终端验证成功代替。

## Existing capability and note audit

功能地图已记录 SSH Fail2ban 防护；源站已有公钥认证及两把授权公钥。检索密钥迁移、key-only、PasswordAuthentication、XTerminal 和 SSH 自动封禁，仅 [Fail2ban 笔记](../../implemented/feature/2026-10-17-ssh-fail2ban-protection.md) 部分重叠，其自动封禁与保留密码的原决定保持。本篇承接独立客户端密钥接入，不改变 Fail2ban、Nginx 或业务服务。

## Alternatives considered

- 复用加密旧密钥：已有服务器授权且保护落盘密钥，但用户不记得口令，不能满足 XTerminal 当前导入要求；保留它供现有客户端使用。
- 为新密钥再次设置口令：可保护文件被复制后的风险，但仍需 XTerminal 正确传入口令；本次用免口令专用密钥简化接入，明确以本地文件保护承担相应风险。
- 立即关闭密码登录：减少暴力猜测入口，但 XTerminal 尚未验收，不满足用户日常登录连续性要求。

## Testing

新私钥权限 0600，创建前检查同名文件和符号链接均不存在；本地仓库的 Git ignore 排除认证目录。源站 authorized_keys 原两把公钥保持并备份，追加后 root 所有、0600。通过不继承 SSH 配置、禁用 agent/连接复用/密码和 keyboard-interactive 的独立连接，验证新密钥登录；服务端 2026-10-17 06:43:06（北京时间）记录匹配的 Accepted publickey 指纹。SSH、Fail2ban active，应用状态接口 200；原有加密密钥已用于本次追加操作。指纹、回退和验收边界见 [接入说明](../../../../scripts/server-security/SSH_KEYS.md)。

## Consequences

免口令私钥被复制后可用于源站 root 登录，因此必须由用户妥善保护，不共享、不明文上传或提交 Git。目录是否由用户同步到其他设备不由本轮推断。此阶段完成不代表已关闭密码登录，也不代表 XTerminal 已成功接入。
