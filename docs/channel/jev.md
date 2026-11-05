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

## 生产接入记录（2026-11-06）

生产已通过 `newapi-update sha-aaa02ae` 更新到 v1.20.2，完成在线备份、两次数据库迁移预检、健康检查和恢复包封存。实现提交为 `e1555e1ff`，价格来源及参数错误状态修复为 `aaa02ae63`。

- 独立渠道 `OpenRouter · TypeSafe`（1653），厂商 TypeSafe（71），分组“企业级”；模型元数据 399/400 对应 1.14/1.13。
- 两个型号均仅声明 `POST /v1/decisions`，上下文 262144；禁用自动上游型号同步，避免未经核价的新型号进入渠道。
- 原厂输入 $0.042/M、输出免费；本站采用 ECB 2026-11-05 的 USD/CNY 6.668389166072702 快照和现有每能量 ¥0.2 口径，`ModelRatio=0.700180862438`、`CompletionRatio=0`，企业级倍率为 1。当前输入约 ¥0.280072344975/M，输出免费；这是接入时快照，不是自动跟随汇率的任务。
- 渠道限定 TypeSafe、按价格排序，并保留调用者更严格的价格上限；最高输入 $0.042/M、输出和请求附加费上限均为 0。更严格的零输入限价已实测被上游拒绝。

公网目录从 217 个型号增至 219 个，既有型号未移除。1.14 经 `meta-api.com`、1.13 经 `meta-api.vip` 的文本判断均成功，每次实际 272 输入/22 输出，各扣 190 quota，两条消费日志合计 380；输出未计费。流式请求返回 400、模型越权返回 403、无合格限价服务商返回 404，失败请求未产生消费扣费。临时验收令牌均已删除，后台两个型号的原生 Decisions 测试也通过。

桌面及手机深色页面已检查模型资料、官方参考价、分组价格和七语言示例入口。图像及其余问题类型未做生产推理验收。服务器操作证据保存在 `/opt/meta-api/operations/jev-20261106/`，其中配置前价格快照、限价策略、部署日志和验收记录分开存放；凭据不进入仓库。

[v1.20.2 Release](https://github.com/Raymi-Labs/meta-api/releases/tag/v1.20.2) 的 Linux 两架构程序、双架构 Docker 清单、Windows 安装/便携包、文档和管理包均已核实。[发布运行](https://github.com/Raymi-Labs/meta-api/actions/runs/42068369096) 首轮因 Ubuntu Azure 镜像下载交叉编译器反复重试，导致 Windows 上传等待 Linux Release 超时；后续单独重试 Linux 作业及原 Windows 上传，保留已有附件，最终全部作业成功。该附件恢复未再次改动生产版本。

## 来源

- [OpenRouter Decisions API](https://openrouter.ai/docs/api/api-reference/alphadecisions/submit-a-decisions-request)
- [Jev 1.14](https://openrouter.ai/typesafe/jev-1.14)
- [TypeSafe 官方价格](https://help.typesafe.ai/articles/5187202169-how-much-does-jev-cost)
