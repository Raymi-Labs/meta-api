# Agent Note: MiMo 同步图标兼容与厂商回退

Status: implemented

## Problem

线上公开 pricing 中六个 MiMo 模型图标为 `XiaomiMiMo`，厂商图标为已支持的 `Xiaomi.color='#FF6900'`。当前安装的 @lobehub/icons 2.1.0 不含 XiaomiMiMo，公共渲染函数退回字母 Avatar；模型入口仅按字符串是否非空选择图标，无法继续使用有效厂商图标。没有图片 URL 请求，故障不是远端图片下载。

## Decision

在既有 getLobeHubIcon 入口兼容同步名称 XiaomiMiMo，复用已安装的 SiXiaomi 本地 SVG。为该入口增加可选厂商回退参数：优先可解析模型图标，其次厂商图标，均无法解析时保留原字母占位。目录卡片、详情头卡/固定栏、对比、详情侧栏和模型管理使用同一规则；已有合法图标变体、颜色、尺寸、懒加载行为不变，不改数据库或人工配置。

## Existing notes audit

- [模型目录](../../implemented/feature/2026-10-25-model-catalog.md)和[参考布局](../../implemented/feature/2026-10-26-reference-layout-alignment.md)：相关入口，继续复用图标位置和尺寸，不调整布局。
- [供应商一致性](../../implemented/bug-fix/2026-10-31-model-management-vendors.md)：复用现有供应商字段与权限，修复前端解析，不修改服务端归属。

## Alternatives considered

- 仅改线上模型 icon 可恢复当前记录，但下次同步可能写回不支持名称；保留数据，在共用渲染层兼容。
- 整体升级图标库可能获得新图标，但会扩大依赖及其他厂商组件变更范围；复用已打包小米 SVG，无新增图片请求。
- 仅添加别名能修复 MiMo，但其他不支持的模型图标仍遮住有效厂商图标；同时增加有界回退。

## Verification

- 线上只读 `/api/pricing` 核对六个 XiaomiMiMo 模型与一个未配置模型图标的 MiMo ASR，厂商图标正常存在；本地安装包没有 XiaomiMiMo 组件。
- 修复前 Chrome 回归明确失败：同步名称没有 SVG，只显示字母；修复后同一断言通过。
- Chrome 覆盖目录点击详情、固定栏、对比，实际侧栏/管理图标渲染，合法图标优先、链式变体/颜色/尺寸、未知/缺失回退与390px暗色无溢出；无页面错误。
- 图标注册/安装包2项、MiMo与Inception厂商识别26项回归和 ESLint 通过；前端全量构建含17语言及图片应用通过。预渲染因本机缺少 `/usr/bin/chromium` 按原脚本跳过，Chrome验收独立完成。公共 render.jsx 保留既有格式，避免混入无关格式化。

## Consequences

兼容名称展示小米品牌标志，不宣称重制 MiMo 独立商标；未收录的图标仍可能回退到厂商或字母。没有后台字段修改、外部资源热链或新依赖。

补充回退只处理未注册名称，动态模块加载失败继续沿用既有占位，不改变图标加载策略。发布不等于生产升级。
