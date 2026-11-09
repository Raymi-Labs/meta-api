# 元衡客户端：速度档位交接方案

可将本文直接交给客户端窗口实现。Meta端正在配套实现，未部署前能力接口不存在时只展示标准，不从模型名称猜测支持能力。

## 用户界面

沿用现有模型参数区域增加“速度”单选：标准 / Fast ×2 / UltraFast ×6。默认标准，不与推理强度混用，不强制开启。单模型模式先支持，多模型比较先隐藏该控制并不携带加速参数。费用倍率直接在选项中显示，详细解释放info tooltip，不增加说明段落。

## 能力接口（Meta端约定）

GET /api/user/service-tiers?model=gpt-6-astra&group=OpenAI%20%C2%B7%20%E9%B9%88%E9%B9%95%E7%8E%8B

使用已有本人控制台鉴权（会话/已有系统访问认证），不要传Muze密钥或使用前端自报支持范围。响应：

```json
{"success":true,"data":{"model":"gpt-6-astra","group":"OpenAI · 鹈鹕王","managed":true,"tiers":[{"id":"default","multiplier":1},{"id":"priority","multiplier":2},{"id":"ultrafast","multiplier":6}]}}
```

tiers是当前本人/当前组可用候选，可能只有default；不得展示未返回的付费选项。不保证请求时仍有渠道，服务端始终复核。managed=false表示未登记此功能，不代表其他上游接口的所有service_tier都不支持。加载失败显示标准入口并禁用发送已选付费档位，提供重试；不能静默把用户选定的付费档位换成标准发送。

## 请求与保存

- 标准：请求体省略service_tier。
- Fast：service_tier="priority"，绝不能写fast。
- UltraFast：service_tier="ultrafast"。
- 与reasoning.effort独立，放JSON顶层；Chat Completions和Responses一致。
- 后端对受管模型保证路由、出站字段、预扣与结算一致，客户端不得自己乘token数量或改价格配置。
- 模型/分组/账号变化后重新查询，丢弃旧请求响应，未确认新能力前禁止付费档发送；不要跨账号复用请求去重缓存。
- 可保存用户选择到当前本地会话，但恢复后必须重新校验能力；不自动迁移其他会话，不在后台自动调用模型。
- 重新生成、重试、代码导出使用相同有效参数；恢复或预览不发请求。
- 自定义JSON模式中字段原样由服务端校验，不显示与JSON矛盾的档位指示。
- 上游响应service_tier可能回显default，不据此改写用户选择或假称普通价。显示本地已发送档位；费用以Meta日志为准。

## 错误

service_tier_invalid：值不合法；service_tier_unavailable：当前无符合档位的授权渠道；service_tier_mismatch：配置造成出站档位变化；service_tier_pricing_unavailable：定价不支持或含重复倍率。错误不可自动改档重发，提供重试或由用户明确选择标准。

## 验收

能力请求鉴权/越权分组、接口404回退、首载/离线、账号切换迟到响应；标准不传字段、Fast=priority、UltraFast=ultrafast；恢复旧会话能力重查、快速切组和参数导出；手机/明暗/多语言；失败不偷偷换档或自动重试付费生成。不要保存上游Key。完成报告包含测试结果及本轮遗留进程为零。
