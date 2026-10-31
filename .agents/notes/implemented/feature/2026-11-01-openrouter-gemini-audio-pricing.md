# Agent Note: OpenRouter Gemini 统一名称、语音计量与售价汇率

Status: implemented

## Problem

渠道 1638 只配置了 8 个带 google/ 前缀的条目，含 batch 特殊入口，未统一到现有官方模型名。旧 OpenRouter 销售价按固定 USD×6.8×5 写入能量价，当前官方对比采用 ECB USD/CNY 6.652882205513785，导致同源单价显示高 2.211%。此外 OpenRouter 语音请求绕过渠道参数覆盖，通用 TTS 估算使用每分钟 1000 Token，与 Gemini 音频每秒 25 Token 不同，不能用于精确结算。

## Decision

用户确认仅 1638 的 Gemini 去前缀，补充当前目录全部具体型号，继续跳过 batch 等特殊入口并暂停未核实原厂价的型号。已存在官方名称复用现有元数据和较低售价，不覆盖其他厂商名称。新增/校正 OpenRouter 价格使用与官方对比相同的公开汇率快照，不通过改变显示汇率掩盖价差；只降不升，不改变充值和全局分组倍率。输入/输出/缓存/图像/音频保持各自单位。

语音修复与生产升级已经用户明确授权。OpenRouter 原生语音接口不执行 sort/only/max_price；用户另明确接受调用前逐端点核价：任何未审核服务商、超过上限、报价缺失或查询失败均拒绝。每次请求核价，不缓存许可结果；保留核价与上游实际收费之间无法消除的时间差，不宣传成上游硬限价。聊天和向量继续通过原有参数覆盖落实上游限价。

OpenRouter TTS 使用 X-Generation-Id 查询同账号 generation 的实际输入/输出 Token，输出归为音频，不按文件 KB 或其他模型的音频系数估算。STT 规范化上游 usage 的输入音频和输出文本，并支持既有文件上传流程。错误/缺 usage 不用假 Token 结算，不对已发出的语音自动重试。协议转换、计量、核价只针对 OpenRouter 语音路径，其他提供商保持原行为。

## Existing notes audit

- [厂商接入](../../implemented/feature/2026-10-31-openrouter-vendor-onboarding.md)：部分取代；本轮明确放开 Gemini，固定 6.8 的售价换算被公开汇率快照替代。
- [原厂限价](../../implemented/feature/2026-11-01-openrouter-price-routing.md)：部分重叠；保留所有其他渠道策略，新增语音端点不支持硬限价时经用户确认的核价门禁。
- [官方价比较](../../implemented/feature/2026-10-30-model-group-official-discount.md)：复用 ECB 公开汇率，保持官方对比真实，不修改参考价展示来制造优惠。

## Alternatives considered

- 继续固定 6.8 且把比较显示也改成 6.8 能隐藏价差，但不符合真实公开汇率比较，选择修正销售配置。
- 在语音 JSON 里塞入 max_price 看似可复用聊天规则，但 OpenRouter 明确不执行，不能当作限价成功。
- 保持语音停用可以避免扩展程序，但用户明确选择补齐计量、接受调用前核价并更新生产。
- 按音频时长或文件大小估算实现简单，但音频 Token、输入指令和格式不同会造成实际扣费不一致，优先使用上游实际 usage。

## Verification

适配器测试覆盖全部候选价格、未知提供商、额外费用、超价、报价失败、重定向、不缓存许可、PCM 限制、映射后的 JSON/文件上传、实际 usage 和缺失 usage 拒绝。完整控制器用模拟 OpenRouter HTTP 服务验证预扣、实际结算与失败退款：7 文本输入 + 79 音频输出按 12 倍计费，实际只扣 955 quota；超价零语音请求，缺 usage/上游失败各一次请求且不重试、不增加消费。SQLite 和 MySQL 8.0.46 均通过；MySQL 使用生产 channels/abilities/options/models 空结构与 utf8mb4_unicode_ci 排序规则，没有复制生产用户数据。

既有 133 型号策略 733 项通过；Gemini 审核候选策略 305 项、暂停策略 29 项通过。相关 relay/service/controller/model/billing/setting 测试与前端构建通过；本机无 Chromium，前端构建的 prerender 跳过，依赖既有后端元信息渲染。Python 价格计划测试覆盖分别保留低价组件、音频倍数、图像输出及缓存独立价格和保留旧按次价格。

生产已通过受保护入口从 v1.11.4 更新至正式版 v1.11.9（发布提交 45f77afbf）；在线备份、两轮 MySQL 预检、健康检查和恢复包封存成功。随后经管理 API 写入暂停配置：29 个映射、111 个价格项、4 个图像表达式与 24 个元数据记录；其余 45 个 OpenRouter 渠道、充值价和全局分组倍率不变，线上 199 项价格组件核对通过。真实 Inception 调用的 5 输入（含 1 缓存）和 15 输出 Token 按新倍率只结算一次 94 quota，消费日志与令牌消耗一致，短期测试令牌已删除。该调用证明新售价实际生效，不证明 Gemini 语音可用。详情见[生产验收](../../../../docs/channel/openrouter-gemini.md#生产验收2026-11-01)。

## Consequences

所有 29 个具体型号保留无前缀配置与映射；10 个 batch 入口跳过。24 个有公开原厂价，其中 3.5 Flash 音频输入在 OpenRouter 高于本次原厂标准口径，不能启用；另 5 个旧 preview 型号报价/身份未核实。不能将旧预览版自动映射成其他正式版。

真实上游验收被外部条件阻止：同一渠道密钥的 4 个语音型号、3.8 Flash、2.5 Flash-Lite、Embedding-001、Nano Banana 2.1 均返回 provider Terms Of Service 403。因此 1638 全渠道暂停，并有无条件拒绝规则，解除账号限制后仍须重新核价和真实验收；程序发布与模型启用分别记录，不能宣传语音已能真实调用。generation 原生 Token 字段的端到端计量尚未得到该账号真实语音响应证明。

官方价和汇率是有日期的快照，不是自动降价任务。销售价格只降不升，已有按次价格不擅自迁移成按 Token 合约。语音核价与收费存在时间差；原生语音无法实现最低价排序，全部候选在原厂价内才放行。图像输出/音频输入等上游费用仍需要完整 usage 验收。程序支持先发布，渠道随后以暂停状态写入映射、价格，保留恢复前验证门禁；不停止服务修改数据库。
