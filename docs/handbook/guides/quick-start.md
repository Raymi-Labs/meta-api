# 快速开始

本教程使用 Chat Completions 发起最小请求。运行请求会产生模型费用，请先在模型广场确认价格，并给测试令牌设置适当额度。

## 1. 准备账户和令牌

登录[控制台](/console)，查看账户余额或当前可用套餐。在[令牌管理](/console/token)创建专用于测试的令牌，选择所需分组、有效期和模型限制。不要把密钥放进浏览器前端、公开仓库或截图。

在本机终端设置环境变量。下面的值是占位符，需要替换为自己的密钥；不要原样复制为真实配置。

```bash
export API_KEY='YOUR_API_KEY'
export MODEL_ID='MODEL_ID'
```

## 2. 选择实际可用的模型

打开[模型广场](/pricing)，复制支持 Chat Completions 的模型 ID，替换上面的 `MODEL_ID`。也可以列出当前令牌可见的模型：

```bash
curl --fail-with-body https://meta-api.vip/v1/models \
  -H "Authorization: Bearer $API_KEY"
```

模型列表不是所有能力的承诺。若想发送图片、使用推理参数或调用工具，继续核对模型的支持端点与说明。

## 3. 发出最小请求

以下 Python 示例从环境变量读取配置，只发送文本，不启用工具或其他可选参数。

如尚未安装 SDK，可在自己的 Python 虚拟环境中运行以下命令。安装依赖不会调用模型；后面的生成请求才会使用模型服务。

```bash
python -m pip install openai
```

```python
import os
from openai import OpenAI

client = OpenAI(
    base_url="https://meta-api.vip/v1",
    api_key=os.environ["API_KEY"],
)
result = client.chat.completions.create(
    model=os.environ["MODEL_ID"],
    messages=[{"role": "user", "content": "Hello!"}],
)
print(result.choices[0].message.content)
```

已有应用通常只需修改提供商、Base URL、密钥和模型；是否接受自定义地址取决于应用版本。环境变量应在运行脚本的同一终端或部署配置中设置，`MODEL_ID` 必须是实际模型 ID；不要将占位符原样提交。

## 4. 验证结果与费用

成功返回后，在[使用日志](/console/log)核对时间、模型、令牌和消费。流式请求应等到终止事件或连接结束后再判断完整结果。账户余额、令牌额度和上游模型可用性分别检查，不要仅凭余额充足就排除权限问题。

## 第一次调用失败时

| 现象             | 下一步                                        |
| ---------------- | --------------------------------------------- |
| 401              | 核对密钥、Bearer 格式、是否误用了账户管理令牌 |
| 403 或模型不可用 | 核对令牌模型限制、分组和来源 IP               |
| 400              | 移除可选参数，确认模型支持当前协议            |
| 429              | 降低并发并等待，不要立即循环重试              |
| 连接失败         | 核对域名、代理、TLS 与本地网络                |

记录响应中的 `X-Oneapi-Request-Id`，再按[错误诊断](/docs/guides/http-errors)排查。提交问题时隐藏完整密钥和个人数据。
