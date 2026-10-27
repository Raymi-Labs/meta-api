# Agent Note: 发票附件复用浏览器打印模板自动生成

Status: implemented

## Problem

明细账单与产品明细清单已具备完整浏览器打印模板，但审批要求管理员打印另存后上传。旧 html2canvas 截图方案在提交 6ec7214de 被移除：屏幕样式、图片切片与原生打印分页不同，不能重新使用。

## Decision

管理员审批或重发时，前端用现有两份 HTML 模板及同一订单详情快照，调用仅管理员可用的原生 Chromium PDF 渲染接口。返回 PDF 后复用现有上传、落盘与邮件接口。手动上传优先，重发优先复用已保存附件；缺失才自动生成。生成失败阻止审批/发送，保留输入并允许手动替换；不生成真实税务发票。

渲染器仅接收有长度上限的自包含 HTML，不接受 URL；独立无凭证浏览器上下文、禁止脚本和外部请求、超时与并发限制，使用 print 媒体、CSS @page、背景色并等待字体与内嵌印章。无需新数据字段或数据库迁移。Docker 运行层随版本安装 Chromium 与中文字体，独立二进制部署明确依赖。

## Existing capability and note audit

入口为 TopupHistoryModal → 管理员 invoice 路由 → controller/invoice_attachments.go → model.InvoiceEmailAttachments；数据使用发票申请时的交易/产品快照。后台 buildInvoiceDetailBillHTML 为未接入路由的旧模板，不作为新的生成源。

活跃笔记检索命中[全站语言](../feature/2026-10-25-site-language.md)（部分重叠，保留发票模板多语言与保护占位符约束）和[公开通知](../feature/2026-10-25-public-notification-bell.md)（仅含发票未读请求说明，与 PDF 无关）。本笔记不改变这些决定。

## Alternatives considered

- 恢复 canvas 截图：仅前端即可执行，但不能保留原生打印分页、可选文本和样式，已由历史失败证实不适合。
- 后台另写 PDF/HTML 模板：无需接收客户端 HTML，但形成重复模板，印章、布局、语言继续漂移；复用现有打印 HTML 更直接。
- 单独外部 PDF 服务：隔离明确，但新增部署及私密账单跨服务传输，不适合当前单服务发布流程。

## Verification

`bun web/scripts/invoice-pdf.browser.mjs` 使用当前模板构造六类合成文档，覆盖单页、50 行长表、正式印章、草稿水印；共 24 页经服务端白名单与 Chromium 渲染后，与同版本 Chrome 原生打印的尺寸、文字和像素逐页一致。首末及中间页视觉检查通过。明细列宽采用百分比，清单逐行分页、跨页重复表头，旋转印章保留边距。

`node --test web/scripts/invoice-pdf.test.mjs web/scripts/invoice-template-language.test.mjs web/scripts/site-language-resources.test.mjs` 覆盖选择项、上传/已存优先、缺失生成、生成中止与17语言。`INVOICE_PDF_INTEGRATION=1 go test ./service ./controller ./model ./common -run 'Invoice|Email' -count=1` 覆盖接口、附件持久化、邮件与真实浏览器隔离；相关路径的 Linux CI 执行同组检查。`bun web/scripts/invoice-send.browser.mjs` 挂载真实审核组件并模拟 API，验证自动生成后提交、失败保留输入再重试、取消选项、已存附件直接重发四个流程。完整前端/文档构建、Go 编译及笔记门禁通过。测试只使用合成申请及本机 SMTP，不发送客户邮件。

## Consequences

自动附件继续走原上传与保存契约，不改变账务模型或数据库。每实例两次并发、30秒超时，HTML请求上限2MiB、PDF上限10MiB；印章必须内嵌且可解码，页面脚本、远程/本地文件和主动元素被禁止。

Chromium 增加运行镜像体积与按需内存；字体跨系统仍有差异，统一模板使用可用的 Noto 字体优先。渲染接口与既有管理员 PDF 上传一样允许管理员提交自定义内容，不视为独立的账务真实性认证。发布不等于生产部署，真实客户邮件不作为测试。
