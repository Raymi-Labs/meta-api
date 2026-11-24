# Agent Note: Chat 输出预算的上游拒绝诊断

Status: implemented

## Problem

同一 Chat 模型已有预算别名转换。实际渠道返回 max_tokens 被拒时，预扣记录只有预算值，历史日志无法定位字段出现在网关还是下一跳。

## Decision

复用出站 HTTP 和失败日志的 upstream 快照，在原生 Chat 入站经 Chat 或 Responses 上游收到 HTTP 拒绝时记录预算字段名、出站协议、模型及透传。针对明确的 Chat 字段替换提示提供用户可读建议。

## Alternatives considered

- 入口统一替换字段会越过未知模型、协议桥接和显式透传的约定。
- 无条件删掉上限再重发可能增加费用并改变调用语义。

## Testing

完整 Chat 处理器与模拟 HTTP 上游的 18 个回归覆盖流式及非流式、两种别名与优先级、均省略、显式 0/null、出站参数和模型覆盖、透传、Responses 桥接。测试比对上游收到的字段、值和协议，以及对应的拒绝快照。服务回归核对多次尝试的独立性、公开日志权限和相近错误；前端回归区分无字段与未知、核对公开报告和翻译。

## Consequences

只在共用 HTTP 上游返回 4xx/5xx 时提取字段名；原始输入的元数据在本次请求内复用。出站模型取自实际 JSON，仅接受有界的模型标识。空数组表示没有这些字段，null 表示未取得对应请求体。发生重定向、出站路径不明、独立 SDK、HTTP 200 内的流错误或未收到 HTTP 响应时没有这份快照。历史日志及上游内部的二次转发不可追溯。

[GPT 预算能力](2026-10-12-upstream-security-static-compat.md)和[渠道参数适配](../feature/2026-10-25-channel-reasoning-profiles.md)仍控制原有字段转换、覆盖及透传。本次只补拒绝证据和提示；Kimi 历史预算策略不在本次范围。
