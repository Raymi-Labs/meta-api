# Agent Note: K3 复测范围与原始请求保真

Status: implemented

## Problem

复测验收必须区分网关保真、供应商行为、部署配置和测量口径。单元测试证明字段被转发，不证明模型理解视频或执行工具；客户使用 max_tokens 的反例也不能用 max_completion_tokens 自动改写来代替复现。

## Existing capabilities and impact

- 复用请求 DTO 对消息级工具的省略 content 与显式空值区分、视频字符串/对象解析和现有请求体缓存；不新建代理或计费实现。
- `ResponsesRequestBodyLimitMB` 是复用的管理配置，覆盖路径在 `common/request_body_limit.go`；若 Messages 不应用同一配置，就无法统一声明三种协议都是192 MiB。
- [既有流式完整性决定](2026-09-29-kimi-request-stream-integrity.md)部分重叠：保留有本地回归证据的保真与清理修复，撤销仅凭模型名把 K3 输出参数改写的决定；不把跨协议修复当作客户 Chat 延迟已解决。
- 活跃渠道用量及笔记流程记录与本次运行时行为无重叠。现有对话上限设置继续由管理员控制；已有较低全局上限优先，不迁移数据库或修改生产配置。

## Decision

- 保留调用者的 max_tokens、max_completion_tokens、reasoning_effort、temperature 等原始要求；未知供应商是否遵守参数由实际请求验收，不擅自改成另一种契约。
- 对 Chat、Messages、Responses 及已登记兼容路径统一应用配置的解压后完整请求体上限；采用严格路径边界，不限制无关接口。不将全站默认值或管理员已有值强改成 192。
- 使用报告相同的 max_tokens=16/32/50、reasoning_effort=none、视频两种格式及工具省略/空值用例核对出站请求。
- 以隔离 HTTP 上游与数据库验证本地三协议 SSE 和退款；以实际大请求和超限边界验证字节完整性、413 和缓存清理。
- 第一项“已知假成功识别”留作阻塞验收项：当前仓库未定位到伙伴对应实现，不重复编造识别规则，不拿普通 Responses 失败退款用例替代。七条历史候选不调整余额。
- 首页/文档/KVV 单独核对；找不到 KVV 入口、原始产物或实际部署证据时明确写未验收。
- 失败验收发现的日志计数并发访问按[局部日志同步决定](2026-09-29-relay-log-counter-synchronization.md)处理，只同步计数及调度状态，不改变扣费、退款、渠道路由或日志内容。
- 动态工具本地模拟与官方正反例的对应按[官方KVV对齐](../testing/2026-09-29-official-kvv-alignment.md)记录。system + content="" + tools为官方正例，不将空字符串与非空内容混同；省略/null仍独立保真。门户与官方脚本的覆盖范围分别核对。

## Alternatives considered

- **继续按模型名自动兼容所有参数**：可以让部分供应商接受请求，但无法解释报告原本使用 max_tokens 的失败，且可能掩盖不同供应商的语义。撤下该改写，只做保真检查。
- **把全站默认限制直接改为 192 MiB**：操作简单，但会覆盖既有资源保护意图，且无法提高 Nginx/CDN 的较低限制。复用管理配置，补齐三协议一致性并给出部署检查清单。
- **整体撤回所有局部修复**：回到原状态最容易比较，但会重新引入已复现的工具字段、视频解析和流式清理缺陷。本轮只撤销缺乏证据的参数改写，其余不扩大范围。

## Consequences

[请求体失败诊断](2026-10-17-relay-failure-diagnostics.md)保留首次读取失败，仍沿用本篇的实际字节限额、413 与分路由规则。

Messages 会受已有对话业务上限约束，部署时需要管理员核对保存值，避免遗留 20 MiB 意外限制 Messages；不修改默认或线上配置。撤销别名改写可能让只接受另一种字段的供应商拒绝请求，这是明确错误而非静默改变客户要求。第一项代码与真实流量证据缺失时不能宣布全部整改完成。

工具省略 content 保持省略，显式 null/空字符串不被隐式删除。三协议原生保真不保证供应商履约，但避免网关在证据不足时扩大承诺。验收矩阵将本地与真实环境拆分，代价是生产、模型能力与第一项假成功需要另行验收。

## Verification

- `go test -p 2 -timeout 120s ./...` 全仓通过；Go 1.25.1，GOMAXPROCS=2，GOPROXY=off。
- 参数保真、请求体限制、工具与三协议HTTP用例的定向 `go test -race` 通过。既有 `TestRelayResponsesInterruptedOutputRefundsAllFundingSources/wallet/client_canceled` 暴露的日志计数竞态由局部同步修复覆盖，包含该用例的定向竞态检查连续三轮通过；不宣称未执行的全仓竞态或生产负载验证通过。
- `NEW_API_LARGE_BODY_TEST=1` 下，192 MiB落盘与重复读取 SHA-256 一致；三协议路径经过本地HTTP中间层向模拟接收端发送完整192 MiB，摘要一致，超出1字节返回413且不发往接收端，缓存计数恢复。夹具不是视频解码或完整语义转发测试。
- gzip解压后的实际体积、未知/伪造Content-Length、较低全局上限、严格路由前缀、已保存20 MiB配置均有回归。新HTTP模拟测试与SQLite的Chat/Messages/Responses失败退款功能测试通过，不改变正常扣费或客户余额。
- 主前端Vite构建、改动文件Prettier检查通过；浏览器在模拟只读API下检查首页及`/docs`，页面渲染且无JavaScript异常。没有执行生图子项目构建、真实后台配置保存或生产页面验收。
- KVV未在当前仓库找到独立页面，未跑完整套件。伙伴的已知假成功识别代码未定位到，保留为待验收项。没有真实供应商压测或生产部署。
