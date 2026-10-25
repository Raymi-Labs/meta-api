# NAN 常见报错排查

> 按错误文本快速定位。先看通用检查，再看对应模块。

## 1. 通用 5 项

1. API Key 是否完整，通常以 `sk-` 开头。
2. 令牌是否启用、过期或额度受限。
3. 账户余额是否充足。
4. 模型是否在当前分组可用。
5. Base URL 是否正确：OpenAI/Codex 通常带 `/v1`，Claude/Gemini 通常不带。

## 2. 注册与登录

| 现象         | 常见原因                       | 处理                               |
| ------------ | ------------------------------ | ---------------------------------- |
| 收不到验证码 | 邮箱错误、垃圾箱、邮件延迟     | 检查邮箱，等待 1-2 分钟重试        |
| 人机校验失败 | Turnstile token 过期或重复提交 | 刷新页面，重新校验后提交           |
| 邀请未绑定   | 链接缺少 `aff` 或不是同页注册  | 使用 `/register?aff=xxxx` 完整链接 |
| 重定向过多   | 旧 Cookie 或注册链接异常       | 清理站点 Cookie，确认链接格式      |

## 3. 认证错误

### 3.1 `401 unauthorized` / `invalid api key`

检查：

- Key 是否复制完整。
- 请求头是否为 `Authorization: Bearer sk-xxxx`。
- 令牌是否启用。
- 客户端是否还在读旧环境变量。

验证环境变量：

```bash
echo $NAN_API_KEY
echo $ANTHROPIC_AUTH_TOKEN
echo $GEMINI_API_KEY
echo $CRS_OAI_KEY
```

Windows PowerShell：

```powershell
echo $env:NAN_API_KEY
echo $env:ANTHROPIC_AUTH_TOKEN
echo $env:GEMINI_API_KEY
echo $env:CRS_OAI_KEY
```

### 3.2 `missing api key` / `no auth credentials`

客户端没有读到 key。设置环境变量后，重新打开终端、VSCode 或 IDE。

## 4. 模型与分组

| 报错                | 常见原因               | 处理                                  |
| ------------------- | ---------------------- | ------------------------------------- |
| `model not found`   | 模型名写错或分组不可用 | 去模型广场复制模型名                  |
| `model unavailable` | 模型或渠道暂时不可用   | 换同类模型或联系管理员                |
| Claude 调用失败     | 分组或协议不匹配       | 核对模型广场后修改令牌分组            |
| Gemini 调用失败     | 分组或协议不匹配       | 核对模型广场后修改令牌分组            |
| Codex 调用失败      | 分组或协议不匹配       | 核对模型是否支持 Responses 和当前分组 |

## 5. 客户端配置

| 客户端      | 必查项                                                                       |
| ----------- | ---------------------------------------------------------------------------- |
| Codex       | `base_url=https://meta-api.vip/v1`，`wire_api=responses`，`NAN_API_KEY` 生效 |
| Claude Code | `ANTHROPIC_BASE_URL` 不带 `/v1`，分组匹配模型类型                            |
| Gemini      | `GOOGLE_GEMINI_BASE_URL` 不带 `/v1`，`GEMINI_MODEL` 可用                     |
| OpenCode    | `baseURL` 带 `/v1`，`CRS_OAI_KEY` 生效                                       |
| OpenClaw    | `openclaw status` 可达，配置为 Responses 新格式                              |
| CC Switch   | 已安装客户端，浏览器允许打开 `ccswitch://`                                   |

## 6. 流式与网络

| 报错                                    | 常见原因                       | 处理                                      |
| --------------------------------------- | ------------------------------ | ----------------------------------------- |
| `stream disconnected before completion` | 上游断流、客户端超时、网络中断 | 记录时间、模型、请求 ID，联系管理员查日志 |
| `timeout awaiting response headers`     | 上游响应慢                     | 换模型/线路或稍后重试                     |
| `EOF` / `unexpected EOF`                | 上游连接提前关闭               | 联系管理员查渠道日志                      |
| `connection refused`                    | 上游地址不可达或本地网关没启动 | 检查网关、代理、防火墙                    |
| SSE 一直断                              | 代理/CDN 长连接不稳定          | 尝试非流式或换线路                        |

## 7. 充值与扣费

| 现象           | 处理                                       |
| -------------- | ------------------------------------------ |
| 充值未到账     | 提供订单号、支付时间、金额，联系管理员补单 |
| 余额不足       | 充值并检查令牌额度限制                     |
| 不知道为何扣费 | 看使用日志的模型、时间、消耗、令牌         |
| 扣费异常       | 截图使用日志，带请求时间联系管理员         |

## 8. 发票与明细账单

| 现象             | 处理                                   |
| ---------------- | -------------------------------------- |
| 订单不能选择     | 只可选支付成功且未被申请占用的订单     |
| 驳回后重新申请   | 驳回会释放订单，可重新提交             |
| 专票提交失败     | 单位名称、税号必填                     |
| 邮件没收到附件   | 检查邮箱和垃圾箱，必要时联系管理员重发 |
| 需要明细账单 PDF | 管理员审核时勾选发送明细账单附件       |

## 9. 联系管理员时提供

```text i18n-template
账号邮箱/用户ID：
发生时间：
客户端：
模型名：
令牌名称：
错误文本：
是否流式：
```

不要发送完整 API Key。
