# 精确模型端点可用率来源核实（2026-11-17）

用户要求以官方已有数据补充缺失可用率。此次匿名读取OpenRouter自己发布的端点API，两项均HTTP200，未调用模型、未读取凭据。

| 精确模型 | 来源 | 最近24h | 最近30m | 最近5m |
|---|---|---:|---:|---:|
| recursive/flint-v1.0:free | https://openrouter.ai/api/v1/models/recursive/flint-v1.0:free/endpoints | 100 | 100 | 100 |
| cohere/north-mini-code:free | https://openrouter.ai/api/v1/models/cohere/north-mini-code:free/endpoints | 100 | 100 | 100 |

字段为uptime_last_1d、uptime_last_30m、uptime_last_5m，直接数值百分比；100不再乘100。响应模型ID与路径一致，各一个端点。这是平台自报的具体端点可用率，不是模型原作者自营API、厂商总体SLA或本站分组统计。仅能作为独立参考，有时效。窗口由字段名给出；响应没有该窗口的原始样本数或采集结束时刻，因此不生成样本数、不宣称精确测量截止时间。

现有代码只读取latency_last_30m和throughput_last_30m；本轮扩展同一字段读取，仍按实际授权路由、服务商策略与限价选取匹配可调用端点，拒绝没有官方数值的情况。只有status=0不代表100%，不得由状态码推算比例。

最小原始样本保存于[测试数据](../../service/testdata/openrouter_uptime_public.json)，用于解析回归；真实数值不写死在产品。官方说明页 https://openrouter.ai/docs/api/api-reference/endpoints/list-endpoints 本次403，未将其错误页当资料；有效证据来自一方机器可读API本身。

另派发的厂商整体状态页研究因研究代理502失败，没有返回有效结论；未据此声明厂商无数据。因已获得与当前Flint/North需求对应的精确端点数据，本轮不扩展整体状态抓取。无新增定时任务或数据库字段，运行时参考仍有界缓存，非本站历史。

## 完成结果

本地后端已读取24h字段，当前精确模型/分组缺少本站比例时触发已有参考读取；前端将数值标为参考可用率24h。本站实测0不被100参考覆盖，外部参考不生成历史曲线。后端定向、前端13项、浏览器8种状态、手机提示与构建/预渲染均通过。未提交、未推送、未发布，因此不宣称线上已经显示。
