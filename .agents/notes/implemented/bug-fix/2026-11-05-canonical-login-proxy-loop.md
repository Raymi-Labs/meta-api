# Agent Note: 登录导航使用浏览器来源避免代理 Host 自重定向

Status: implemented

## Problem

主域名登录和带邀请码注册在线返回指向自身的302；本地实际HTTP代理证明，新增服务端统一登录中间件在代理把Host改成备用域名时产生相同循环。服务端无法从已丢失的Host区分主站与备用站访问者，单独补协议头无效。用户要求按第一性原理和对抗性审查优化。

## Decision

删除服务端基于Host的登录页重定向及其孤立辅助函数，复用已有App入口的浏览器origin判定、跳转前表单阻断和OAuth发起前主站导航。保留服务端OAuth兑换与桌面批准的固定主域名规则。登录/注册HTML明确no-store及no-referrer，桌面入口继续防嵌入。HTTP提供页面，浏览器决定是否导航，不接受转发头、查询参数或后台ServerAddress作为浏览器来源。没有JS时不声称能登录；本产品表单本身依赖JS。

## Existing notes audit

[四线路登录](../../implemented/bug-fix/2026-11-05-canonical-login-origin.md)部分取代：仅撤销服务端Host跳转，保留OAuth、邀请码过滤和浏览器导航决定与旧理由。桌面交接笔记部分相关，保持PKCE、session、Origin、CSRF和原子消费不变。未发现其他归属笔记。既有本地代理诊断转为完整WebRouter回归，原失败条件保存在旧笔记。

## Alternatives considered

- 只修代理Host：能解决已知错配，但需要确认每层生产配置，仍让两套导航依赖不同来源；应继续核对代理，应用无需据此跳转。
- 信任X-Forwarded-Host：可兼容某些代理，但未定义可信代理链和覆盖规则，攻击者头部或多层代理仍可误导，不采用。
- 加跳转次数或标记参数：可截断循环，但引入可伪造状态、参数污染及登录停在错误域名的边界；已有浏览器origin足够，不新增协议。
- 复用现有浏览器导航：不猜测丢失的来源，保持单一决策点；接受备用入口先加载JS的延迟。

## Verification

修复前完整WebRouter代理验收首先失败：改写为三个备用Host均返回302；仅补协议头或转发Host仍失败。删除重复导航后，8组代理配置×6域名×14条GET/HEAD路径组合共672次真实回环HTTP请求全部通过，覆盖带邀请码注册、桌面授权、价格/API及回调页面，登录no-store/no-referrer和桌面防嵌入头均保留。所有客户端禁自动跳转，有3秒超时，服务和连接由测试cleanup关闭。原单层测试更新为实际WebRouter与已注册API路由，不以空处理器代替入口。

Go router/common/controller/oauth定向回归通过，覆盖主站回调规范化、Google模拟兑换、桌面Origin/CSRF/会话/单次批准兑换边界；router登录测试的race检查通过。前端16项Node导航/OAuth/协议同意检查及auth-redirect脚本通过，6项真实React鉴权路由组件检查通过。增加三备用入口到主站单次收敛、主站不自跳转、危险next和伪造origin/host/跳转标记无效的测试。复用未改动的App入口：别名导航时在Routes/表单挂载前返回Loading。未执行真实Google/GitHub授权或完整浏览器登录，也未部署生产。

## Consequences

备用入口需要成功加载JS；不支持无JS登录，不新增外域自动降级或Cookie跨域同步。外部代理若自身发出循环重定向仍须修改配置；本地修复和测试不表示生产已恢复。不执行生产更新、不修改OAuth后台或站点配置。

验证与本次修改范围不涉及数据库结构、资金、API路由或已有GIF改动。功能地图与部署文档同步；项目笔记校验与差异检查通过，结束前核对本轮测试进程为零。
