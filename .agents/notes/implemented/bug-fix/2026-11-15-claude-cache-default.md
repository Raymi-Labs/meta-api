# Agent Note: Claude 转换请求默认启用增量缓存

Status: implemented

## Problem

渠道 1508 支持缓存读写，但 OpenAI Chat Completions 客户端没有缓存断点时，旧配置默认关闭增量缓存，同一渠道出现原生 Messages 有缓存、转换请求没有缓存的差异。已有断点插入和用量解析可以复用，无需另建缓存系统。

## Decision

渠道设置使用可空布尔值：缺失或 null 默认开启，显式 false 保留关闭，显式 true 开启。前端新建、编辑、复制及保存缺失值统一默认 true。只影响 Claude 适配器的 Chat Completions 转换，原生 Messages 和其他渠道不插入断点。运行时默认策略保留显式关闭；本轮用户另外明确授权线上全部开启，故线上 23 个 Claude 渠道均设置 true（此前 20 个缺失、3 个开启、0 个显式关闭），其他 JSON 字段和渠道状态保持，依赖现有缓存刷新，不停服。

## Consequences

默认开启可能产生首次缓存创建费用，不承诺缓存命中；旧版若曾把 false 省略，无法恢复当时意图，缺失值按本次授权采用新默认。配置格式仍为可选 JSON 布尔值，无 schema 迁移。原生 Messages 和其他协议不受默认断点策略影响。

## Validation

`go test ./dto ./relay/channel/claude ./model ./controller`、前端 `bun run build`、前端产物完成后的 `go build` 和 `make verify-notes` 通过。测试覆盖缺失/null/true/false 编解码、显式 false 持久化、旧配置实际转换默认插入断点、关闭后不插入、原生请求不变。前端预渲染因本机缺少指定 Chromium 跳过，Vite 构建通过；未做浏览器交互验收。

生产 MySQL 临时表使用实际 23 条渠道设置演练相同 JSON 更新，23 条均开启、其他字段差异为 0，随后备份并事务更新全部 23 条。备份位于生产 `/root/claude-cache-enable-20261115.nlPxQJ/settings-before.tsv`；容器 ID、启动时间与重启数保持，21:11:03 渠道缓存完成刷新。1508 的新消费日志 17083906 带 `claude_incremental_cache=true`，确认现有线上程序已使用断点，但该条缓存读写为 0，不据此宣称命中或节费。只更新生产配置，不升级生产程序。

## Alternatives considered

只修改前端默认值不能覆盖存量及 API 新建；运行时强制全部开启会覆盖管理员明确关闭。复用三态配置避免数据库结构迁移，也避免 bool 的 omitempty 丢失 false 后再次被开启。线上全量开启是本轮独立授权操作，不替代后续显式关闭的语义。

## Related notes

已检索活跃笔记的“增量缓存 / incremental_cache”，没有此能力的归属笔记。其他缓存计费、TTL 与日志展示决策不改变，本次不覆盖其范围。
