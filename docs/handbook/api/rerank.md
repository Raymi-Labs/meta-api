# 文档重排 Rerank

Rerank 根据查询与一组候选文档的相关程度重新排序，适合检索后的精排。它不会代替向量库或搜索引擎召回文档，也不会直接生成最终答案。先在[模型广场](https://meta-api.vip/pricing)确认模型支持 Rerank 协议，将 `MODEL_ID` 替换为实际 ID。

## 最小请求

`query` 和 `documents` 都不能省略或为空。下例要求返回最相关的一条候选；Key 只从环境变量读取。

```bash
curl --fail-with-body -sS 'https://meta-api.vip/v1/rerank' \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "MODEL_ID",
    "query": "如何查看 API 调用费用？",
    "documents": [
      "使用日志记录请求模型、用量及相关费用。",
      "语言按钮可以切换界面语言。",
      "在令牌管理中创建或管理 API Key。"
    ],
    "top_n": 1
  }'
```

## 字段与结果

`model` 指定重排模型；`query` 是检索意图；`documents` 是已经召回的候选文本。`top_n` 控制期望返回的条数，具体限制由模型决定。部分模型支持 `return_documents` 返回原文，或支持长文分块参数；这些字段不是所有渠道都通用。首次接入建议先使用纯文本数组，避免直接提交复杂业务对象。

兼容结果通常在 `results[]` 中，`index` 指向原输入位置，`relevance_score` 表示模型的相关性评分，`document` 是否出现取决于参数及上游响应。应通过 index 关联应用自己的文档 ID，不能依靠返回文本反向猜测。得分不是经过统一校准的事实置信度，也不能直接跨模型比较。

## 接入检索流程

先用搜索或 Embeddings 找到有限的候选，再把查询与候选送入 Rerank，最后将靠前且有访问权限的文档交给生成模型。只传实际需要的片段有助于减少无关内容和输入消耗。应用需要保留原始排名、文档标识及过滤条件，便于分析某次结果为何被选中。

长文、候选数量和多语言匹配能力都取决于模型。若要设置相关性阈值，应使用自己的检索样本验证，不能把示例得分当通用标准。重排不会执行文档内的操作指令，后续生成或工具环节也应把候选内容作为资料处理。

## 常见错误

400 常见于空查询、空候选、模型不支持重排或输入长度超限；先缩小候选集复现。429 应降低并发并退避。应用应校验返回 index 是否在输入范围内，并处理缺失或不完整结果；不要让异常排名导致越权文档进入回答。

关联：[Embeddings](/docs/reference/embeddings)、[Chat Completions](/docs/reference/chat-completions)、[模型与协议](/docs/reference/models)、[故障排查](/docs/troubleshooting)。
