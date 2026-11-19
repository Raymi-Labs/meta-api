# 文本模型首字与速度覆盖核查（2026-11-18）

纽约时间约 13:06–13:09。对 [本站目录](https://meta-api.com/api/pricing)、[站级最近 24 小时汇总](https://meta-api.com/api/perf-metrics/summary?hours=24)和 OR 的公开模型页按精确 ID 逐项合并。现有广场规则有 168 个适用项目；135 个有核实映射，这一轮 OR 对 123 个给出可用的首字与输出速度两项 P50。本站汇总保守地只统计有正数的字段（两项均正值有 47 款，不代表 0 就一定没有测量）。两源合并，有 136 个能确认两项均有数字，32 个仍未确认。这不是线上所有分组或所有用户的覆盖率，未计受权 OR 路由和站内历史回退。

首字与吞吐可能分别来自近半小时和近 24 小时，即使是同一个端点也必须逐项标窗口。未以受限网页或无法获取字段断言 OR 没有数据，未发送收费请求。已验证 North Mini Code 免费版 680.5ms / 124 tok/s（一次公开端点快照，不是站内实测）。

## 待处理（不拿其他型号代填）

- 有精确映射，此轮取数未补齐（10）：`gemini-2.5-flash`、`gemini-2.5-flash-image`、`gemini-2.5-pro`、`gemini-3-pro-image`、`gemini-3-pro-image-preview`、`gemini-3.1-flash-image`、`gemini-3.1-flash-lite-image`、`gemini-nano-banana-2.1`、`nemotron-3.5-lightning:free`、`pareto-26.10`

- 无已核实的精确映射（22）：`kimi-k2.7-code-highspeed`、`lfm-2.5-2.6b:free`、`mercury-2`、`mercury-2.5`、`mercury-3`、`qwen-audio-3.1-asr-flash`、`qwen-audio-3.1-asr-flash-message`、`qwen-audio-3.1-realtime-flash`、`qwen-mt-flash`、`qwen-mt-lite`、`qwen-mt-plus`、`qwen-turbo`、`qwen-vl-max`、`qwen3-235b-a22b`、`qwen3-coder-plus`、`qwen3-max`、`qwen3-vl-plus`、`qwen3.5-ocr`、`qwen3.8-livetranslate-flash-realtime`、`qwen3.8-max`、`xing4.0-29b-a4b:free`、`z-image-turbo`

复核 OR：`OR_MARKET_LIVE_AUDIT=1 go test ./service -run '^TestLiveOpenRouterMarketCoverage$' -count=1 -v`。默认不运行网络核查，手动启用时并发 3。覆盖随端点状态、频率及观测时间变化。
