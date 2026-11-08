# Meta Pulse 集成配置

Meta Pulse 是独立的增长与权益服务，不进入 new-api relay 请求主链路。中奖后，Pulse 将额度奖励自动发给已绑定的 new-api 账号，无需导入兑换码。内部奖励接口只用于内网或受控网关之间的服务调用，公网不应直接暴露。

## 社区展示与后台对接

Meta API 已移除「Meta Pulse」和「Pulse 运营」页面与侧栏入口；券余额、抽奖和奖励记录请在社区使用，结算运营由 Pulse 自身工具管理。API 消费不产生社区贡献、经验、等级或抽奖券。

`/api/pulse/summary`、`/api/pulse/rewards`、`/api/pulse/ops/overview` 已删除。旧书签与自行调用这些只读 BFF 的客户端需迁移至社区。`PULSE_INTERNAL_URL`、`PULSE_USER_BFF_HMAC_SECRET`、`PULSE_ADMIN_HMAC_SECRET` 在 Meta API 侧不再读取，旧配置不会阻断奖励与 SSO 配置加载；这些配置在其他项目中的用途不受影响。

## 在管理面板配置 new-api

升级到支持本配置页的版本后，进入「系统设置 → 配置 Meta Pulse 对接」。以下 new-api 配置均可在页面保存，无需为了修改这些设置重建容器：

| 配置 | 对应部署环境变量 | 用途 |
|---|---|---|
| 运行环境 | `PULSE_ENV` | 生产环境使用 `production`，执行强密钥和 Redis 防重放检查 |
| 自动发奖密钥及轮换旧密钥 | `PULSE_SERVICE_HMAC_SECRET`、`PULSE_SERVICE_HMAC_SECRET_PREVIOUS` | 接收 Pulse Worker 的发奖和查询请求 |
| 撤销密钥及轮换旧密钥 | `PULSE_ROLLBACK_HMAC_SECRET`、`PULSE_ROLLBACK_HMAC_SECRET_PREVIOUS` | 接收 Pulse 管理 API 的查询和受控撤销请求 |
| 允许新奖励到账 | `PULSE_BENEFIT_ENABLED` | 默认关闭；关闭后历史重放、查询和受控撤销仍可使用 |
| 单笔额度上限 | `PULSE_BENEFIT_MAX_GRANT_QUOTA` | 单笔奖励的正整数 quota 上限 |
| 单用户每日额度上限 | `PULSE_BENEFIT_USER_DAILY_QUOTA` | 同一用户一天累计收到的正整数 quota 上限 |
| 全站每日额度上限 | `PULSE_BENEFIT_DAILY_QUOTA` | 全站一天累计发出的正整数 quota 上限 |
| 论坛 SSO 密钥及轮换旧密钥 | `PULSE_FORUM_SSO_SECRET`、`PULSE_FORUM_SSO_SECRET_PREVIOUS` | 与社区插件的 SSO 配置对齐 |
| 论坛 SSO 回调地址 | `PULSE_FORUM_SSO_CALLBACK_URL` | 例如 `https://metar.uk/api/user-center/login/callback` |

页面将本次修改的配置存入 new-api 数据库，数据库中的对应字段优先于部署环境变量；没有修改的字段继续沿用原有配置，未设置的字段仍回退到环境变量。保存时清理已退役的 BFF 配置键，保留其他有效设置。修改同名环境变量不会覆盖已经明确保存的字段，因此后续调整这些字段仍应在页面完成。

保存后，处理该请求的实例立即使用新配置；共享数据库的其他实例按既有 `SYNC_FREQUENCY` 周期同步，默认 60 秒。多实例切换密钥或启停奖励时，须为同步留出时间。所有实例仍须先部署支持这些设置的新版本；页面配置不能代替代码升级、数据库迁移或 Redis、数据库等基础服务配置。

### 保留已有密钥与安全轮换

密钥输入框留空表示保留当前值，页面只展示已配置状态，不回传已有密钥。不要因输入框为空就重新生成整套密钥，也不要修改已有数据库密码、SSO 密钥或其他随机密钥。明确执行清除操作才会删除对应密钥；清除后不会重新启用同名环境变量中的旧值。轮换结束清除旧密钥也应使用明确清除操作。

「生成新密钥」只生成并展示新值，不自动保存，不会自动修改 Pulse 或社区配置。每个签名角色应使用不同的强随机密钥，尤其不能将发奖、撤销和 SSO 密钥混用。真实密钥不得提交 Git、写入日志或交给普通用户页面。

发奖与撤销由 new-api 接收验签：先在接收端设置新密钥并把旧密钥放入对应 `*_PREVIOUS`，待所有 new-api 实例同步后，再切换 Pulse 发送端，确认旧请求排空后清除旧密钥。SSO 由 new-api 使用当前密钥签发，社区插件必须在轮换期间接受新旧密钥，之后再清除旧值。不能只改一端就认为轮换完成。

### 额度单位与启用次序

