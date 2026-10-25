# 邀请推广

## 使用入口

- 邀请人：钱包管理 → 邀请奖励 → **推广素材与点击统计**。可复制文字链接、推广按钮和横幅卡片的 HTML，粘贴到允许 HTML 的博客或网站。
- 被邀请人：短链 `/i/<邀请码>` 跳转到 `/register?aff=<邀请码>`，展示“满血模型，尽在元衡”、用户提供的品牌角色、深蓝光环背景及 CC Max / GPT Pro 模型卡片；表单采用同场景配色，普通注册页保持原样。角色使用本地透明WebP素材，有限次数CSS动效支持减少动态效果偏好。邮箱和 OAuth 注册继续使用原来的绑定规则。
- 管理员：用户管理 → 用户的邀请关系，查看该邀请人的近30天点击。返佣明细继续由原台账提供。

素材复用当前邀请链接的域名，带固定来源 `src=text|button|banner`。不更换域名，也不额外加载广告脚本或追踪像素。横幅引用同源静态图片 `/images/yuanheng-invite-character.jpg`，使用 CSS 仅展示四视图中的正面形象；不带邀请码或访客参数，设置 `referrerpolicy="no-referrer"`，图片加载不计点击或曝光。图片无法加载时，文字和邀请链接仍可用。该资源须随前端发布后再投放广告。预览 iframe 禁止脚本和弹窗，预览不记点击。修改专属邀请码后，新素材自动更新，已投放 HTML 需要重新复制替换，旧邀请码不继续积累点击；历史数据按邀请人 ID 保留。

## 统计口径与权限

- 统计入口是有效、未禁用、未删除邀请人的短链 GET 请求；直接打开 `/register?aff=...` 不记入短链点击。
- 每 IP 每分钟最多60次短链请求，沿用项目 Redis/内存限流框架；显式 prefetch/prerender 不计。统计是通过限流的访问次数，重复访问和未标记的机器人可能计入，不代表 UV、广告展示次数或注册转化率。来源参数可被访客修改，不用于结算。
- `invite_promotion_clicks` 按邀请人、UTC 日期、固定素材来源四类聚合，不保存访客 IP、User-Agent、浏览器标识或逐次访问记录；既有访问日志和临时限流策略不受此表影响。
- 查询窗口固定为 UTC 今天及此前29天，日期空缺补零。没有上线前的历史点击数据，也不反推历史值。
- `/api/user/invite-promotion/self` 只从登录态取本人 ID；`/api/user/:id/invite-promotion` 需要管理员权限。统计和返佣分别展示，不计算口径不一致的转化率。
- 点击写入有2秒数据库上下文时限，统计存储错误不阻止原注册跳转。限流超限仍返回429。

## 数据库与部署

新表使用三字段复合主键和 GORM 原子 upsert，接入普通与快速迁移入口。无需改动历史用户、邀请关系或返佣表。SQLite 执行与并发测试通过；MySQL/PostgreSQL 检查生成 SQL，不等同于生产数据库实机验收。新增表可随新版启动迁移，回退旧代码时保留该表，不删除数据。

本轮仅修改工作区代码，生产更新仍须使用受保护发布入口；本地构建成功不代表部署完成。

## 验证

```sh
GOMAXPROCS=4 go test -p 2 ./model ./controller ./router ./middleware -run 'TestInvitePromotion|TestUserInsertPersistsInviterAndRewards|TestUserInsertWithTxPersistsInviterForOAuthFlow|TestInviteBinding|TestGetUserIdByAffCode' -count=1 -timeout=120s
cd web
bun run test:invite-promotion
# 在另一个终端启动本地 Vite，测试用合成 API，不调用生产注册/付费接口
bun run dev --host 127.0.0.1 --port 5187 --strictPort
# 测试执行后关闭该 Vite 进程
bun run test:invite-promotion:browser
```

浏览器脚本使用本机 Chrome；可通过 `CHROME_PATH`、`INVITE_TEST_URL` 指定路径和本地地址，浏览器在 finally 中退出。覆盖邀请与普通注册、邮箱提交邀请码、OAuth 显示、桌面/手机/深色、广告代码复制和预览隔离、来源切换、统计错误/重试。

代码片段单测覆盖危险 URL 拒绝、来源参数与文字 HTML 转义。后端覆盖并发不丢计数、UTC 边界、账号隔离、无效/禁用邀请码、改码保留历史、数据库故障仍跳转，以及未登录/非管理员权限拒绝。

品牌角色实现与资源边界见[品牌角色决策](../.agents/notes/implemented/feature/2026-10-22-invite-brand-character.md)。
