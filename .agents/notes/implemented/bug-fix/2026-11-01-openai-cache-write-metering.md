# Agent Note: OpenAI 默认缓存写入计量

Status: implemented

## Problem

OpenRouter OpenAI 企业级接入验收中，真实 GPT-6 Luna 请求报告 6022 输入、6019 缓存写入、5 输出 Token。现有 InputTokenDetails 丢弃标准 cache_write_tokens，普通与阶梯结算已有缓存写入价格却收到零用量。用户明确要求补齐计量、验证后更新生产。

## Decision

复用现有 CachedCreationTokens、inclusive 输入语义、普通倍率和阶梯 cc 结算。解析标准嵌套 cache_write_tokens，并保留独立的可空公开字段证据；只有上游明确报告的字段才对客户端输出，不将 Claude 内部创建量伪装成 OpenAI 原生字段。深拷贝和多段聚合同时保留它，流式终态快照不重复相加。补齐 Responses 的 prompt_cache_options 和 Chat 转 Responses 传递，确保限价规则能看到真实缓存参数。

当前 1640 仅审核默认 30m 缓存，新请求参数与内容块内显式 12h 缓存均拒绝；不把 Claude 的 cc1h 变量借作 12h 计价，也不扩大为所有缓存时长功能。已有价格、其他厂商原有内部计量及历史账单保持。上线使用受保护 newapi-update，配置先向旧程序兼容，程序升级后验证真实写入、读取、JSON/SSE 和余额。

## Existing notes audit

- [跨协议输入缓存语义](../../implemented/bug-fix/2026-10-24-cross-protocol-cache-semantics.md)：沿用 prompt 包含缓存的契约，先扣独立分项再计费，不重复增加上下文长度。
- [Kimi 用量字段](../../implemented/bug-fix/2026-09-29-kimi-usage-field-preservation.md)：顶层 cache_write_tokens 不误当读取量；本次只解析标准嵌套写入字段。
- [官方价同步](../../implemented/feature/2026-10-31-official-pricing-sync.md)：30m/12h 参考价展示不等同运行时计量；本次不修改来源和展示档位。
- [OpenRouter OpenAI 接入](../process/2026-11-01-openrouter-openai-enterprise.md)：本次真实验收发现的必要程序补齐，仍仅聊天接口。

## Alternatives considered

- 只保存写入价格最省改动，但真实上游字段被丢弃，无法实际按该价结算。
- 将内部 CachedCreationTokens 直接改成公开 JSON 字段更短，却会把其他协议转换得到的内部量无条件输出，破坏已有边界；保留上游报告证据。
- 暂时关闭新一代模型缓存可以完成配置，但用户已选择修复程序并上线。
- 支持 12h 需独立用量、表达式变量与显示契约；现有渠道明确未审核该时长，继续拒绝，不混用 Claude 1h。

## Verification

真实上游 6022 输入、6019 写入 Token 的样本解析通过。标准嵌套字段缺失、null、明确零、非法负数/小数/字符串/溢出、深拷贝、公开字段证据、独立请求相加与同次流式快照不重复均有回归。Responses 聚合保持写入用量，Chat 转 Responses 保留缓存参数。

实际控制器在 Chat/Responses × JSON/SSE × 普通/阶梯 8 个组合中验证输入 1000、读取 800、写入 100、输出 20：准确扣 325 quota，钱包、令牌和单条消费记录一致。SQLite 和导入生产 10 张原始表结构的隔离 MySQL 8.0.46 / utf8mb4_unicode_ci 均通过；没有生产数据拷贝。初始化测试容器首次导入遇到启动就绪窗口，等待后重建隔离库并用准确生产排序规则完整重跑成功。

相关 dto、service/openaicompat、relay/channel/openai、controller、service、relay/common、billingexpr 全包回归通过；专项 -race 通过。前端 Bun 生产构建及 Go 应用构建通过，浏览器预渲染因本机没有 /usr/bin/chromium 跳过，后端既有元信息渲染继续兜底。生产更新和真实缓存扣费验收须在 v1.11.13 镜像发布后单独完成。

## Consequences

其他 OpenAI 兼容服务若在标准嵌套位置报告写入量，会按现有已配置缓存创建比例结算；不接受顶层字段猜测。多模态缓存交集、完整长上下文和 12h 缓存不在此次验收范围。无 schema 变更，不重算历史账单。
