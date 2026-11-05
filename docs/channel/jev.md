# Jev 决策模型

Meta API 通过现有 OpenRouter 渠道支持 `POST /v1/decisions`，供 Jev 返回结构化判断。它不使用 Chat Completions，不生成聊天回复，也不支持 SSE。模型广场的具体可用版本和分组以 `/api/pricing` 为准。

## 调用

```bash
curl https://meta-api.com/v1/decisions \
  -H "Authorization: Bearer $META_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "typesafe/jev-1.14",
    "state": "订单收到时已经损坏，请安排换货。",
    "questions": {
      "needs_support": {
        "type": "noul",
        "instructions": "客户是否需要订单售后帮助？"
      }
    }
  }'
```

响应保留上游 `answers`、概率、模型版本和 `usage`。`state` 支持字符串、对象、数组；`questions` 保留原生 `noul`、`choice`、`score`、`box` 定义，具体类型和多模态能力取决于所选版本。`assets`、`provider`、`session_id`、`trace`、`user` 可选字段保留。图像资产及 box 为供应商实验能力，文本验收不代表它们均已实测。

令牌模型限制、分组权限、余额预扣、限流、消费日志和健康统计沿用现有规则。最终按上游实际 `input_tokens`/`output_tokens` 结算；失败或用量缺失退回预扣，不使用估算用量收费，不自动重发已经提交的 Decisions 请求。客户端超时后也不应盲目重发。

## 管理员接入

1. 部署包含 Decisions 协议的应用版本。
2. 使用类型 20 的独立 TypeSafe 渠道，复用现有 OpenRouter 账号；标准基址为 `https://openrouter.ai/api`。
3. 通过原管理 API 配置价格、TypeSafe 厂商和精确模型元数据，再启用对应渠道。具体版本 `typesafe/jev-1.14`、`typesafe/jev-1.13` 默认只声明 `decisions`；别名须配置元数据端点 `{"decisions":{"path":"/v1/decisions","method":"POST"}}`。
4. 原厂公开价为输入 $0.042 / 百万 token、输出免费。销售倍率按本站当前口径单独设置，`CompletionRatio` 明确为 0；官方参考价仅用于展示。
5. 渠道覆盖可约束 `provider.only` 和 `provider.max_price`，须验证当前上游实际执行。映射后的模型不允许被参数覆盖再次替换。后台测试自动构造低成本的 Decisions 问题，模型详情提供七种语言的原生调用示例。

Jev Router 是动态选模入口，不继承本协议或固定价格。本次不自动加入 Router/latest；不新增数据库表或修改已有模型售价。

## 来源

- [OpenRouter Decisions API](https://openrouter.ai/docs/api/api-reference/alphadecisions/submit-a-decisions-request)
- [Jev 1.14](https://openrouter.ai/typesafe/jev-1.14)
- [TypeSafe 官方价格](https://help.typesafe.ai/articles/5187202169-how-much-does-jev-cost)
