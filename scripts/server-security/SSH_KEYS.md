# XTerminal 源站密钥登录

本流程把客户端接入与关闭密码认证分开。公钥可以追加到服务器，私钥只保存在用户指定的本地认证目录，不能进入本仓库、聊天或运行日志。现有 Mac SSH 别名 `NAN` 的登录成功不能代替 XTerminal 的验证。

## 当前阶段（2026-10-17，北京时间）

源站现有私钥带口令，用户忘记口令，XTerminal 导入提示未提供 passphrase。为同一管理员新增独立 Ed25519 密钥 `nan_newapi_xterminal`，私钥为免口令、权限 0600；本地服务器清单登记用途、目标和指纹。认证目录已被其所在本地仓库的 Git ignore 排除。

服务器只追加公钥，原有两把公钥保持，`authorized_keys` 为 root 所有、0600。备份在 `/root/ssh-key-setup-20261016T224236Z-5qeo0xwf/authorized_keys.before`；其中只有原公钥授权记录，没有私钥。追加操作通过临时文件原子替换，并检查写入前原文件未被并发修改。

独立验证使用 `-F /dev/null`、`IdentityAgent=none`、`IdentitiesOnly=yes`、`ControlPath=none`，关闭密码及 keyboard-interactive，保留严格主机密钥校验。北京时间 06:43:06 的源站日志确认新密钥 `Accepted publickey`，指纹为 `SHA256:N8d/LkjjmLneV6uz0CXDxUizKltWC6zQLxIT/YRbOxQ`。SSH、Fail2ban active，应用状态接口 200。

**源站仍允许密码登录。XTerminal 的导入、保存及独立连接尚待用户完成。** 此阶段没有修改 sshd 配置或重启 SSH、应用、数据库。

## 用户在 XTerminal 操作

1. 编辑 NAN 连接，保留目标地址、root 用户、端口和既有跳板设置；验证方式选“秘钥”。
2. 从本地认证目录加载 `nan_newapi_xterminal`，不要选择 `.pub` 文件。
3. 清空“秘钥密码”，测试连接成功后保存，再新建一个连接。
4. 核对源站成功认证日志中的新密钥指纹，确认实际经过 XTerminal 新建连接；旧会话可用不构成验收。

凭据导入和保存由用户本人在桌面工具完成。免口令密钥的文件本身就是凭据，复制者可能获得源站 root 访问；0600 不能代替设备安全或受保护备份。此流程未验证用户目录的云同步和异地备份策略。

## 后续关闭密码的门槛

XTerminal 独立新连接确认成功后，保留现有管理会话和云控制台恢复路径，先备份并启动有时限的回退，再变更 sshd。核对 Include 顺序及 Match，有效配置需关闭密码和 keyboard-interactive，允许公钥。`sshd -t`、`sshd -T -C` 通过后 reload，再分别验证 XTerminal 和独立终端的新连接，以及服务器不再提供密码认证；确认无误后取消回退并复核。

## 撤销本次新增密钥

通过仍可用的旧密钥或云控制台备份当前 `authorized_keys`，只移除指纹与上述新密钥一致的授权行，再验证原有密钥仍可登录。不能直接覆盖为早期快照，以免删除之后其他会话追加的合法公钥。客户端删除密钥文件不能代替服务器撤销公钥授权。
