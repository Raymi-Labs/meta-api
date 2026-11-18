# Agent Note: 站内工单与封禁申诉

Status: proposed

## Problem

平台目前没有能持续往来的站内沟通渠道。用户遇到计费、接口报错、发票或账号问题，只能通过站外渠道联系，管理员也无法在后台集中追踪“谁在等谁回复”。客户端反馈只能单向提交，不支持回复和附件。账号被封禁后，所有登录方式都会被拒绝；内容安全停用日志只写“如需申诉，请联系管理员”，被封用户在站内没有任何入口。

用户在2026-11-18要求一次实现完整工单系统：支持图片附件；被封禁用户可以提交申诉工单；不新增客服角色，所有管理员都能看到全部工单，并支持“指派给我”；等待用户回复满7天自动转为已解决，已解决再满7天自动关闭。

约束：同时兼容SQLite、MySQL ≥5.7.8和PostgreSQL ≥9.6，不使用JSON数据库运算；附件不存入数据库；JSON统一经过`common`包；不新增角色或权限体系。

失败条件：申诉会话能访问普通接口或他人数据；内部备注或其附件暴露给用户；附件可以被用来做路径穿越、伪装类型或存储型XSS；通知邮件被刷屏；被封禁用户通过工单接口绕过封禁继续使用普通功能。

## Existing capabilities and impact

### 已有能力与本次复用

| 用户操作 | 已有入口与数据链路 | 复用或扩展 |
| --- | --- | --- |
| 登录鉴权 | [authHelper](../../../../middleware/auth.go)读取会话里的id、role、status，并要求`New-Api-User`头与会话一致；封禁时返回“用户已被封禁” | 普通工单接口沿用UserAuth/AdminAuth；申诉接口单独鉴权，不放宽authHelper |
| 密码登录 | [Login/setupLogin](../../../../controller/user.go)调用[ValidateAndFill](../../../../model/user.go)，密码错误和账号封禁返回同一句提示 | 密码正确但账号封禁时返回哨兵错误，进入申诉分支；密码错误时提示不变 |
| 其他登录方式 | [OAuth](../../../../controller/oauth.go)、[微信](../../../../controller/wechat.go)、[Passkey](../../../../controller/passkey.go)在登录前检查状态；[Telegram](../../../../controller/telegram.go)和[二步验证](../../../../controller/twofa.go)没有复查状态 | 封禁时统一签发申诉会话；补上Telegram与二步验证的状态检查 |
| 客户端反馈 | [desktop_feedback](../../../../model/desktop_feedback.go)只收集问题和内部状态，见[客户端报错笔记](../../implemented/feature/2026-10-26-desktop-feedback.md) | 保留原功能，不合表；复用请求头和私钥两条脱敏规则 |
| 通知 | [NotifyUser/NotifyRootUser](../../../../service/user_notify.go)按用户设置发送邮件、Webhook、Bark或Gotify；[CheckNotificationLimit](../../../../service/notify-limit.go)对同一用户同一类型限频 | 新增`ticket_new`、`ticket_reply`两种类型；正文只放工单号、转义后的标题和链接 |
| 后台定时任务 | [企业维护任务](../../../../service/enterprise_maintenance_task.go)只在主节点运行，用防重入标记 | 新增工单自动流转任务，按同样方式启动 |
| 文件保存 | [发票附件](../../../../controller/invoice_attachments.go)写入`data/`下按年月分的目录，用随机文件名，写入失败就删除 | 工单图片按同样方式保存到`data/tickets/` |
| 请求日志 | [logs.request_id](../../../../model/log.go)是有索引的varchar(64) | 工单可以关联一个请求ID；管理端按“请求ID+工单所属用户”查询日志 |
| 按用户限流 | [userRateLimitFactory](../../../../middleware/rate-limit.go)从上下文读取id | 新建、回复、申诉分别限流；申诉鉴权要先写入id |
| 侧栏与铃铛 | [SiderBar](../../../../web/src/components/layout/SiderBar.jsx)、[useSidebar](../../../../web/src/hooks/common/useSidebar.js)、[useNotifications](../../../../web/src/hooks/common/useNotifications.js) | 新增“我的工单”“工单管理”两个模块及未读角标；铃铛加入工单数量 |
| 使用日志 | [UsageLogsTable](../../../../web/src/components/table/usage-logs/UsageLogsTable.jsx)的展开行显示Request ID | 增加“提交工单”入口，自动带上请求ID |

