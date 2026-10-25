# 模型、分组与协议

调用前需要同时确认模型 ID、接口协议和令牌分组。模型广场负责展示当前公开模型与价格信息，带 Key 的模型列表更贴近该令牌的调用范围。二者用途不同：公开目录可见不代表当前账号有使用权限，模型出现在目录中也不保证每条渠道此刻都健康。

## 获取令牌模型列表

先在当前终端配置 `API_KEY` 环境变量，再读取列表：

```bash
curl --fail-with-body -sS 'https://meta-api.vip/v1/models' \
  -H "Authorization: Bearer $API_KEY"
```

OpenAI 格式通常返回 `object: "list"` 和 `data` 数组。使用每项的 `id` 作为请求中的模型名；不要使用营销名称、中文展示名称或自行猜测版本后缀。若响应提供 `supported_endpoint_types`，可辅助核对协议；未提供时去模型广场确认，不应默认支持全部协议。

| 信息 | 使用方式 |
| --- | --- |
| 模型 ID | 原样填入 `model`；Gemini 原生接口填在 URL 模型段 |
| 分组 | 创建或编辑令牌时选择，影响可用范围及计费倍率 |
| 支持协议 | 决定使用 Chat、Responses、Messages 或其他端点 |
| 价格与计价单位 | 区分输入/输出、缓存、按次或特定能力计费 |

## 在 SDK 中使用

安装 `openai` Python 包后运行。把 `MODEL_ID` 替换为模型广场中支持 Chat Completions 且令牌可用的 ID。

```python
import os
from openai import OpenAI

client = OpenAI(base_url="https://meta-api.vip/v1", api_key=os.environ["API_KEY"])
result = client.chat.completions.create(
    model="MODEL_ID",
    messages=[{"role": "user", "content": "用一句话打个招呼。"}],
)
print(result.choices[0].message.content)
```

只修改 model 并不能让 Chat 请求自动拥有 Responses 的全部功能。迁移协议时，还要同步调整输入结构、工具定义、流式事件和响应解析。不要把上游的内部映射名当作公开模型 ID；本站可能按公开名路由到不同上游。

## 分组变更和报错

分组下线、令牌模型限制、账号权限变化都会导致之前可用的配置失效。遇到“模型不存在”“无可用渠道”或“无权访问分组”，先重新读取目录，再检查令牌设置和系统公告。自动回退仅在已启用且备用范围可用时生效；实际消费按最终使用的分组和计费规则结算，具体看使用日志。

关联：[认证](/docs/reference/authentication)、[协议总览](/docs/reference/overview)、[请求样例](/docs/api-examples)、[平台教程](/docs)。
