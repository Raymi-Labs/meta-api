# 文本向量 Embeddings

Embeddings 将文本转换为数值向量，常用于语义检索、相似度比较和检索增强生成。先在[模型广场](https://meta-api.vip/pricing)确认模型支持向量接口，将 `MODEL_ID` 替换为实际 ID。本站提供推理接口，不会自动为应用保存向量、建立索引或托管向量数据库。

## 最小请求

下面一次编码两段文本。`API_KEY` 从终端环境变量读取，输出应按 `data[].index` 与输入对应。

```bash
curl --fail-with-body -sS 'https://meta-api.vip/v1/embeddings' \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "MODEL_ID",
    "input": ["如何创建 API Key？", "在哪里查看调用费用？"]
  }'
```

## 字段与响应

| 字段 | 用途 |
| --- | --- |
| model | 支持 embeddings 的模型 ID |
| input | 待编码的单段文本或模型支持的批量输入 |
| encoding_format | 向量编码形式，需确认模型与客户端支持 |
| dimensions | 可选维度要求，仅部分模型支持 |
| data[].embedding | 返回的数值向量或所选编码表示 |
| data[].index | 结果对应的输入索引 |
| usage | 此次输入的用量信息，以实际返回为准 |

输入不能为空。批量条数、单条长度、最大 Token 数以及可调维度没有全站统一值，应以所选模型为准。不要为了兼容而自动截断重要文本；应按段落或业务结构拆分，并保存分段与原文位置的对应关系。

## 建立检索流程

应用通常先将文档分块、计算向量、写入自有索引，再对用户问题计算向量并检索相近片段。文档与查询应使用同一个兼容模型、同一维度及相同的必要前处理。更换模型时应重新评估并重建索引，不能把不同模型的向量混在一起比较。向量相近表示语义相关，并不证明文档事实正确或用户拥有访问权限。

有些模型对查询和文档编码有专门参数，只有确认支持时才添加。检索到候选后，可使用 Rerank 再排序，再把有权访问的文本交给对话模型；这些步骤都由应用编排。

## 常见错误

400 常见于空输入、批量或长度超限、维度不支持；无可用渠道时检查模型分组和 Key 权限。输出维度与数据库索引不一致时，应停止写入并核对配置，不要补零或裁剪来掩盖问题。429 应降低并发并分批退避。请求超时后记录批次标识，避免重复写入相同文档。

关联：[文档重排](/docs/reference/rerank)、[模型与协议](/docs/reference/models)、[Chat Completions](/docs/reference/chat-completions)、[认证与 API Key](/docs/reference/authentication)。
