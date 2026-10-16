# Agent Note: 桌面端不计费线路探测

Status: implemented

## Problem

桌面端需要在用户授权的跳板 meta-api.vip 和 Cloudflare nan.meta-api.vip 之间选择传输入口。现有 `/api/status` 含平台配置，不能通过固定响应确认未命中缓存，也不能检测流式缓冲；真实模型请求会产生费用。

## Decision

复用 Gin 路由及现有限流工厂，新增 `/api/route-probe` 和 `/api/route-probe/stream`。只接收单个 32 位小写十六进制 nonce，返回 version=1、service=yuanheng-api、原样 nonce；短流附加 sequence=0..2，每段间隔350ms并 flush。普通路径120次/分钟/IP，短流12次/分钟/IP，独立于模型限流。禁止进入鉴权、模型调用、数据库查询和消费记账。

通过独立注册避开 API 分组的 gzip，设置 Cache-Control 与 CDN no-store，短流设置 X-Accel-Buffering=no。客户端仍须拒绝缓存命中和身份不符，代理/CDN强制缓存规则必须在线核实。

## Existing capability review

入口为 router/api-router.go、controller/status.go、middleware/rate-limit.go；无既有同题探测端点。检索活跃笔记后，反代迁移笔记部分相关并继续持有 Host/SNI/TLS 边界；Kimi/Responses 真实请求完整性及上游渠道故障切换均属于模型协议，不改变这些路径。

## Alternatives considered

- 复用 `/api/status`：无需新增接口，但响应较大且没有随机校验、短流间隔证据，不用于持续择路。
- 模型请求测速：能观察模型首字，但有计费且混入上游推理因素，拒绝作为自动探测。
- 静态 Nginx 200：成本小，但不能确认应用可达及流式转发，不能作为服务身份依据。

## Testing

`go test ./controller ./router ./middleware -run 'TestRouteProbe|TestRedisRateLimiter' -count=1` 通过：无认证访问、nonce/重复参数校验、不可缓存、真实 HTTP 分段抵达、取消和独立限流。`go build -o /tmp/newapi-route-probe .` 通过；生产端点尚未部署。

## Consequences

公共探测只证明入口到应用的传输能力，不证明账号、模型权限和服务性能。共享出口可能触发按IP限流，桌面端必须退避；生产部署、两入口缓存及流式实测单独验收。
