# Agent Note: Claude 拒答换渠道重试

Status: implemented

**日期**: 2026-11-22  
**类型**: 功能增强  
**影响范围**: Claude 渠道、重试逻辑、流式响应

## 问题

Claude 可在 HTTP 200 或正常结束的事件流中报告拒答，客户端可能收不到可用的文档。

## 决策

当 Claude 模型返回拒答（`stop_reason=refusal`）时，自动在同一模型的其他可用渠道上重试，而不是直接返回拒答给用户。这是一个默认启用的功能，无需任何配置开关。

## 核心行为

### 1. 重试触发条件

拒答重试会在以下**全部**条件满足时触发：

- Claude 响应包含 `stop_reason=refusal`（非流式或流式识别）
- `RetryTimes >= 1`（全局重试开关已开启）
- 当前不是用户指定的特定渠道请求
- 渠道亲和设置允许失败后重试
- 请求可重放（非透传、无参数覆盖或可缓冲重放）
- 存在同模型的其他可用渠道候选

### 2. 流式暂存机制

**目的**: 在识别拒答前不向客户端输出任何内容，确保可以无痕切换到其他渠道。

**实现**:
- 启动 30 秒暂存窗口
- 可暂存帧类型：
  - `message_start`（消息开始）
  - `ping`（心跳）
  - `content_block_stop`（内容块结束）
  - 空 delta（无实际内容变化）
  - thinking signature（思考标记）
- 遇到实际内容帧（文本、图像等）立即释放所有暂存帧并正常输出
- 识别拒答时：
  - 若在暂存窗口内且允许重试 → 丢弃所有暂存帧，换渠道重试
  - 若已超时或不允许重试 → 释放暂存帧，正常返回拒答

**交付安全判断**: 利用 keepalive 包装器的 evidence（`ping=true, unsafe=false`）确认尚未向客户端输出有效内容。

### 3. 重试预算

- 拒答重试使用**独立计数器** `refusalRetries`，不占用主重试预算
- 但仍受全局 `RetryTimes` 上限约束（不能无限重试）
- 每次拒答重试调用 `ResetRetryNextTry()`，使本轮不计入主重试次数
- 设置 `safeFailureRetry = true`，确保总尝试次数不超过 `RetryTimes + 1`

### 4. 错误分类与渠道健康

- 拒答被分类为 `FailureUserRequest`（用户错误），**不惩罚渠道健康评分**
- 路由统计和组健康观测都记为 `user_error`
- 原因：拒答通常由用户输入内容触发，非渠道能力问题

### 5. 日志记录

**成功重试后的消费日志**:
- `channelId` 指向最终成功的渠道
- `admin_info.attempts[]` 包含失败尝试记录：
  ```json
  {
    "retry_kind": "claude_refusal",
    "error_code": "claude_refusal",
    "channel": {...},
    "claude_message_id": "msg_xxx",
    "claude_refusal_category": "policy_violation"
  }
  ```
- 顶层不含 `claude_message_id` 或 `reject_reason`（这些只记录在失败尝试中）

**全部渠道都拒答**:
- 返回最后一个渠道的拒答响应
- 消费日志指向最后尝试的渠道
- `admin_info.attempts[]` 包含所有失败尝试（除了最后一次）

## 实现细节

### 预授权逻辑

每轮 relay 调用前，`claudeRefusalRetryAllowed()` 检查 6 个条件，结果存入 `constant.ContextKeyClaudeRefusalRetryAllowed`：

1. RelayFormat 是 Claude 或 OpenAI
2. 重试预算和已用渠道数在合理范围内
3. 不是特定渠道请求
4. 渠道亲和设置允许重试
5. 请求可重放
6. 存在备用渠道候选

### 拒答检测

**非流式**: `observeClaudeRefusal()` 检查响应中的 `stop_reason` 和 `stop_details.category`

**流式**: 
1. 启动暂存机制（若允许重试）
2. 每帧判断是否可暂存
3. 遇到 `message_delta` 包含 `stop_reason=refusal` 时触发检测
4. 根据暂存状态和预授权决定是否重试

### 重试循环集成

在 `controller/relay.go` 的主循环中，VM 容量重试分支之后、错误规范化之前插入拒答分支：

```go
if newAPIError.GetErrorCode() == types.ErrorCodeClaudeRefusal {
    service.AppendClaudeRefusalAttempt(...)
    common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "")
    relayInfo.ResetFirstResponseTime()
    retryParam.ExcludeChannels = appendUniqueInt(retryParam.ExcludeChannels, channel.Id)
    refusalRetries++
    retryParam.ResetRetryNextTry()
    safeFailureRetry = true
    lastRelayError = newAPIError
    logger.LogInfo(...)
    continue
}
```

### 候选探测

使用 `service.HasAlternateChannelCandidate()` 调用只读的 `CacheGetSatisfiedChannelCandidates()`，确保探测不推进分组状态。

### 亲和粘性

`ResolveAffinityChannelForRelay()` 会检查 `ExcludeChannels`，避免将已拒答的渠道再次选为亲和渠道。

## 适用范围

### 支持的协议

- Claude Messages API（`/v1/messages`，原生格式）
- OpenAI Chat Completions（`/v1/chat/completions`，转换到 Claude）

### 支持的渠道类型

- `ChannelTypeAnthropic`（直连 Claude）
- `ChannelTypeAWS`（AWS Bedrock Claude）

