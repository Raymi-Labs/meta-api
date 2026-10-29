# 常用接口速查

本页示例使用 `https://meta-api.com`，可复制后替换凭据。不要把文档域名作为 API 地址，也不要给 `/api/*` 路径额外添加 `/v1`。

## 认证与 API Key

`API_KEY` 是令牌管理中的 `sk-...` 密钥。`ACCESS_TOKEN` 是个人设置中生成的“系统访问令牌”，`USER_ID` 是该账户的数字 ID；查询账户接口时必须同时发送两个请求头，不能用 API Key 替代系统访问令牌。

```bash
export API_KEY='sk-REPLACE_ME'
export ACCESS_TOKEN='REPLACE_ME'
export USER_ID='123'
```

以下账户接口也支持登录 Cookie，但仍须携带匹配的 `New-Api-User`；示例使用系统访问令牌，凭据只保存在自己的服务端或终端环境中。

## 查询接口

| 用途           | 方法与路径                           | 认证                     | 返回字段                                                                   |
| -------------- | ------------------------------------ | ------------------------ | -------------------------------------------------------------------------- |
| 站点配置       | `GET /api/status`                    | —                        | `data.quota_per_unit`、`data.quota_display_type`、`data.usd_exchange_rate` |
| 公开模型与价格 | `GET /api/pricing`                   | —                        | `data`、`usable_group`、`group_ratio`                                      |
| 模型列表       | `GET /v1/models`                     | `API_KEY`                | `data[].id`                                                                |
| Key 剩余额度   | `GET /api/usage/token/`              | `API_KEY`                | `data.total_available`、`data.total_used`、`data.unlimited_quota`          |
| Key 最近日志   | `GET /api/log/token`                 | `API_KEY`                | `data[]`                                                                   |
| 账户余额       | `GET /api/user/self`                 | `ACCESS_TOKEN + USER_ID` | `data.quota`、`data.used_quota`                                            |
| 可用分组       | `GET /api/user/self/groups`          | `ACCESS_TOKEN + USER_ID` | `data`                                                                     |
| 令牌列表       | `GET /api/token/`                    | `ACCESS_TOKEN + USER_ID` | `data.items`、`data.total`                                                 |
| 调用日志       | `GET /api/log/self`                  | `ACCESS_TOKEN + USER_ID` | `data.items`、`data.total`                                                 |
| 用量统计       | `GET /api/log/self/stat`             | `ACCESS_TOKEN + USER_ID` | `data.quota`、`data.rpm`、`data.tpm`                                       |
| 支付记录       | `GET /api/user/payment-records/self` | `ACCESS_TOKEN + USER_ID` | `data.items`、`data.total`                                                 |

公开价格目录不代表 Key 的可用范围；调用前用 `/v1/models` 核对。`/api/log/token` 只返回该 Key 的最近记录，不支持分页，完整筛选请用账户日志接口。

## 余额与额度

```bash
curl --fail-with-body -sS 'https://meta-api.com/api/usage/token/' \
  -H "Authorization: Bearer $API_KEY"

curl --fail-with-body -sS 'https://meta-api.com/api/user/self' \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "New-Api-User: $USER_ID"

curl --fail-with-body -sS 'https://meta-api.com/api/status'
```

账户余额读 `data.quota`，Key 剩余额度读 `data.total_available`；两者都是内部额度，不是模型 Token 数。`unlimited_quota=true` 只表示 Key 不限额，不代表账户余额无限。过期、禁用或额度耗尽的 Key 会被查询鉴权拒绝。

美元金额 = 内部额度 ÷ `quota_per_unit`；人民币展示再乘 `usd_exchange_rate`。这些参数从 `/api/status` 的 `data` 读取，当前换算基数为 `500000`，即 `500000` 额度对应 1 美元；其他展示单位按站点设置解释。

## 日志、统计与账单

