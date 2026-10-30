# Agent Note: 渠道非流式响应头等待覆盖

Status: implemented

## Problem

Grok 渠道非流式调用在默认60秒响应头等待处失败，而43/57秒完成的请求成功。全局延长会让无关渠道的故障等待变长，按模型/渠道ID硬编码又不能审计或撤回。

## Decision

复用管理员渠道 setting JSON，新增 nonstream_response_header_timeout_sec，0/缺省继承，1至600秒覆盖；拒绝负数、超上限和非整数。仅公共 doRequest 的非流式 Chat/Completions/Responses 文本请求生效，图片（含Responses图片工具）、流式和其他任务沿用原客户端。保留总时限、重定向策略、TLS与代理配置，不改变重试、收费或路由。

基于已有客户端克隆 transport，按基础客户端指针与超时缓存，最多32个LRU条目；不修改共享transport，不逐请求创建空连接池。淘汰/配置重置关闭空闲连接，不中断在途请求。取消沿用请求context，SOCKS拨号需使用context接口。日志管理员字段记录所用响应头等待和总时限。

基础客户端的 TLS 配置独立性由[定向安全修复](../bug-fix/2026-10-12-upstream-security-static-compat.md)补齐，本篇连接池与超时语义保持不变。

## Alternatives considered

- 全局60改120可立即缓解，但影响所有渠道且不能隔离，否决。
- 渠道1417硬编码最少代码，但绑定生产数据且不可配置，否决。
- 每请求克隆客户端/transport可隔离，但损失连接复用；选用有界缓存避免无限代理/配置组合增长。

## Existing notes audit

检索活跃笔记的超时/HTTP客户端关键词；Kimi流完整性笔记部分相关但不持有HTTP响应头配置，本次保留其取消与失败不重试边界，不改Kimi实现。

## Verification

`go test ./dto ./service ./relay/channel ./controller -count=1`通过。`go test -race ./service ./relay/channel -run 'Test(HeaderOverride|ChannelHeaderOverride|ChannelNonStream)' -count=1`通过：短默认失败、覆盖成功、慢响应体继续读取、取消和总时限、代理/重定向、SOCKS握手取消、有界并发缓存、重置不取消在途请求。配置校验覆盖0/1/120/600及负数、601、非整数、溢出。消费/失败日志均有管理员诊断；流式、图片工具及非文本模式不使用覆盖。

真实编辑组件在浏览器模拟API下验证无修改120秒提交、改180再打开、清空恢复0，原分组和未知setting字段保留。无生产请求。渠道表单工具6项测试通过。Bun前端构建通过，有既有分块大小警告；预渲染脚本因默认Chromium路径不存在跳过，独立浏览器测试使用本机Chrome完成。没有MySQL/PostgreSQL实机测试，也没有线上Grok收费试调用。

## Consequences

延长只提高可等待时间、不提高上游速度，并发名额占用增加；客户端/Nginx/CDN及总时限仍可能更短。只提供配置能力，线上1417设120须部署后另行操作；不自动写生产配置。验证以本地模拟HTTP为主，不发收费请求。

## Scoped extension

[Video submission waits](../bug-fix/2026-10-30-video-channel-wait.md) extend this setting to VideoSubmit. Background polling, streaming and image operations retain their original clients. The original text-only acceptance results above describe the initial implementation.
