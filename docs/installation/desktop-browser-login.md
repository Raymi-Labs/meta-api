# 桌面 GitHub / Google 登录部署交付

本改动只新增元衡账号的桌面授权交接，不改第三方OAuth客户端ID/密钥，不复制浏览器Cookie，不改模型渠道、余额或计费。生产部署由维护者执行；提交/构建通过不等于已经部署。

## 部署内容

- 后端与网页必须一起更新：新增`desktop_login_grants`表及`/desktop-login`确认页。表通过既有普通/快速GORM迁移创建，字段为请求ID摘要、S256 challenge、提供商、显示码、状态、用户ID、过期时间；无明文verifier、Cookie或第三方token。
- 复用现有GitHub提供商与slug为`google`的自定义OAuth。`ServerAddress`必须是用户实际访问的规范HTTPS站点，批准接口严格核对Origin，反向代理不得篡改浏览器Origin。
- 保留现有session配置，多副本使用同一数据库及兼容的session签名设置。无需开放本地回调端口，也不用注册新的第三方回调URL。
- 发布使用现有受保护的`newapi-update`入口；先备份和预检，不能先停旧服务。部署前在与生产相同引擎/字符集的环境检查迁移和现有表兼容。

## 接口协议 v1

| 接口 | 作用 |
| --- | --- |
| GET `/api/desktop-auth/capabilities` | 只读，返回protocol_version=1和当前启用的github/google |
| POST `/api/desktop-auth/start` | 原生提交provider和S256 challenge，取得随机request_id、核对码及300秒期限 |
| GET `/api/desktop-auth/info?request_id=…` | 浏览器读取核对码/当前账号，生成绑定此session和请求的CSRF |
| POST `/api/desktop-auth/approve` | session + 精确Origin + CSRF + 页面user_id + 核对码；明确批准或拒绝 |
| POST `/api/desktop-auth/exchange` | 原生用request_id和verifier查询；批准后原子消费并签发独立session |

批准前必须勾选“本人发起且账号/码正确”。设备授权不是第三方OAuth的新实现：浏览器使用已有网站登录，登录后的next跳转回确认页。网站若已登录，直接明确确认该账号；这不保证使用的是客户端按钮所选的第三方身份，界面展示的元衡账号才是授权对象。

## 上线检查（由部署者完成）

1. `GET /api/desktop-auth/capabilities`应为200 JSON、`Cache-Control: no-store`，列表与后台启用设置一致。
2. `/desktop-login`响应不缓存，`Referrer-Policy: no-referrer`、防iframe策略存在；非法/过期request_id不能批准。
3. 分别用GitHub、Google在系统浏览器完成一次真实登录；核对码、账号一致后批准，客户端出现正确账号。检查网络中Cookie只在响应头/后续认证请求，不进入URL。
4. 覆盖拒绝、取消、五分钟超时、重复兑换、浏览器切号、两个客户端并行、服务器多实例、用户禁用。
5. 确认原密码、注册、2FA和网站OAuth登录未受影响。不使用他人账号或生产用户凭据作自动测试。

客户端可先安装，但Meta未部署时会提示授权接口未开放，密码入口仍可用。没有静默降级为WebView或长期令牌链接。兑换响应丢失、目录同步失败等终止后重新发起授权，不自动重复消费。

## 验证与限制

本地SQLite原子竞争及控制器CSRF/Origin/账号/重放测试通过；CI运行42005572249实际覆盖MySQL5.7（utf8mb4_unicode_ci）/PostgreSQL9.6并通过。桌面原生取消、过期、替换授权、已登录账号和一次持久化均有回归；完整真实第三方登录由上述上线步骤确认。网页新增文案有简体/繁体中文及英语，其他站点语言暂使用英语回退。

回退旧服务后，新登录入口不可用，旧登录不受这张授权表影响。已签发的桌面会话沿用原session生命周期；未消费授权最长五分钟失效，表不需要在回退时删除。请勿把数据库授权记录、浏览器Cookie、verifier或日志中的真实账号公开。
