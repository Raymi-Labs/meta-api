# Agent Note: 图片工作台开箱即用的对话与生图分离默认配置

Status: implemented

## Problem

用户要求三个入口不必手工配置即可上手。当前托管入口只下发 Images profile，Agent 切换被 Responses 要求阻挡；即便改为文本模型，其原生 image_generation 工具也不能保证由独立图片分组执行。电商已有文本规划与图片分离流程。

## Decision

仅为系统图片工作台 profile 下发可用的 agentTextModel 元数据，沿用 gpt-6-sol。前端为 Agent 临时派生 Responses 文本配置，并保留 agentImageModel 指向原图片模型；不改用户保存的画廊配置。托管 Agent 仅提供应用执行的 generate_image_batch（允许一张）和 continue_generation 函数工具，图片复用 Images API 与专用组；不请求上游原生 image_generation；默认不开启 web_search，保留用户显式启用设置，后者仍取决于上游工具支持。自定义 Responses 配置保留原行为。文本可达性沿用服务端真实回退范围，无渠道不伪装可用。

应用执行生图须传递取消信号、参考图与遮罩；按尝试次数约束图片预算，失败不自动重发生图；相同工具ID和相同批次提示词在本轮去重；限制连续文本工具循环，限制并发为临时令牌允许的2。切换与重试保留图片任务的真实模型，画廊/电商图片继续走 Images。所有调用仅在用户提交后发起。

## Alternatives considered

- 只改默认 apiMode 为 Responses：图片会借道文本上游的原生工具，无法守住图片组优先。
- 多个用户可见 API profile 自动切换：可实现但增添配置噪声和旧任务凭据覆盖风险；采用同凭据的模式内派生配置。
- 新增服务端 Agent 编排：可集中执行，但当前已有前端工具循环和图片存储，先复用以保持范围。

活跃笔记审计：[画廊路由](../../implemented/bug-fix/2026-11-02-gallery-media-routing.md)部分重叠，保留分组与权限规则；[旧会话默认](../../implemented/bug-fix/2026-11-01-image-playground-defaults.md)保留历史理由；其他视频/健康/界面笔记无关。

## Verification

Go 会话入口定向回归通过（`go test -p 2 ./controller -run ImagePlayground -count=1 -timeout=120s`），验证文本可达才下发默认 Agent 模型。图片子应用 19 个测试文件、207 项 Vitest 回归通过，包含托管配置重新导入/撤销、普通对话不生图、模式切换及真实图片任务模型、文本函数→Images 生成/参考图编辑→工具结果回传、遮罩字段、取消、失败不重试、工具去重、尝试预算、工具空转上限和两张并发。已有自定义 Responses 与电商规划测试继续通过。

TypeScript 与 Vite 子应用生产构建通过，提交同步静态产物。`node web/scripts/image-workspace-defaults.browser.mjs` 用真实 Chrome 和生产构建，完全拦截为本地合成接口，验证 Agent 不需设置即可进入、文本→生图→结果回传、回画廊、电商入口、390px 无横向溢出及停止。初次手机测试点到隐藏桌面按钮，修改测试选择可见按钮后通过，未改产品布局；遮罩单测缺少 Node Canvas，显式模拟像素转换后验证真实 multipart 请求字段，不将其视为浏览器像素转换验收。

语言覆盖门禁 805 个源 key、16 个目标资源通过，静态产物门禁通过；`make verify-notes`（215 篇笔记）与 `git diff --check` 通过。浏览器/测试服务在 finally 中关闭，测试进程核对为零；无真实收费模型请求。

## Consequences

不能从模型名称推断真实供应商支持 Responses、工具和视觉输入，离线合成通过不等于真实任务验收。自动编排会消耗文本及图片额度，遵守用户发起请求和有界预算；取消不保证上游已接受的计费任务退款。已有用户需重新进入工作台获取新启动配置。
