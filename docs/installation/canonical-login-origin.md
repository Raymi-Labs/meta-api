# 四线路统一到 meta-api.com 登录

四个入口为 `https://meta-api.com`、`https://cn.meta-api.vip`、`https://nan.meta-api.vip`、`https://meta-api.vip`。仅浏览器登录、注册、桌面授权确认统一在主站完成，模型API线路不变；不会把已有各站Cookie、第三方code或state跨域传递。

四域名上的`/login`、`/register`、`/desktop-login`由服务端正常提供HTML，不再按Host发出域名重定向。浏览器App入口读取真实`window.location.origin`，备用域名先跳到主站并在跳转前阻止登录/注册/桌面授权页面挂载，主站才创建OAuth state。直接访问和SPA导航复用这一入口。登录后停留主站，不声称备用域名已同步登录。备用站个人页的绑定按钮进入主站个人页，要求用户确认主站账号后再次操作，不自动绑定另一个已登录账号。

GitHub和自定义Google使用当前标签页跳转，不依赖异步弹窗。Google发起与兑换的回调统一；GitHub在公司域名配置下显式发送相同回调。其他自部署域名/localhost保留原有回调来源，不修改全局API `ServerAddress`。

## 部署者必须核对

1. Meta后台站点地址建议设为 `https://meta-api.com`（无末尾路径），反向代理仍应保留实际浏览器Host、Origin和正确HTTPS信息；应用的登录页导航不再依赖后端Host。仅增加X-Forwarded-Proto或修改站点地址不是旧版本自重定向的修复。
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

## 主域名登录页自重定向与修复边界

旧实现按后端`Request.Host`将备用域名导航到主站。若代理把浏览器的`meta-api.com`改写成`meta-api.vip`、`cn.meta-api.vip`或`nan.meta-api.vip`，后端每次都误判为备用站，造成Location指向当前URL的无限302。修复前的本地真实HTTP代理测试复现了此条件；线上公开响应与其吻合，但未读取生产代理配置，不据此认定具体哪层代理改写了Host。

当前实现删除服务端重复导航，复用现有浏览器origin判断；不改为信任`X-Forwarded-Host`，也不引入跳转计数或可伪造的放行参数。登录和注册HTML使用`Cache-Control: no-store`与`Referrer-Policy: no-referrer`；桌面入口继续禁止缓存和嵌入。浏览器跳转仅保留已校验的next、邀请码或桌面request_id，不转发code/state/token。

代价是备用入口必须先成功加载网页脚本；禁用JS或脚本加载失败时不能完成导航和登录，原有交互表单本身也依赖JS。主站无法访问时不自动换域完成认证。OAuth提供商仍需登记主站回调，真实授权验收不能由本地测试代替。

部署后可在服务器上使用实际后端监听端口做只读对照（3000是示例，127.0.0.1指服务器）：

```sh
curl -sS -I --connect-timeout 3 --max-time 5 -H 'Host: meta-api.com' http://127.0.0.1:3000/login
curl -sS -I --connect-timeout 3 --max-time 5 -H 'Host: meta-api.vip' http://127.0.0.1:3000/login
curl -sS -I --connect-timeout 3 --max-time 5 -H 'Host: meta-api.vip' -H 'X-Forwarded-Proto: https' http://127.0.0.1:3000/login
```

修复后的应用三条均应返回200且不带Location；备用域名的统一登录由浏览器执行。外部URL若仍出现自重定向，继续沿CDN/网关/Nginx定位独立的重定向规则或未更新副本。应用不能消除代理自己发出的循环。HTTPS上游的TLS SNI与HTTP Host分别核对，不关闭证书验证。部署仍走受保护入口，本地测试不代表生产恢复。
