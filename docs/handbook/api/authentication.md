# 认证与 API Key

模型接口使用控制台“令牌管理”中创建的 API Key。登录网站的账号密码、浏览器 Cookie、第三方厂商密钥和元衡令牌不能混用。创建前先在[模型广场](https://meta-api.vip/pricing)确认目标模型及其协议，再选择账号有权使用的分组；一把令牌可以调用哪些模型，由分组、模型限制及当前渠道共同决定。

[常用接口速查](/docs/reference/common-endpoints)

## 三种常见认证格式

| 协议 | 推荐请求头 |
| --- | --- |
| OpenAI 兼容 API | `Authorization: Bearer $API_KEY` |
| Claude Messages | `x-api-key: $API_KEY`，同时携带适用的 `anthropic-version` |
| Gemini 原生 API | `x-goog-api-key: $API_KEY` |

以下示例通过模型列表检查 Key。请先在终端设置 `API_KEY`，不要把值写进源码；也不要同时发送相互冲突的多个认证头。

```bash
curl --fail-with-body -sS 'https://meta-api.vip/v1/models' \
  -H "Authorization: Bearer $API_KEY"
```

返回 `data` 数组表示目录请求成功。后续生成请求还需正确的模型名、协议与余额；示例中的 `MODEL_ID` 必须替换为模型广场里支持该协议的 ID，不能仅凭认证成功就认为任意模型都可调用。

## 令牌限制如何生效

令牌可设置有效期、额度、模型范围、IP 范围、并发和时间窗请求数量。账号余额与令牌额度是不同限制：令牌标记为无限额度，不代表账户有无限余额，也不会绕过上游限流。填写 IP 白名单时应使用实际出网 IP；切换网络、代理或部署机器后要重新核对。

自动分组回退只会使用账号和令牌允许的备用范围。它不会授予新权限，也不保证每个模型都存在备用渠道。删除或停用 Key 后，应更新使用它的应用并检查是否仍有旧进程发送请求。

## 常见错误与安全习惯

401 常见于 Key 缺失、复制不完整、过期或被停用；403 可能是用户状态、IP 或分组限制；429 可能是令牌并发/窗口额度或其他层级限流。记录错误原文比只记录状态码更有用。不要通过把 Key 放到查询字符串中排查，URL 容易进入访问日志。

按应用或环境分配不同 Key，设置合理额度并定期检查使用日志。浏览器和移动端不应内置长期服务密钥，通常由自己的服务端保管并转发经过授权的请求。怀疑泄漏时，停用旧 Key、创建替代 Key，再检查异常用量；不要将完整 Key 发送给客服。

关联：[模型与分组](/docs/reference/models)、[API 总览](/docs/reference/overview)、[平台教程](/docs)、[错误排查](/docs/troubleshooting)。