### 本轮源码确认的缺口

| 现状 | 影响 |
| --- | --- |
| `ValidateAndFill`不区分“密码错误”和“账号封禁” | 无法只对已证明身份的封禁用户开放申诉 |
| `TelegramLogin`查到用户后直接调用`setupLogin`，没有检查状态 | 封禁账号可以通过Telegram建立登录会话；普通接口会被authHelper拦下，但这本身是状态不一致 |
| `Verify2FALogin`验证通过后直接`setupLogin`，没有复查状态 | 等待二步验证期间被封禁的账号仍能完成登录 |
| 前端`showError`遇到401会清空本地用户并跳转登录页 | 铃铛轮询和申诉页请求必须跳过全局错误处理，否则会误把用户踢出登录 |
| 会话中的status是登录时写入的缓存 | 登录后才被封禁的用户，旧会话仍能通过authHelper；工单写入必须在数据库中复查状态 |

### 相关笔记

检索`proposed/implemented/rejected`中与工单、申诉、封禁相关的笔记，只有[客户端报错笔记](../../implemented/feature/2026-10-26-desktop-feedback.md)相关。该笔记当时明确不做“支持回复和附件的完整客服工单”，所以本方案新建主笔记，不改写它的决定；实现后在那份笔记补一条反向链接。

### 不改变的产品规则

- 封禁用户依然不能使用普通控制台、令牌和接口。申诉会话只能访问本人的申诉工单。
- 不新增客服角色；所有管理员看到全部工单，超级管理员没有额外的工单特权。
- 客户端反馈继续独立存在，不迁移到工单表。

## Proposal

### 1. 数据与状态

新增三张表，全部通过GORM迁移，同时在`migrateDB`和`migrateDBFast`中登记：

- `tickets`：用户、标题（≤100字符）、分类（billing、api、account、invoice、enterprise、appeal、other）、状态、指派人、请求ID（≤64，字符集`[A-Za-z0-9._:-]`）、`user_unread`与`staff_unread`两个布尔值、最近消息时间、状态变更时间、创建与更新时间。
- `ticket_messages`：工单、发送人、发送方（user、staff、system）、事件（如`status_resolved`）、正文（text，≤5000字符）、是否内部备注、创建时间。
- `ticket_attachments`：工单、消息、显示用文件名、MIME、大小、存储路径（不出现在JSON中）、创建时间。

状态为open、in_progress、waiting_user、resolved、closed：

- 管理员公开回复后默认转为waiting_user，也可以在回复时选择处理中、已解决或已关闭。内部备注不改变状态，也不通知用户。
- 用户在waiting_user或resolved状态下回复，工单重新变为open。closed之后用户不能再回复，只能新建工单；管理员可以通过修改状态重新打开。
- 用户可以关闭自己的工单。
- 管理员修改状态、用户关闭和自动流转都写一条事件消息，用户可见；指派变化不写消息。
- 所有状态变化都使用带原状态的条件更新，并发时只有一方生效。

### 2. 用户与管理接口

用户接口挂在`/api/user`的UserAuth分组下：

- `POST /tickets`：新建。每账号每10分钟最多5次；同一账号同时处于open、in_progress、waiting_user的工单不超过5个；在事务里复查账号为启用状态。普通用户不能选择appeal分类。
- `GET /tickets/self`、`GET /tickets/self/unread`、`GET /tickets/self/:id`（打开即清除用户未读）。
- `POST /tickets/self/:id/reply`（每10分钟最多30次）、`POST /tickets/self/:id/close`、`GET /tickets/self/:id/attachments/:aid`。
- 写操作都在数据库中复查账号状态，避免登录后被封禁的旧会话继续写入。

管理接口为`/api/tickets`，使用AdminAuth：