额度按整数 quota 配置。先查看 `/api/status` 的 `data.quota_per_unit`，再据此配置 Pulse 的 `PULSE_QUOTA_PER_UNIT`。例如返回 `500000` 时，单笔 `250000`、单用户每日 `1000000`、全站每日 `5000000`，分别表示 0.5、2、10 API 额度单位；这些不是人民币金额，也不是实际运营预算建议。

三个上限均须为有效正整数，才能启用新奖励到账。每日上限按北京时间自然日累计毛发放额，撤销奖励不会恢复当天限额；限制在数据库事务中检查，Redis 不是额度事实源。

设置消费日志的开关只控制 Meta API 自身的日志留存，不受 Pulse Benefit 开关影响。填写生产环境、独立密钥和接收端限额，保存并核对各实例同步。社区赠券与抽奖由 Pulse/社区侧规则控制；生产验收应核对奖励凭证、账本和实际账户余额，不能只看前端成功提示。

## Pulse 与社区仍须分别配置

new-api 页面只管理 new-api 自身的设置，不会写入其他服务：

- Pulse 保留原有数据库密码和只读日志账号配置，直至新版 Pulse 已部署并确认不再连接日志库后再回收专用读取权限；账号共用时先拆分权限。Pulse 配置 `NEWAPI_INTERNAL_BASE_URL`、与 new-api 一致的发奖/撤销密钥和额度换算单位。发奖密钥只交给 Worker，撤销密钥交给管理 API。`PULSE_ACTIONS_ENABLED`、奖池创建与数据库迁移仍在 Pulse 侧完成。
- 社区插件的 `sso_hmac_secret` 对应 new-api 的论坛 SSO 密钥；`pulse_hmac_secret` 对应 Pulse 的 `PULSE_FORUM_HMAC_SECRET`，不是自动发奖密钥。插件的 `community_bff_hmac_secret` 与 Pulse 的 `PULSE_COMMUNITY_BFF_HMAC_SECRET` 使用另一组独立值，该值不属于 new-api 配置。
- 原有绑定正常时保留已有 SSO 设置。论坛回调必须为 HTTPS，路径固定为 `/api/user-center/login/callback`，不允许用户名密码、query 或 fragment。

历史付费来源证明设计记录见 [PULSE_FUNDING_PROVENANCE.md](PULSE_FUNDING_PROVENANCE.md)，其中消费证明已停用，钱包和资金核算记录仍须保留。社区赠券、奖池和抽奖配置按 Pulse 的社区运营文档操作。

## 部署环境兼容与内部接口

原有部署仍可通过环境变量提供上述设置；完整示例见仓库 `.env.example`。Compose 使用环境变量时，必须把配置实际注入 new-api 容器，仅修改用于 Compose 插值的 `.env` 不保证传入容器。可用权限为 `600` 的 `pulse.env` 配合 `env_file`，并检查 `environment` 中的同名值是否覆盖它。环境变量变更需要重建容器；页面保存的配置则按前述规则即时生效和同步。

Benefit 内部接口使用已验签服务身份独立限流，默认每 60 秒 600 次，不使用公共 API 的 IP 限流。`PULSE_BENEFIT_RATE_LIMIT_ENABLE`、`PULSE_BENEFIT_RATE_LIMIT`、`PULSE_BENEFIT_RATE_LIMIT_DURATION` 仍是部署环境参数，未包含在本次页面配置范围。SSO 入口另使用 CriticalRateLimit。生产环境仍须配置 Redis 防重放，并按真实吞吐完成压测。

奖励路由前缀为 `/api/internal/pulse/benefits`：

- `POST /grant`：仅接受 `pulse-settlement` 角色，按 `source_ref` 幂等发放不可转赠额度；`grant_id` 必须等于 `source_ref`。
- `POST /query` 或 `GET /query/:source_ref`：接受 `pulse-settlement` 或 `pulse-rollback` 角色，查询原始奖励状态，用于超时恢复；不得更换 `source_ref` 重发。
- `POST /rollback`：仅接受独立的 `pulse-rollback` 角色，使用原始 `source_ref` 追加可审计的撤销记录。

Grant 的请求体 `user_id` 必须与已验签的 `X-Pulse-User-Id` 一致。同一 `source_ref` 携带相同 payload 返回幂等成功，payload、用户或额度不一致返回 conflict。奖励凭证与钱包额度入账在同一事务中完成，不增加可转赠额度或付费抽奖资格。

系统设置保留 `/api/option/pulse`，密钥生成迁移到 `/api/option/pulse/secret/generate`，均要求超级管理员权限。Internal Benefit 和论坛 SSO 路由、鉴权与幂等恢复能力保持不变。

论坛登录入口为 `/api/forum/sso/start`。未登录用户先回到 `/login?next=/api/forum/sso/start`，登录（含 2FA）成功后由后端签发短期、单次 Login Ticket，并重定向到已配置的固定回调地址。浏览器不能通过 query 参数声明可信 `user_id`。
