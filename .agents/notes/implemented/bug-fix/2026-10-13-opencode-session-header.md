# Agent Note: OpenCode 会话头默认转发

Status: implemented

## Problem

OpenCode Go 的普通 Chat 请求要求稳定的 x-opencode-session。OpenAI 兼容适配器不默认复制该客户端头，逐渠道设置 header_override 虽能修复现有请求，但新建渠道会再次遗漏。后台渠道测试没有业务会话，现有客户端头占位符也会在测试模式被跳过。

## Decision

在公共 DoApiRequest 路径根据实际出站 URL 识别 HTTPS opencode.ai 的 /zen/v1/ 与 /zen/go/v1/，默认复制非空客户端会话头。识别不依赖渠道 ID、名称、模型或分组，覆盖 Chat、Messages、Responses 与流式请求。明确的渠道请求头覆盖仍在默认处理之后生效。

仅后台 IsChannelTest 请求生成独立随机会话 ID，不使用管理浏览器的会话头。普通 API 缺少会话头时不按令牌、用户或渠道合成共享会话。自动携带的会话头不跟随重定向到其他目标，保留已有重定向检查。

未知代理域名继续使用已有 header_override，不凭渠道名称猜测供应商，不增加设置项或接口。

活跃笔记检索：渠道响应头超时、CPA 身份文档与本改动属于相邻出站路径，保持其超时、代理、可信身份和显式覆盖规则；上游能力切换笔记不涵盖会话头，不改变其重试决定。没有可直接更新的既有会话头笔记。

## Testing

- `go test ./relay/channel/... ./controller ./middleware ./service -count=1` 通过。
- `go test -race ./relay/channel -run 'Test(OpenCode|ProcessHeaderOverride|CPAIdentity)' -count=1` 通过。相邻旧测试在 t.Parallel 中写 Gin 全局模式，竞态检查暴露此测试自身问题；移除六处无必要的全局模式写入后原有断言保持不变。
- HTTP 客户端传输桩核验实际出站 URL、头和请求体，覆盖两种渠道 ID、三种协议、流式/非流式、缺头、覆盖优先级、后台独立会话及重定向；未向第三方发送测试请求。
- Bun 生产构建及 Chromium 五个公开页预渲染通过。

## Alternatives considered

- 只维护渠道模板：实现最少，但无法覆盖未复制模板的新渠道，保留本次故障诱因。
- 全部供应商默认透传：兼容未知代理，但会将会话标识发送到不相关上游，范围超过需求。
- 按渠道 ID 或名称匹配：可以命中当前配置，但新增、重建或重命名后失效。
- 普通请求缺头时生成固定或每请求随机 ID：可消除报错，但前者合并不同会话，后者破坏连续会话与缓存；保持调用者边界。

## Consequences

官方渠道无需逐项填写会话头透传配置，后台测试也拥有独立测试会话；代价是调用者仍需维护自己的稳定会话 ID。识别范围是当前已验证的官方 API 地址，未来域名变化或自定义代理需要显式配置或后续适配。不改变数据库、计费、重试或请求体；会话头成功不证明视频、声音或其他模型能力可用。发布与生产升级分别核实。