- `GET /`：按状态、分类、指派人（我、未指派）、关键词、未读筛选并分页。
- `GET /pending-count`：统计`staff_unread`为真且状态为open或in_progress的工单。
- `GET /:id`：返回工单、全部消息（含内部备注）、附件、用户摘要，以及关联请求ID在该用户名下最多5条日志；打开即清除管理端未读。
- `POST /:id/reply`：支持内部备注和回复后状态。
- `PATCH /:id`：修改状态、指派人（只能指派给启用中的管理员，或取消指派）和分类。
- `GET /:id/attachments/:aid`。

### 3. 封禁申诉会话

- 密码正确但账号被封禁时：如果账号开启了二步验证，仍然先走原有的待验证流程，验证通过后再签发申诉会话；否则直接签发。OAuth、微信、Telegram和Passkey在确认身份后同样处理。
- 签发申诉会话时清除正常登录和待验证的会话键，只写入`appeal_user_id`和`appeal_expires_at`（2小时）。接口返回`success:false`、封禁提示，以及`data.appeal_available:true`；前端据此跳转`/appeal`。
- `setupLogin`和退出登录都会清除申诉键。
- 新增AppealAuth中间件：会话未过期、用户存在（未软删除）且仍处于封禁状态才放行，并写入id供限流使用。账号已解封时清除申诉键，提示重新登录；会话过期时返回401，前端用`skipErrorHandler`自行处理。
- 申诉接口`/api/appeal`：`GET /session`、`GET|POST /tickets`、`GET /tickets/:id`、`POST /tickets/:id/reply`、`GET /tickets/:id/attachments/:aid`，只能访问本人category为appeal的工单；同时最多1个未关闭的申诉。不依赖`New-Api-User`头（申诉页没有本地登录用户）；Cookie本身是SameSite=Strict，跨站请求不会带上会话。
- 账号解封后，用户在普通工单列表里能看到自己之前的申诉工单。
- 内容安全停用日志改为提示登录后按提示提交申诉工单。

### 4. 附件

- 只接受PNG、JPEG、WebP，拒绝SVG。单张≤5MB，每条消息最多3张，单像素边≤20000且总像素≤4000万。
- 用`http.DetectContentType`判断类型，再用`image.DecodeConfig`确认文件确实能解析为对应格式；不信任上传者给出的类型和文件名。
- 文件写入`data/tickets/YYYY/MM/`，文件名随机生成；数据库写入失败时删除已写入的文件。显示用文件名去掉路径和控制字符。
- 下载接口校验访问权限，用户和申诉方看不到内部备注的附件。响应带固定的Content-Type、`X-Content-Type-Options: nosniff`和`Cache-Control: no-store`。前端用带鉴权头的请求取回blob后显示。
- 上传请求在处理函数里用`http.MaxBytesReader`限制在约16MB，不使用会把整个请求体读进内存的匿名请求体限制。

### 5. 通知与自动流转

- 新工单通知超级管理员；管理员公开回复通知工单所属用户（申诉工单的链接指向登录页，普通工单指向工单详情）；用户回复通知指派人，没有指派人时通知超级管理员。
- 通知异步发送，正文只包含工单号、经过HTML转义的标题和链接，不带回复正文或附件。沿用既有的同类通知限频。
- 定时任务只在主节点运行，每30分钟执行一次：waiting_user满7天转resolved，resolved满7天转closed。每批200条，逐条条件更新并写事件消息；不发送通知。

### 6. 前端

- `/console/tickets`：我的工单列表、新建弹窗（支持`?create=1&request_id=`预填）、详情（`?id=`）、回复、关闭、图片预览。
- `/console/ticket-admin`：管理端列表、筛选、详情、回复/内部备注、修改状态和分类、指派给我。
- `/appeal`：公开路由，显示当前申诉账号、申诉记录、新建与回复。
- 侧栏新增两个模块并显示未读角标；后台“侧边栏模块”和个人侧栏设置同步加入新模块。铃铛未读数加入工单数，并新增工单页签。
- 使用日志展开行增加“提交工单”。
- 未读数请求只在已登录时发起，并跳过全局错误处理。

