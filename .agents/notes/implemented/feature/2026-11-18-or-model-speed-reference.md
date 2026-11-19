# Agent Note: 非 OR 路由可显示同型号 OR 性能参考

Status: implemented

## Problem

仅在分组绑定 OR 中继时取性能值，导致有公共型号但本站未测的文本模型仍缺数字。

## Existing capabilities and impact

扩展[端点参考](../../implemented/feature/2026-11-01-openrouter-performance-p50.md)；目录、详情、对比共用健康接口和 modelPerformance，不改变上下游路由或计费。

## Decision

按已启用的精确模型和当前用户可见分组，额外读取同型号 OR 市场端点的P50（逐项保留近 30 分钟或 24 小时窗口）。只送已核实的公开 ID，不借用其他用户或分组的中继凭据。复用有界缓存与年龄校验；市场参考不产生本站样本。

## Alternatives considered

直接把全平台统计当作该组实测会伪造来源；借用另一渠道的密钥会跨越凭据边界；均不采用。

## Verification

无 OR 绑定仍能获得同型号参考；两项独立选择，过期、未授权、错误型号都不泄露数据。

## Consequences

市场取数可无密钥读取公共模型页的文本 endpointStats，或使用独立服务端元数据 Key 读官方 API；历史数据不得因缓存命中延长新鲜期，匹配清单为 135 个站内文本型号，不代表所有型号都有数值；OR 缺值、无映射时不伪造样本。普通角色浏览器为隔离数据验收，不等于生产账号验收。

真实公开端点快照见[覆盖核查](../../../../docs/audits/2026-11-18-openrouter-speed-coverage.md)。

公开速度不展示渠道品牌；参考数值用“参考 P50”区分于本站实测，后端仍保留出处。对比页只展示有依据的可用率与用量图；正的请求汇总无需额外有 observed_count 才能展示，不能用成功率代替可用率。
