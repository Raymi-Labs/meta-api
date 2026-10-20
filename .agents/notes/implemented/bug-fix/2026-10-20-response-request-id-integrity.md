# Agent Note: HTTP 响应请求 ID 与本站日志一致

Status: implemented

## Problem

RequestId 中间件已生成本站 ID，消费/失败日志从 context 取值；IOCopyBytesGracefully、音频、视频代理复制上游响应头时可覆盖 HTTP 的同名 X-Oneapi-Request-Id。2026-10-20 的四条实际消费记录均为同一渠道的非流式 Responses，客户端提供的另一个 ID 不在本站日志中，上游未鉴权响应也返回具有相同节点标记的 ID。原始成功响应头未留存，历史各条映射不能凭 token 数量重新构造。

## Decision

复用 RequestId 中间件，在实际写出 HTTP 头之前固定为本次生成的本站 ID，大小写变体和重复值统一为一个；覆盖 Write、WriteString、WriteHeaderNow、Flush 和无 body 的响应，保留 Gin 的状态、流式及连接接口。响应 body 中供应商生成的 id 不变。

共用 HTTP 出站入口保存最近五个实际收到的上游请求 ID、来源头与渠道 ID，仅进入已有 admin_info。优先上游 X-Oneapi-Request-Id，再沿用现有供应商头选择。列表明确表达收到过的响应，不把重试前的 ID 标成最终响应 ID；不保存其他头、凭据或 body。失败诊断识别同名网关头。

## Related decisions

- [失败诊断](../../implemented/bug-fix/2026-10-17-relay-failure-diagnostics.md)：部分重叠，继续沿用日志权限和最终失败语义。
- [Kimi 实测边界](../../implemented/testing/2026-09-29-kimi-live-acceptance.md)：部分重叠，曾记录关联缺口；没有原始头证据，不把历史缺口直接归因为本缺陷。
- [能力 failover](../../implemented/bug-fix/2026-10-12-upstream-capability-failover.md)：部分重叠，重试仍共享本站 ID，不改变重试/计费预算。
- Claude 入站头透传、采样兼容笔记只约束入站透传和重试触发，未持有响应 ID 契约，不需替换。

## Alternatives considered

- 分别修改四处复制响应头代码，改动容易理解；但遗漏新出口会再次破坏全局 ID 契约，选择在原 ID 所属中间件统一保护。
- 只在 c.Next 返回后恢复 ID，实现短；非流式 body 和 SSE 首帧通常已提交响应头，无法修复线上实际值。
- 把日志 ID 改为上游值，客户端可直接关联；多次尝试、无上游响应和不同供应商会破坏本站请求唯一性，不能采用。
- 只保留最后一次上游 ID，字段少；后续尝试可能没有 HTTP 响应，会错误归属，因此记录有界的实际响应列表。

## Testing

- 真实 HTTP socket 测试覆盖复制头、重复/大小写键、Write/WriteString/WriteHeaderNow/Flush、空响应和 recovery。覆盖音视频复制头使用的 Set/Add 与流式写出原语；不声称已对真实音视频供应商实测。
- SSE 首帧在 handler 结束前到达；ResponseController deadline/Flush 和 WebSocket 升级、文本帧通过。不缓冲 body、不改变状态码和模型响应对象 id。
- 六个 Responses Relay/SQLite 场景、二十个 Chat/Messages 采样兼容场景验证响应头与持久化日志一致、上游 ID 逐响应保存、普通用户脱敏、单次结算及失败退款。无上游 ID 时不造诊断；上限和异常值有服务层回归。
- middleware、service、controller、relay/channel、openai、claude、model 全包回归、定向 race 和后端构建通过。overlay 换回旧中间件时，真实 HTTP 复制头测试按预期失败（收到上游 ID）；新中间件通过。
- Bun 完整前端构建、系统 Chrome 五个公开页预渲染、版本化后端构建及 make verify-notes 通过。没有修改界面交互；线上生效和供应商复测不属于本地通过结论。

## Consequences

ResponseWriter 包装必须覆盖 Gin 的所有提交入口，含无 body 时 engine 直接提交的路径；不提前写头破坏失败状态码和 WebSocket hijack。SDK 不经过共用 HTTP 出站入口时不保证采集上游 ID，但 HTTP 输出保护仍由全局中间件承担。不能恢复历史缺失的上游 ID，且生产生效需另行部署。

上游 ID 诊断最多保留五条，每条 ID 最多 256 字节且排除控制字符；响应列表比单值多占少量日志空间，但避免无响应重试造成归属错误。无数据库迁移、不新增重试或费用；管理员通过现有日志 API 读取，普通用户仍仅使用本站 ID。