## Alternatives considered

- **扩展客户端反馈为工单。** 能复用现有表和后台页面，改动最少；但反馈表的字段（版本、系统、错误摘要、报告ID幂等）是为单向报告设计的，加入对话、指派、状态流转和附件后两种语义会混在一起，客户端已有的幂等约定也要随之改变。选择新建工单表，反馈保持原样。
- **接入外部工单系统或客服邮箱。** 功能成熟，不用自建状态机和附件存储；但需要另一套账号体系，无法直接关联平台用户、请求日志和封禁状态，被封用户的身份也难以验证。用户要求站内处理，所以不采用。
- **新增客服角色。** 权限划分更细，可以让客服只看工单；但需要改动角色常量、鉴权、侧栏权限和用户管理，团队目前也没有独立客服。用户选择所有管理员可见并支持“指派给我”。
- **公开表单加邮箱提交申诉。** 不需要改动登录链路；但任何人都能以他人名义提交，管理员要靠邮件往来核实身份，也容易被刷。选择在完成原有登录验证（含二步验证）后签发只能访问申诉的会话。
- **在普通登录会话上给封禁用户加接口白名单。** 不需要新的会话键；但authHelper和所有依赖会话的中间件都要识别这种“半登录”状态，任何遗漏都会让封禁用户访问到普通接口。选择独立的申诉会话键和中间件，普通鉴权保持不变。
- **附件存数据库BLOB。** 备份和迁移简单，多实例天然共享；但三种数据库的二进制类型和单行上限不同，大字段也会拖慢备份和查询。沿用发票附件存放在`data/`目录的方式；多实例部署需要共享该目录，这是已有发票附件同样存在的前提。

## Acceptance criteria

- 普通用户：新建（含请求ID和最多3张图片）→ 管理员回复 → 用户收到未读和通知 → 用户回复重新打开 → 用户关闭，全流程可用；超过5个未结工单或超过限流时被拒绝。
- 越权：用户访问他人工单、他人附件、内部备注附件均返回不存在；申诉会话访问普通接口返回未登录；普通会话访问申诉接口返回未登录；申诉会话不能读取非appeal工单。
- 封禁申诉：密码登录、二步验证、OAuth、微信、Telegram、Passkey在账号封禁时都进入申诉页；密码错误时仍返回原提示；开启二步验证的封禁账号必须先通过验证；同时最多1个未关闭申诉；账号解封后申诉会话失效并提示重新登录。
- 附件：SVG、伪装扩展名的HTML、超过5MB、超过3张、无法解析的图片都被拒绝，且不留下文件；数据库写入失败时文件被删除。
- 自动流转：waiting_user满7天转resolved，resolved满7天转closed；未到期的不变；重复执行不会重复写事件；非主节点不运行。
- 数据库：模型测试在SQLite上运行，MySQL与PostgreSQL通过DryRun检查生成的SQL；三张表迁移可重复执行。
- 前端：`bun run build`通过；17种语言补齐新增文案；浏览器实测用户页、管理页和申诉页的主要流程与空状态。
- `go build ./...`、相关`go test`、`make verify-notes`和`git diff --check`通过。

## Risks

- `staff_unread`是全体管理员共享的：任何一位管理员打开工单后，其他管理员的角标也会消失。管理员人数少时可以接受，以后需要时再改成按人记录。
- 附件存放在本地`data/tickets/`。多实例部署时必须共享该目录，否则其他节点读取不到附件；备份需要包含该目录。
- 未结工单数量上限在并发提交时可能被超过一两个，由每10分钟5次的限流兜底，不加用户行锁。
- 申诉入口会让“密码正确但账号已封禁”与“密码错误”在响应上可以区分。只有知道正确密码的人才能看到这个区别，开启二步验证的账号还要先通过验证，所以不构成账号枚举。
- 登录后才被封禁的用户，旧会话在过期前仍能浏览控制台（既有行为）。工单写操作会在数据库中复查状态并拒绝。
- 通知受同类限频影响，短时间内多次回复时，部分通知会被合并或丢弃；用户仍能在站内看到未读。
