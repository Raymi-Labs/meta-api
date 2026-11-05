# Agent Note: 桌面浏览器登录交接

Status: implemented

## Problem

网站支持GitHub及自定义Google OAuth，但桌面仅支持密码。浏览器Cookie不能直接交给桌面，不能通过链接传递长期会话或创建新账号替代已有OAuth身份。

## Decision

复用网站OAuth、登录后next跳转、账号状态及setupLogin。新增五分钟设备授权：桌面生成32字节verifier，只提交S256 challenge；服务端生成256位请求ID与核对码，数据库只保存ID摘要。系统浏览器进入/desktop-login，用户完成已有登录并核对码、明确批准当前账号。客户端每3秒用POST携verifier查询，原子消费批准记录后独立签发客户端session Cookie。无自定义协议回调、不绑定本地端口、不把秘密放URL、不新增OAuth client secret。

审批必须是浏览器session（拒API token），同源Origin及会话绑定CSRF；账号禁用、过期、重复审批/兑换、错误verifier、取消、并发竞争均拒绝。客户端取消后丢弃内存verifier及迟到结果，不自动切换账号。能力接口未上线时不伪称支持。

## Alternatives considered

- 内嵌WebView登录：无需交接接口，但Google限制嵌入代理且扩大密码接触面，拒绝。
- 自定义协议回传Cookie：流程短，但URL泄露与协议劫持可暴露长期会话，拒绝。
- 系统浏览器加PKCE绑定的短期设备授权：需服务端和网页先上线，换取多实例原子消费与无凭据回调链接。

## Verification

模拟验证重放、竞争、过期、拒绝、取消、CSRF、不同账号、禁用用户、错误proof；GORM兼容SQLite/MySQL/PostgreSQL。只读能力自检不触发真实授权；真实Google/GitHub交互另验。服务端未部署则客户端不得声称流程已可用。

## Consequences

批准网页本质授予客户端完整账号会话，必须明确显示账号与核对码。网络丢失消费后的响应需要重新发起，不自动重放。记录保留短期、清理不参与授权。生产部署必须复用受保护发布流程，不能仅合并就算上线。

## Local progress

本地实现授权记录/原子状态转换、五个HTTP接口、独立浏览器确认页。模型层覆盖wrong verifier、wrong code、重放、12路消费竞争、取消、过期；控制器覆盖Origin、缺session、账号切换、重复批准、session签发、重复兑换及禁用用户。SQLite测试和网站Vite构建通过，MySQL5.7/PostgreSQL9.6通过独立CI矩阵另验，真实提供商流程留给用户部署后联调；不部署生产、不修改现有OAuth provider配置。用户明确要求先推送Meta、客户端合并发布，服务器最后由用户部署；安装包不宣称已具备线上可用性。17语言词条覆盖，非中文新增提示暂使用英语。
