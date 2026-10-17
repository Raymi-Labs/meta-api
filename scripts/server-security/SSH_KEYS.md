# XTerminal 源站密钥登录

本流程把客户端接入与关闭密码认证分开。公钥可以追加到服务器，私钥只保存在用户指定的本地认证目录，不能进入本仓库、聊天或运行日志。现有 Mac SSH 别名 `NAN` 的登录成功不能代替 XTerminal 的验证。

## 当前阶段（2026-10-17，北京时间）

源站现有私钥带口令，用户忘记口令，XTerminal 导入提示未提供 passphrase。为同一管理员新增独立 Ed25519 密钥 `nan_newapi_xterminal`，私钥为免口令、权限 0600；本地服务器清单登记用途、目标和指纹。认证目录已被其所在本地仓库的 Git ignore 排除。

服务器只追加公钥，原有两把公钥保持，`authorized_keys` 为 root 所有、0600。备份在 `/root/ssh-key-setup-20261016T224236Z-5qeo0xwf/authorized_keys.before`；其中只有原公钥授权记录，没有私钥。追加操作通过临时文件原子替换，并检查写入前原文件未被并发修改。

独立验证使用 `-F /dev/null`、`IdentityAgent=none`、`IdentitiesOnly=yes`、`ControlPath=none`，关闭密码及 keyboard-interactive，保留严格主机密钥校验。北京时间 06:43:06 的源站日志确认新密钥 `Accepted publickey`，指纹为 `SHA256:N8d/LkjjmLneV6uz0CXDxUizKltWC6zQLxIT/YRbOxQ`。SSH、Fail2ban active，应用状态接口 200。

**源站已于北京时间 14:59 切换为仅公钥 SSH 登录。** 用户完成 XTerminal 导入和保存，14:55 的成功认证满足切换前置条件；15:04:00 在密码关闭后经原跳板再次使用同一新密钥登录，用户也确认新会话成功。原加密密钥与 XTerminal 新密钥均通过独立终端的新连接。仅 reload SSH，无主机或业务容器重启。

## 用户在 XTerminal 操作

1. 编辑 NAN 连接，保留目标地址、root 用户、端口和既有跳板设置；验证方式选“秘钥”。
2. 从本地认证目录加载 `nan_newapi_xterminal`，不要选择 `.pub` 文件。
3. 清空“秘钥密码”，测试连接成功后保存，再新建一个连接。
4. 核对源站成功认证日志中的新密钥指纹，确认实际经过 XTerminal 新建连接；旧会话可用不构成验收。

凭据导入和保存由用户本人在桌面工具完成。免口令密钥的文件本身就是凭据，复制者可能获得源站 root 访问；0600 不能代替设备安全或受保护备份。此流程未验证用户目录的云同步和异地备份策略。

## 仅公钥配置与验收

配置 [00-newapi-key-only.conf](00-newapi-key-only.conf) 安装在 `/etc/ssh/sshd_config.d/`。OpenSSH 采用首次设置值，`00-` 在现有 `50-cloud-init.conf` 之前生效；主文件 Include 位于 root 登录设置之前。有效值为：

```text
PubkeyAuthentication yes
AuthenticationMethods publickey
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin prohibit-password
```

`sshd -T` 将最后一项显示为同义值 `without-password`。PAM 会话管理保持启用；本次关闭 SSH 密码认证，没有更改系统 root 密码或删除原授权公钥。

变更前保留一个管理连接，备份配置，先设置 15 分钟 systemd 临时回退，再原子安装片段。默认和实际 root 来源及普通用户上下文均通过 `sshd -t` / `sshd -T -C` 验证后 reload。禁用公钥的独立探测只收到服务器允许 `publickey`，以 `Permission denied (publickey)` 拒绝，无实际密码被提交。首次探测受本机网络路径超时影响；绑定物理接口的复测通过，不将网络超时当作认证拒绝。

XTerminal、独立新旧密钥连接、SSH/Fail2ban active、本机应用和源站 HTTPS 状态 200 均通过。北京时间 15:05 提交事务并撤销临时回退，复核 timer/service inactive 且已由 systemd 回收，无回滚标记；15:06 的全新密钥登录再次通过。部署片段与仓库 SHA-256 一致。

本次备份：`/root/ssh-key-only-20261017T065917Z-dg3ku4x9/`，含原 sshd 主配置、片段目录、有效配置、候选片段、回退脚本及 committed 标记，不含私钥。回退脚本只撤销本轮片段，并以锁和提交标记防止验收完成后误回滚。事务已提交后，脚本不会再自动恢复密码。

以后需要有意恢复原认证策略时，先通过仍可用密钥或云控制台检查现状并备份，再把本轮 `00-newapi-key-only.conf` 移出 Include 目录；通过 `sshd -t` 并确认 `sshd -T -C` 符合预期后 reload。不要覆盖整个 SSH 目录或恢复陈旧快照，以免破坏后续配置。

## 撤销本次新增密钥

通过仍可用的旧密钥或云控制台备份当前 `authorized_keys`，只移除指纹与上述新密钥一致的授权行，再验证原有密钥仍可登录。不能直接覆盖为早期快照，以免删除之后其他会话追加的合法公钥。客户端删除密钥文件不能代替服务器撤销公钥授权。
