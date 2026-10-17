# Agent Note: Vidu 出站计价字段保持一致

Status: implemented

## Problem

任务入口先按模型和顶层参数计价，原生 Vidu 适配器随后将 metadata 解码进请求体，可以覆盖模型、时长和分辨率。

## Decision

借鉴上游 bfdad293c，在本地原生适配器先合并扩展 metadata，再写入映射后的模型和顶层时长、分辨率。保留参考图 viduq2 规范化及其他扩展字段；不引入 JS 插件，不修改价格和资金结算。

## Alternatives considered

- 迁入上游插件可直接复用其实现，但会引入本地不存在的运行体系。
- 直接拒绝整个 metadata 能阻止覆盖，但也会丢掉 seed、回调等合法扩展；仅将已参与路由/计价的字段设为权威值。

## Testing

`go test -race ./relay/channel/task/vidu ./relay/channel/task/taskcommon -count=1` 与适配器构建通过。回归默认参数、模型映射和参考图出站 JSON；冲突 metadata 不改变模型/时长/分辨率，其他扩展保留。

## Consequences

依赖 metadata 覆盖这三个字段的调用方需改用顶层参数；不证明上游实际计费。活跃笔记检索未发现 Vidu 归属，Kimi 诊断和计数同步笔记不相关。
