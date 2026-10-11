# Agent Note: 定向迁移上游安全与静态资源兼容修复

Status: implemented

## Problem

上游 RC.41/42 的签名、角色、TLS 与请求字段修复对应本地真实缺口。前端按需加载文件仍消耗页面 IP 限额，同一出口下可导致合法页面加载收到 429。本地与上游无共同祖先，且当前失败退款、旧 Semi 前端和支付订单具有独立契约。

## Decision

- 升级 go-epay 至 0.0.5，保留签名边界拒绝及只读验签。套餐支付名称中的 ASCII `&` 替换为全角 `＆`，仅处理网关展示名称；套餐标题和订单金额不变。回调 URL 的歧义输入继续失败，不编码绕过。
- 创建和更新用户在写库前校验已有角色枚举，保留原权限高低规则。
- 各 HTTP transport 克隆 TLS 配置，包括本地 SOCKS/CRS 客户端；保留原证书验证开关和非流式超时。
- 在本地 capabilities 扩展准确匹配的 GPT-6 Sol/Luna 及日期快照；原生 Claude 请求保留任意 safeguards JSON。
- 已存在的非 HTML 构建文件 GET/HEAD 走独立 GS 限流，默认关闭；页面、HTML、目录、未知文件和其他方法仍走 GW。保留 API、预渲染与图片工作台路由。

## Existing decisions

检索 proposed/implemented/rejected 后，[支付多网络笔记](../feature/2026-10-09-epay-crypto-networks.md)与[单网络接入笔记](../feature/2026-10-09-epay-usdt-trc20.md)部分重叠，保持现有 CNY 下单与签名回调规则；[渠道非流式超时笔记](../feature/2026-10-05-channel-nonstream-header-timeout.md)部分重叠，保持客户端缓存与超时语义；K3/Claude 计费笔记与本轮无重叠，维持失败退款及用量规则。未发现静态文件限流或本轮请求字段的其他归属笔记。

## Alternatives considered

- 整体移植上游能快速获得全部功能，但依赖 RelayKit、新前端和认证数据库迁移，并改变已明确的失败退款语义，因此只适配六个可独立验证的行为。
- 提高或关闭页面限流能缓解加载失败，但文件数量和同 IP 用户数不可控，同时削弱页面保护，因此按实际构建文件拆分计数。
- 原样传套餐名称可以保持收银台文案完全一致，但 v0.0.5 必须拒绝 `&`；仅替换展示名称可兼容合法套餐，又不放宽签名校验。

## Testing

- `GOMAXPROCS=4 go test -p 4 ./...` 和 controller/service/middleware/OpenAI/Claude 相关定向 `-race` 回归通过。四个通知/返回入口分别验证 GET/POST 歧义签名拒绝，12 种稳定币类型验证 CNY 订单、关闭后在途通知与重复入账保护，套餐含 `&` 的名称下单及合法返回仍通过。
- 非法角色创建/更新无写库；基础、HTTP/SOCKS 代理、CRS 与模型同步 TLS 指针隔离；OpenAI/Azure 预算、角色、采样及未知别名、Claude 任意 safeguards 深拷贝保真通过。
- 网页与静态限流使用内存及模拟 Redis 分别验证 GS 开/关、GET/HEAD/POST、页面/HTML/目录/缺失文件边界。真实构建后端使用独立临时 SQLite，在页面限额 12/180 秒下连续读取 100 个 JS 均为 200，之后 login/pricing/image-playground 三入口仍为 200；真实 Chromium 系统初始化页加载 63 个静态请求。该实例不含生产数据或真实付款。
- 限流回归为每次运行分配独立测试 IP，避免进程内重复执行继承上轮配额；定向 `-race -count=10` 通过。只隔离测试数据，不重置生产限流器或新增运行时行为。
- Bun 构建及 Chromium 五个公开页预渲染、版本化后端构建通过。没有 MySQL/PostgreSQL 实机、真实供应商请求或链上付款测试；本轮无数据库结构/SQL 变化。发布资产与镜像验证结果单独记录在交付报告，不以本地构建代替发布完成。

## Consequences

安全和请求字段缺口得到定向修复，真实静态文件不再耗尽页面配额。代价是维护本地能力表、每次文件判定打开嵌入文件，以及升级 SDK 后拒绝歧义 URL 和历史歧义回调；部署提示见 docs/EPAY_USDT.md。静态文件访问默认不设应用层次数限制，运营可用 GS 环境变量独立开启。若需要预压缩或新协议映射，单列性能/行为测量再扩展。

管理员 step-up、scoped token、搜索新增收费与大型协议转换不包含在本次发布中；这些改动需要明确认证/收费/退款边界后重访，不能隐含在依赖升级中。镜像发布不代表生产部署。
