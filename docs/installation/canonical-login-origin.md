# 四线路统一到 meta-api.com 登录

四个入口为 `https://meta-api.com`、`https://cn.meta-api.vip`、`https://nan.meta-api.vip`、`https://meta-api.vip`。仅浏览器登录、注册、桌面授权确认统一在主站完成，模型API线路不变；不会把已有各站Cookie、第三方code或state跨域传递。

备用域名上的`/login`、`/register`、`/desktop-login`先跳到主站，主站才创建OAuth state；前端SPA导航与服务端直接访问均覆盖。登录后停留主站，不声称备用域名已同步登录。备用站个人页的绑定按钮进入主站个人页，要求用户确认主站账号后再次操作，不自动绑定另一个已登录账号。

GitHub和自定义Google使用当前标签页跳转，不依赖异步弹窗。Google发起与兑换的回调统一；GitHub在公司域名配置下显式发送相同回调。其他自部署域名/localhost保留原有回调来源，不修改全局API `ServerAddress`。

## 部署者必须核对

1. Meta后台站点地址建议设为 `https://meta-api.com`（无末尾路径），反向代理保留实际浏览器Host、Origin，HTTPS正确。
2. Google Cloud → Google Auth Platform → Clients → 网站当前使用的OAuth客户端，在 **Authorized redirect URIs / 已获授权的重定向URI** 添加精确地址：`https://meta-api.com/oauth/google`。不是只加根域名，也不是仅加JavaScript来源；无末尾斜杠，必须对应网站实际client_id。不要泄露Client Secret。
3. GitHub OAuth App的Authorization callback URL核对为 `https://meta-api.com/oauth/github`。
4. 使用现有受保护发布入口更新Meta后端与网页，先备份/预检，不先停旧服务。本任务不操作生产或Google/GitHub控制台。
5. 关闭旧授权页，从客户端或网站重新发起；旧state来自备用域名，不能通过转发旧回调补救。

代码统一回调不等于Google后台已登记。若仍出现redirect_uri_mismatch，以Google错误详情中的实际client_id/redirect_uri对照登记值；换Chrome或Safari不能修复登记不一致。

## 验收

- 从三个备用站的登录/注册页进入主站，页面不循环；合法邀请码/桌面request_id保留，外链next、重复参数和code/token/state不跨站携带。
- 从每条线路打开API、模型请求或已有控制台，不应被本次中间件重定向。API客户端可继续使用原地址。
- 主站分别完成GitHub/Google真实授权，确认账号身份；桌面v0.1.130从vip发起的授权页跳com后可批准，并由原API兑换。
- 账户绑定不因跳域而自动执行，桌面审批仅主站Origin允许，已登录错误账号必须退出重来。

已做8项离线前端导航/授权参数测试，以及Go白名单、URL污染/重定向边界、模拟Google令牌兑换、桌面Origin授权回归。未登录真实第三方账号或更改控制台设置，线上验收需部署和回调配置完成后确认。
