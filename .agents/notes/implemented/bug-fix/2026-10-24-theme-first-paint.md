# Agent Note: 首帧主题与 React 状态一致

Status: implemented

## Problem

现有 ThemeProvider 已支持 light/dark/auto、布尔参数兼容与系统主题变化，但要等 React effect 才更新 html.dark 和 Semi 的 body[theme-mode]。慢网或入口脚本尚未执行时，深色用户先看到浅色画布；损坏的持久化值被当成主题，且存储写入失败会让切换抛错。

## Decision

在 HTML head 用同步小脚本读取现有 theme-mode 键，规范化为 light/dark/auto，并在首帧前应用 html.dark 与 color-scheme；body 开始处立即同步 Semi 属性。React 复用同一规范化语义并容忍存储不可用，继续监听系统主题。脚本不依赖额外网络请求，不写入用户偏好、不改品牌元信息、不新增存储键。

## Existing capabilities and scope

检索活跃主题/首帧笔记：流式首帧属于无关能力；下载入口笔记明确公开下载内容保留浅色设计，本次不改其样式。沿用 ThemeProvider 和已有公开页/控制台主题按钮，只增加初始化与独立回归；不更换主题组件或引入新状态库。

## Alternatives considered

- 只提前 React effect：改动少，但下载/解析入口模块之前仍无主题，不能覆盖慢网首帧。
- 额外经典外链主题脚本：可与 React 共用实现，但首帧需再等待网络与缓存，增加阻塞请求；选择内联小脚本，自动回归约束它与运行时的语义一致。
- 迁移上游存储键和 system 值：能直接套用上游实现，却丢失本地用户偏好并破坏现有 auto/布尔调用约定，不采用。

## Verification

- `cd web && node --test scripts/theme-init.test.mjs`：19 项通过，涵盖亮/暗/auto、空/缺失/坏值与系统主题矩阵；内联和 React helper 语义一致；读取/写入失败、无 matchMedia、无 DOM、旧版监听器及取消监听均覆盖。预渲染遗留深色属性在浅色启动时清除，不碰无关 class。
- `node scripts/theme-init-browser.test.mjs`：真实项目 Vite + Chromium，拦住入口模块后检查首帧，覆盖 8 种配置；释放模块并确认 React 真正替换根节点后再核对主题。坏值显示自动、存储读写失败仍可切换；自动跟随系统、手动覆盖、移除测试种子后的真实刷新持久化、返回自动均通过。API 使用合成数据，无后端写入。
- `bunx vite build` 与显式系统 Chrome 的 `PUPPETEER_EXECUTABLE_PATH='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' node scripts/prerender.mjs` 通过；五页实际产出，无跳过。再以 4174 preview 执行 `THEME_TEST_URL=http://127.0.0.1:4174 THEME_TEST_PRERENDERED=1 node scripts/theme-init-browser.test.mjs`：同一矩阵与五页预渲染首帧全部通过，验证脚本留存且在入口/内容前，亮色预渲染也能按深色用户偏好启动。
- 目视复核开发首帧亮/暗画布、生产预渲染深色登录页与下载页，截图留于忽略目录 `output/playwright/theme-init/`。下载正文继续浅色设计。改动 JS/JSX 的 ESLint、独立脚本的 Prettier、`git diff --check` 通过；HTML 保留原元信息字节格式，不顺带格式化其原有两个长行。

## Consequences

主题在 React/额外网络请求前即可生效；继续保留 theme-mode/auto 与布尔切换兼容，存储失败时只保证当前页面内切换，不承诺刷新持久化。内联与运行时存在少量规范化逻辑重复，改枚举/键/DOM 契约时必须重跑同一矩阵；若将来已有无额外请求的共享构建机制再重访去重。严格 CSP 禁止内联脚本的部署需维持相应策略支持。这不是全站 localStorage/matchMedia 故障治理，其他业务依赖不在本次范围；无 matchMedia 的真实浏览器验收只确认初始化降级，不声称整站第三方组件可运行。公开下载页自身固定浅色背景继续保留，生产部署另行验证。
