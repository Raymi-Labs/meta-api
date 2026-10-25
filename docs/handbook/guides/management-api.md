# 账户与令牌管理 API

管理 API 用于查询账户和管理令牌，认证与模型推理接口不同。公开文档站不代理这些请求，调用地址始终是 `https://meta-api.vip`。

## 认证与权限

网页控制台使用登录会话。需要脚本访问时，使用账户的系统访问令牌，并在 `New-Api-User` 头中传入与令牌所属账户一致的数字用户 ID。模型调用用的 `sk-` API Key 不能代替账户管理访问令牌。

系统访问令牌沿用所属账户的角色，不是只读密钥，也没有模型 Key 的逐项权限、额度与 IP 限制。它可访问该角色允许的管理接口，应只保存在可信后端，不放入聊天客户端或第三方插件。普通账户不能借此访问管理员接口；需要安全验证的操作仍可能要求登录会话及额外验证。模型体验使用的 `/pg/chat/completions` 不接受系统访问令牌。

在[个人设置](/console/personal)的“安全设置 → 系统访问令牌”生成或轮换。脚本请求不要混带其他账户的登录 Cookie；已有登录会话会优先参与认证，用户 ID 必须与实际认证账户一致。

```bash
export MANAGEMENT_TOKEN='YOUR_ACCOUNT_ACCESS_TOKEN'
export NEW_API_USER='YOUR_NUMERIC_USER_ID'

curl --fail-with-body https://meta-api.vip/api/user/self \
  -H "Authorization: Bearer $MANAGEMENT_TOKEN" \
  -H "New-Api-User: $NEW_API_USER"
```

## 常用只读接口

| 方法与路径                                            | 用途                             |
| ----------------------------------------------------- | -------------------------------- |
| `GET /api/user/self`                                  | 当前账户信息                     |
| `GET /api/user/models`                                | 当前账户模型范围                 |
| `GET /api/token/?p=1&page_size=20`                    | 分页查看本人的令牌               |
| `GET /api/token/search?keyword=NAME&p=1&page_size=20` | 按令牌名称搜索本人的令牌         |
| `GET /api/token/:id`                                  | 本人令牌详情，Key 仍为脱敏值     |
| `GET /api/token/models?token_id=TOKEN_ID`             | 查询本人某把令牌可配置的模型范围 |
| `GET /api/user/payment-records/self`                  | 本人支付记录                     |
| `GET /api/user/quota-notices`                         | 本人额度通知记录                 |

管理响应通常采用 `success`、`message`、`data` 包装。HTTP 成功仍应检查 `success`。令牌列表、支付记录等分页接口使用从 1 开始的 `p` 与正整数 `page_size`，当前每页最多 100 条；返回 `data.page`、`data.page_size`、`data.total` 和 `data.items`。读取实际返回的页大小逐页处理，不要用 `page`、`limit` 替代请求参数，也不要假设结果与 `/v1/models` 相同。

`GET /api/token/models` 接受登录会话或上述管理认证。传 `token_id` 时按本人令牌查询；未传时可用 `group` 与 `auto_group_fallback` 查询拟配置范围，`detail=true` 返回模型选项详情。它返回数组，不走分页，也不是一次模型生成测试。模型调用 Key 应使用 `/v1/models`。

## 修改与删除操作

| 方法与路径                         | 用途与注意事项                                                      |
| ---------------------------------- | ------------------------------------------------------------------- |
| `POST /api/token/`                 | 创建令牌；分组、有效期、额度与模型范围需满足账户权限                |
| `PUT /api/token/`                  | 修改令牌；先读取现有字段，避免覆盖不打算修改的限制                  |
| `PUT /api/token/?status_only=true` | 只修改本人令牌状态；请求体包含 `id` 和 `status`，1 为启用、2 为禁用 |
| `DELETE /api/token/:id`            | 删除令牌，已有客户端随后无法继续使用                                |
| `PUT /api/user/setting`            | 更新个人设置，使用当前版本支持的字段                                |

普通更新不是局部 PATCH，省略字段可能重置原有限制。只需启停时使用状态专用参数；过期或额度耗尽的令牌不能只改状态就恢复。自动化写入前应对照本版本控制台实际提交字段，在专用测试令牌上验证并回查。列表和详情均不返回完整 Key；确需取得本人 Key 时使用 `GET /api/token/:id/key`，不要把该响应写入普通日志。

## 安全与版本兼容

管理 API 与控制台版本一起演进，不属于所有第三方平台共同遵循的稳定标准。升级前验证字段与分页行为，记录变更目标和返回结果。管理员接口仍需要管理员身份；拥有普通系统访问令牌不会提升权限。

个人密钥与轮换流程见[API Key 管理](/docs/account/api-keys)，登录保护见[账户安全](/docs/account/account-security)。
