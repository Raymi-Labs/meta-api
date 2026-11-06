# Agent Note: 元衡四线路统一登录主域名

Status: implemented

## Problem

四个元衡HTTPS入口共用提供商配置，但前端按当前origin创建redirect_uri，后台按ServerAddress兑换，Google报redirect_uri_mismatch。OAuth state Cookie属于创建时的域名，只改回调字符串仍然失败。GitHub/Google异步获取state后window.open还可能被Safari拦截。

## Decision

服务端Host跳转部分由[浏览器来源导航修复](2026-11-05-canonical-login-proxy-loop.md)取代；下文保留原选择理由和验证边界。浏览器导航、OAuth回调及桌面批准规则继续沿用。

用户指定meta-api.com为主站。仅将四个精确元衡域名上的浏览器登录/注册/桌面授权页面统一至https://meta-api.com；发起前导航到主站，在主站创建state、完成回调/兑换，网页登录后停留主站。不把Cookie、授权code、state或token传给备用域名，不承诺四站自动共享登录；四条API/模型线路不改。

前端SPA和后端GET/HEAD入口同时覆盖，next/邀请码只保留校验后的非凭据字段；主站已有登录不自动绑定，备用域名的个人页绑定操作先回主站个人页重新确认。提供商兑换使用规范登录origin（仅四域名映射，其他自部署/localhost沿用原配置）。桌面批准Origin也使用同一规则，旧客户端API仍可走vip、网页转com。GitHub/Google使用当前页跳转，保留state/next。

## Alternatives considered

- Google登记四个回调：能解决授权第一关，但后端兑换域名与浏览器state Cookie仍可能不同，不作为统一主站方案。
- 跨域登录桥：可保持原站登录，但新增敏感授权协议和复杂账号边界，本轮保持登录后在主站，不隐式同步Cookie。
- 全站及API重定向：规则简单，但影响API客户端与四线路用途，拒绝。

## Verification

四域名回调与兑换一致；备用登录/桌面授权跳转在state生成前；恶意host、外链next、重复参数、code/token/state均不携带；API/SSE/已有会话不重定向；后端收到主站 Host 时不循环；localhost和其他部署保持原行为。模拟验证与Google真实授权分开记录。

本地8项Node导航/授权参数测试通过，直接运行现有api.js授权函数并断言没有window.open、备用域名不创建state、主站Google/GitHub回调参数一致、state作用域next保留。Go common/controller/router/oauth定向测试通过，httptest捕获四配置来源的Google真实POST表单、模拟旧客户端跨域授权Origin与单次兑换。6项协议同意回归、18项语言资源检查、auth redirect回归及Vite构建通过。只读核对四域名均为main-ae7a764且同一Google公开client_id，无真实授权/生产配置修改。

## Consequences

Google Cloud须登记主站完整回调，GitHub应用须配置相应回调，不能仅凭代码宣称修复线上OAuth。主站不可达时登录不可用，不能静默切换登录域名。旧state需重新发起。生产部署由用户完成，不操作第三方账号或服务器。

跨域next只保留受控路径，console页面查询/fragment不复制，避免夹带未知凭据；代价是部分页面筛选条件需用户重选。域名列表是用户明确给出的四个精确HTTPS origin，不自动扩大到任意子域名。后续新增线路需同步白名单与测试。关联[桌面浏览器授权](../feature/2026-11-05-desktop-browser-login.md)的审批Origin由规范化规则替代直接ServerAddress解析，其余proof/CSRF/原子消费保持。


## 修复前本地代理链路诊断

初次诊断使用真实回环HTTP代理连接原Gin登录中间件与页面路由，所有探测有超时和最多三次跳转，连接与服务由测试清理。当轮仅做诊断；后续`router/canonical_login_proxy_test.go`已转为修复后的完整WebRouter验收，见关联修复笔记。

结果：固定上游 Host 为 vip/cn/nan 时主域名登录、带邀请码注册、桌面授权均可出现自重定向；价格页与 API 正常。关闭新增导航中间件的对照组正常；代理保留真实 Host 时主站正常，三个备用域名仍正确指向主站并保留邀请码。仅补 X-Forwarded-Proto 或 X-Forwarded-Host 无效，主站 Host 不受伪造的备用域名转发头影响。现有单层中间件测试与8项前端测试不能替代代理链路验收。

线上只读响应已观察到主域名登录及带邀请码注册自跳转、pricing 200、v1.21.0 响应头；这与本地触发条件吻合，但未读取生产代理 Host 或配置，不将模拟条件表述为已确认的生产根因。优先核对代理并保留真实 Host；不盲改后台 ServerAddress、强信转发头或取消整个统一登录功能。详细只读对照见[部署排查](../../../../docs/installation/canonical-login-origin.md)。
