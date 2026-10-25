# 翻译、写作、学术与业务应用

这类应用通常在“模型服务”“AI 设置”或插件配置中保存连接。接入前确认当前版本允许使用自定义 OpenAI 兼容服务；如果只提供其他平台的内置选项，应寻找通用自定义选项，而不是将元衡 Key 填入其他平台的预设地址。客户端插件与本站账户是独立产品，具体界面以当前版本为准。

## 应用适配矩阵

| 场景 | 常见工具 | 重点检查 |
| --- | --- | --- |
| 网页及划词翻译 | 沉浸式翻译、NextAI Translator、Pot、欧路翻译 | 自定义端点、目标语言、单段长度、并发 |
| 笔记与写作 | Obsidian 插件、NoteGen、FeelFish | 当前选区、整篇笔记或整个知识库的发送范围 |
| 学术阅读 | Zotero 插件、GPT-Academic | PDF 提取、引用、长文切块与输出长度 |
| 快捷启动 | uTools 等扩展 | 插件来源、是否支持自定义服务、密钥存储位置 |
| 客服与消息桥接 | 懒人客服、cc-connect 等 | 机器人接收范围、人工审核和发送权限 |
| 配置管理 | All API Hub 等工具 | 导入导出的字段是否属于本站接口，导出是否包含密钥 |

## 通用字段

Base URL 填 `https://meta-api.vip/v1`；若字段明确叫“完整 API 地址”，Chat 接口填 `https://meta-api.vip/v1/chat/completions`。模型填写 `MODEL_ID` 对应的实际名称；GUI 的密钥字段填写独立用途令牌，脚本使用 `API_KEY` 环境变量。令牌的分组和额度应覆盖实际任务，不能用显示名称代替模型 ID。

先用短段落测试语言和格式，再启用全文处理。下面是一条独立的翻译请求，可帮助判断问题来自接口还是应用的提示词模板：

```bash
curl --fail-with-body 'https://meta-api.vip/v1/chat/completions' \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"model":"MODEL_ID","messages":[{"role":"system","content":"Translate into Chinese. Return only the translation."},{"role":"user","content":"The meeting starts at nine."}]}'
```

## 文档与数据范围

网页全文、PDF、邮件、聊天记录和本地笔记可能含个人或工作数据。先确认应用实际发送哪些内容，以及是否还上传到插件运营方或其云同步服务。用公开短文本验收；需要脱敏时在上传前完成。不要把密钥写进提示词、笔记模板、共享知识库或公开流程。

## 常见问题

短文本成功、长文失败时检查上下文限制、切块大小和输出预算；翻译只返回一部分时检查应用是否截断输入或输出。批量任务遇到 429 应降低并发并退避，不通过更换令牌绕过限制。生成的引用与事实仍需核对，翻译或学术插件不会替模型保证来源真实。

客服自动回复先保留人工审核，分别验证生成和发送两个环节。配置管理工具若要求管理权限而你只需调用模型，应取消该项功能；本站模型令牌并不自动具备管理账户或修改余额的权限。其他错误见[排错指南](/docs/troubleshooting)。

## 官方参考

- [沉浸式翻译的 OpenAI 服务配置](https://immersivetranslate.com/docs/services/openai/)
- [OpenAI 文本生成与消息格式](https://developers.openai.com/api/docs/guides/text)
- [Chatbox 自带 API Key 指南](https://chatboxai.app/en/guide/byok)
