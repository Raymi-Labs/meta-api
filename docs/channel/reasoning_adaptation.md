# 渠道思考参数适配

渠道编辑 → 基本信息 → 思考参数适配。复用管理员渠道接口和原 `setting` JSON，不新增数据库列、路由或价格。

## 模式与优先级

- 旧渠道未保存 `reasoning_adaptation` 时继承原全局 `ClaudeToOpenAIReasoningMap`，升级不批量更改旧渠道行为。
- 新建渠道表单默认“按厂商自动适配”；管理员可为旧渠道主动选择该模式。自动模式复用展示厂商的显式设置或全部模型归属一致时的保守识别，混合/不明厂商不猜测。
- “智谱 GLM-5.3 Chat 模板”固定选择模板，不依赖展示厂商；模板仍只匹配精确模型 `glm-5.3`。
- 渠道专属规则按模型映射后的上游名称精确匹配，优先于模板。规则只填写部分档位时，未填写档位沿用旧全局规则，不再落入模板。`inherit` 关闭厂商模板，但显式专属规则仍生效。
- 最后执行原有参数覆盖，管理员显式覆盖保持最高优先级。

精确模型规则支持 OpenAI/Azure 类型渠道的 Chat 与 Responses 出站协议，包括原生 Chat/Responses、Messages → Chat/Responses 和 Chat → Responses。`protocol` 指实际出站协议，不是客户端入口路径；同一模型可分别配置两条规则。厂商模板仍仅用于 Chat，原生 Messages 和请求体透传不应用规则。它不修改渠道类型、模型映射、选渠权限、重试、工具消息或计费。

## 首个模板

`zhipu-glm53-v1` 来自实际 GetToken `glm-5.3` Chat 接口明确拒绝不支持档位的证据，是可覆盖的兼容策略，不是对所有智谱接口的能力承诺。

| 原始档位 | 模板输出 |
|---|---|
| minimal / low | low |
| medium / high | high |
| xhigh / max | max |

Messages 优先取原始 `output_config.effort`，其次按既有 `thinking.budget_tokens` 档位（≤1280 low、≤2048 medium、≤4096 high、其余 max）转换，跳过会合并 high/max 的全局映射。预算缺省但显式启用 thinking 时沿用既有 medium 判定。未传思考参数不新增；显式 disabled 不注入 effort，**省略 effort 不代表已经验证上游关闭思考**，本功能不发明供应商的思考开关。原生 Chat 的 none/未知值和显式 disabled 保留原行为。

Flash、FlashX 和其他型号没有自动登记；型号支持应以实际供应商接口验收为准。不能用 `glm-` 前缀扩大能力声明。

## 精确模型规则

以下示例用于已独立确认其 Chat 档位能力的渠道；它不会使未配置的模型自动加入渠道模型列表。

```json
{
  "mode": "auto",
  "rules": [
    {
      "model": "your-verified-upstream-model",
      "protocol": "chat",
      "mapping": {
        "low": "low",
        "medium": "high",
        "high": "high",
        "max": "max",
        "xhigh": "max"
      }
    }
  ]
}
```

界面的“渠道专属规则”只填写上面 `rules` 数组。接口保存到 `setting.reasoning_adaptation`。最多32条规则，拒绝重复模型/协议、通配模型和未知档位，错误保存不会落库。可用档位为 minimal、low、medium、high、xhigh、max；不要用档位映射表达关闭思考。

## max 档位配置

全局 `ClaudeToOpenAIReasoningMap` 的目标档位允许 `max`，但默认仍为 `max→xhigh`，避免升级后旧模型收到不支持的参数。建议优先给已核实支持 max 的具体渠道设置规则，不批量修改全局默认。

例如，该渠道的 `gpt-6-astra` 已确认支持 max 时，在“渠道专属规则”填写：

```json
[
  {"model":"gpt-6-astra","protocol":"chat","mapping":{"max":"max"}},
  {"model":"gpt-6-astra","protocol":"responses","mapping":{"max":"max"}}
]
```

模型名称须使用模型映射后的上游名称。这里只保留 max，显式 xhigh 仍是 xhigh；不推测 Ultra 的客户端编排模式。Messages 未传 effort、仅传较大 thinking 预算时，沿用已有 max 桶推算：未配置规则仍映射为 xhigh，显式配置上述规则后该桶映射为 max，管理诊断 source 为 `thinking_budget`。这不证明客户端明确选择了 max。

普通日志显示实际发送的 effort。Responses 参数覆盖删除 reasoning/effort 时清除旧值，不继续显示 max。历史日志不回写；配置与本地模拟验证不代表上游真实支持或已部署。

## 诊断与迁移

消费和失败日志的 `other.admin_info.reasoning_adaptation` 包含 channel_id、template、protocol、source、original、mapped、final、final_present、override_changed。失败尝试也保留该次快照。final 在参数覆盖后采集；未知值只记录 `other`，不记录原始正文或任意参数内容。重试清理前次状态，普通用户接口沿用原 admin_info 脱敏。

上游明确返回 reasoning_effort 枚举/值拒绝时，失败提示为“上游不支持当前思考强度”；不增加对400的重试或自动探测。

上线后先备份渠道原 setting 和 param_override，选择一条已验证渠道启用自动模式，以原客户端分别验证 low/high/max、流式、工具多轮及消费记录。既有手写覆盖不会自动删除；确认最终出站值后再清理重复规则。回退可切回“继承全局映射”并清空本次专属规则，恢复原覆盖配置；应用更新/回退仍走受保护入口。本次发布本身不代表生产部署或上游实测完成。