### 流式与非流式

- ✅ 非流式：直接检测响应中的拒答
- ✅ 流式：通过暂存机制支持无痕重试

## 不重试的场景

1. **全局重试关闭**: `RetryTimes = 0`
2. **用户指定渠道**: 通过 API 或管理面板指定特定渠道 ID
3. **渠道亲和限制**: `ShouldSkipRetryAfterChannelAffinityFailure()` 返回 true
4. **请求不可重放**: 
   - 渠道开启了透传模式
   - 使用了参数覆盖
   - 请求体过大无法缓冲
5. **无可用候选**: 所有同模型渠道已尝试或不可用
6. **流式已交付内容**: 暂存超时（30秒）或已输出实际内容

## 备选方案

此前的[令牌级官方备选模型](2026-11-21-token-claude-fallback.md)会改变实际使用的模型。需求方选择保留请求型号。

## 影响

### 额外开销

- **内存**: 流式暂存最多占用约 10-50 KB（取决于 message_start 大小）
- **延迟**: 流式首帧延迟增加约 5-20 ms（暂存判断开销）
- **计算**: 每次拒答重试需要额外的上游调用

### 成本优化

- 只为成功的请求计费（失败尝试不扣用户额度）
- 拒答重试计入渠道使用统计，但不影响健康评分
- 30 秒暂存超时防止长时间占用连接

### 预期收益

- **用户体验**: 减少因单一渠道策略差异导致的拒答
- **可用性**: 充分利用多渠道冗余，提高服务稳定性
- **透明性**: 用户看到成功响应，管理员可在日志中审计重试行为

## 测试覆盖

### 集成测试

- `TestRelayClaudeRefusalRetrySuccess`: 第一渠道拒答，第二渠道成功
  - Claude Messages 非流式/流式
  - OpenAI Chat Completions 非流式/流式
- `TestRelayClaudeRefusalRetryAllRefuse`: 所有渠道都拒答
  - Claude Messages 非流式/流式
- `TestRelayClaudeRefusalNoRetryWhenRetryTimesZero`: RetryTimes=0 不重试
- `TestRelayClaudeRefusalNoRetryWithSpecificChannel`: 指定渠道不重试

### 验证点

- 重试次数正确
- 消费日志指向成功渠道
- `admin_info.attempts` 包含失败记录
- 拒答分类为 `claude_message_id` 和 `claude_refusal_category`
- 不惩罚渠道健康
- 最终响应内容正确（成功或拒答）

## 与现有功能的关系

### VM 容量重试

拒答重试与 VM 容量重试**互不干扰**：
- 都使用独立计数器
- 都不占用主重试预算
- 都调用 `ResetRetryNextTry()`
- 在重试循环中的不同位置判断

### 渠道亲和

拒答重试会：
- 排除已尝试的渠道（`ExcludeChannels`）
- 尊重亲和设置中的"失败后是否重试"选项
- 不破坏成功后的亲和粘性

### 健康路由

拒答不影响：
- 渠道成功率评分
- 首字延迟统计
- 健康分档判断
- 路由权重分配

### 日志与计费

- 只为最终成功的请求计费
- 失败尝试记录在 `admin_info.attempts`
- 保留完整的重试链路追踪
- 支持按渠道/模型的消耗分析

## 配置与部署

### 无需配置

功能默认启用，无需添加任何开关或配置项。只要 `RetryTimes >= 1`，拒答重试就会自动生效。

### 兼容性

- **向后兼容**: 现有 API 行为保持不变
- **数据库**: 无需 schema 迁移
- **缓存**: 复用现有渠道和模型缓存
- **监控**: 与现有日志和统计系统无缝集成

### 回滚方案

如需禁用拒答重试：
1. 设置 `RetryTimes = 0`（同时禁用所有重试）
2. 或在 `claudeRefusalRetryAllowed()` 中添加硬编码 `return false`

## 局限性

1. **30 秒暂存限制**: 超过此时间的慢速流式响应无法重试
2. **内容类型限制**: 只能暂存特定类型的帧，遇到实际内容立即交付
3. **渠道覆盖**: 需要有多个同模型的 Claude 渠道才能生效
4. **透传模式不支持**: 开启透传的渠道不会触发重试
5. **非 Claude 模型不支持**: 仅适用于 Claude 系列模型

## 相关文件

### 核心实现
- `controller/relay.go` — 重试循环主逻辑
- `controller/relay_claude_refusal_retry.go` — 预授权逻辑
- `relay/channel/claude/relay-claude.go` — 暂存机制与拒答检测
- `relay/channel/aws/relay-aws.go` — AWS Bedrock 适配
- `relay/helper/stream_keepalive.go` — StreamPing 统一接口

### 支持代码
- `types/error.go` — `ErrorCodeClaudeRefusal`
- `constant/context_key.go` — `ContextKeyClaudeRefusalRetryAllowed`
- `service/relay_failure_classify.go` — 拒答错误分类
- `service/channel_retry.go` — `HasAlternateChannelCandidate`, `AppendClaudeRefusalAttempt`
- `relay/relaymode/relayinfo.go` — `ResetFirstResponseTime`

### 测试
- `controller/relay_claude_refusal_retry_test.go` — 集成测试

### 文档
- `docs/FEATURE_MAP.md` — 功能地图条目
- `.agents/notes/implemented/feature/2026-11-22-claude-refusal-channel-retry.md` — 本文档
