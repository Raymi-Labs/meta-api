# Agent Note: Kling 提交与轮询密钥解析一致性

Status: implemented

## Problem

Kling 原生 Go 适配器在接受单密钥分支前要求恰好两段，导致非 sk- 的 Bearer 密钥提交失败；轮询却把任意解析失败的原串作为 Bearer 发送，提交和查询的鉴权规则不同。

## Decision

借鉴上游 a8535fac1 的单密钥支持，复用本地适配器，不引入任务插件。提交和轮询共用一个解析器：去除首尾空白；单密钥原样作 Bearer；非空 accessKey|secretKey 签发原有 HS256 JWT；空值、缺少任一段或多段配置在出站前失败。保留现有 sk- 网关规则及 /kling 路径，路径判定与解析使用相同的空白规范化。轮询不再把无效双密钥回退为 Bearer。

范围仅限鉴权和路径判定，不改模型、参数、价格、任务终态或现有 Header Override。视频定价与付费测试提案只涉及相邻能力，不被本修复实现或取代；本轮仅使用本地 HTTP 夹具，不发起真实视频生成。

## Alternatives considered

- 只挪动单密钥判断：最小修复提交，但轮询仍接受格式错误的配置并向外发送，无法保持两条入口一致。
- 将所有密钥都当 Bearer：最容易支持代理服务，但破坏官方 AK/SK 的签名协议，因此保留双模式。
- 搬入上游 JS 插件：可以直接使用上游实现，但本地是 Go 原生任务链，不为局部兼容引入另一套运行时。

## Verification

- 单密钥、带首尾空白密钥、sk- 网关和 AK/SK 在提交及轮询使用相同凭据规则，文本/图片路径保持原语义。
- JWT 算法、issuer、签名与有效期回归通过；空值与错误分隔符不出站、不在错误中输出密钥。
- 回归入口：`go test ./relay/channel/task/kling -count=1`、`go test -race ./relay/channel/task/kling -count=1`、`go build ./relay/channel/task/kling` 与 `make verify-notes`。本地 HTTP 服务覆盖提交和轮询，不调用外部供应商。

## Consequences

以前轮询会向上游发送错误配置，改为本地失败；已有非空 AK/SK 与有效单密钥不受影响。模拟证明适配行为，不证明具体供应商账号或生产配置可用。
