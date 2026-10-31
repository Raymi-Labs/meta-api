# Agent Note: 阿里翻译参数在兼容接口中丢失

Status: implemented

## Problem

qwen3-livetranslate-flash 使用同一音频、zh/en 语言参数直连阿里返回翻译及用量，经过生产 v1.5.0 返回 translation_options 无效。GeneralOpenAIRequest 未声明该字段，解码时丢失，阿里适配器无法再发送必填参数。

## Decision

复用现有 Chat Completions、阿里转换、音频输入及 SSE 用量链路；按 DTO 的厂商扩展约定增加带 omitempty 的 json.RawMessage 字段，保留调用者语言选项，不设置全渠道默认语言。没有字段的普通请求保持不变。

## Existing capabilities and note audit

已检查功能地图、DTO、阿里 requestOpenAI2Ali、生产原接口和官方 API 示例。检索 translation_options/livetranslate/翻译：站点语言、模型介绍翻译笔记与此无关；[阿里实时 TTS](../../implemented/feature/2026-10-31-aliyun-realtime-tts.md)只部分重叠于音频渠道，本次仍走 Chat Completions，不扩大其 WebSocket 协议边界。

## Alternatives considered

- 渠道参数覆盖可临时固定中英语言，但会覆盖调用者的其他语种，不能修复通用接口契约。
- 声明厂商扩展字段可沿用现有解析、权限、路由与结算，因此采用此方案；不新增独立转发端点。

## Verification

回归先在旧代码上复现丢失字段，再验证官方样例结构经过 DTO 解码、阿里转换及序列化后保留 translation_options，缺省字段不新增。go test ./dto ./relay/channel/ali ./relay ./service ./controller 及 go build 通过。生产发布后仍须复测翻译结果和上游用量。

## Consequences

语言支持由上游校验。本轮直连验证 zh/en 有效，官方表格的英文全称返回错误，不在网关私自转换。测试仅验证音频输入和文本输出，不能证明视频输入、音频输出或 WebSocket 实时翻译可用。