```bash
curl --fail-with-body -sS 'https://meta-api.com/api/log/token' \
  -H "Authorization: Bearer $API_KEY"

curl --fail-with-body -sS --get 'https://meta-api.com/api/log/self' \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "New-Api-User: $USER_ID" \
  --data-urlencode 'p=1' \
  --data-urlencode 'page_size=20' \
  --data-urlencode 'type=2' \
  --data-urlencode 'start_timestamp=1793203200' \
  --data-urlencode 'end_timestamp=1793289599'

curl --fail-with-body -sS --get 'https://meta-api.com/api/log/self/stat' \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "New-Api-User: $USER_ID" \
  --data-urlencode 'type=2' \
  --data-urlencode 'start_timestamp=1793203200' \
  --data-urlencode 'end_timestamp=1793289599'

curl --fail-with-body -sS 'https://meta-api.com/api/user/payment-records/self?p=1&page_size=20' \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "New-Api-User: $USER_ID"
```

令牌列表、账户日志和支付记录使用 `p`（从 1 开始）与 `page_size`（最大 100），读取 `data.items` 和 `data.total`。示例时间为 Unix 秒，请换成所需范围；日志 `type=2` 表示消费，列表与统计应使用相同筛选。

日志还可按 `model_name`、`token_name`、`group`、`request_id` 筛选；支付记录可按 `keyword`、`status`、`payment_method` 和起止时间筛选。`/api/log/self/stat` 的 `quota` 是区间消耗，不是当前余额；`/api/log/self/search` 已废弃。

## 模型调用

| 用途            | 方法与路径                                                                              | 文档                                                                       |
| --------------- | --------------------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| Chat            | `POST /v1/chat/completions`                                                             | [Chat Completions](/docs/reference/chat-completions)                       |
| Responses       | `POST /v1/responses`                                                                    | [Responses API](/docs/reference/responses)                                 |
| Claude Messages | `POST /v1/messages`                                                                     | [Claude Messages](/docs/reference/messages)                                |
| Gemini          | `POST /v1beta/models/MODEL_ID:generateContent`                                          | [Gemini](/docs/reference/gemini)                                           |
| 图片生成、编辑  | `POST /v1/images/generations`、`POST /v1/images/edits`                                  | [Images](/docs/reference/images)                                           |
| 音频            | `POST /v1/audio/speech`、`POST /v1/audio/transcriptions`、`POST /v1/audio/translations` | [Audio](/docs/reference/audio)                                             |
| 向量、重排      | `POST /v1/embeddings`、`POST /v1/rerank`                                                | [Embeddings](/docs/reference/embeddings)、[Rerank](/docs/reference/rerank) |
| 视频任务        | `POST /v1/videos`、`GET /v1/videos/{task_id}`、`GET /v1/videos/{task_id}/content`       | [Video](/docs/reference/video)                                             |

模型调用使用 API Key，具体认证头和请求体见各协议文档；只调用模型广场已声明支持的协议。生成请求会按实际使用计费，查询接口成功不能证明生成能力可用。

## 兼容余额接口

旧客户端还可用 API Key 请求 `GET /v1/dashboard/billing/subscription` 与 `GET /v1/dashboard/billing/usage`，也支持去掉 `/v1` 的路径。

普通限额场景的剩余额度 = `hard_limit_usd - total_usage / 100`。统计范围由站点 `DisplayTokenStatEnabled` 决定：开启时是 Key，关闭时是账户；字段虽带 USD，数值仍随站点展示单位变化。无限 Key 使用占位上限，新集成优先使用本页明确区分账户和 Key 的接口。

## 错误处理

除 HTTP 状态外，还须检查响应正文：多数查询接口以 `success` 表示成功，Key 额度接口成功时使用 `code: true`；模型接口可能返回 `error`。401 检查凭据与有效期，403 检查权限，429 降低频率；不要把 HTTP 200 或缺失字段当作余额为零。
