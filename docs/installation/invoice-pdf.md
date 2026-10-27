# 发票明细与产品清单自动 PDF

管理员审核或重发发票邮件时，勾选的明细账单、产品明细清单可自动生成并随邮件发送。实际税务发票仍须上传。生成使用与预览、浏览器打印相同的 HTML 模板和 Chromium 原生打印，保留文本、打印样式与分页；不是截图转 PDF。已有保存附件可直接重发，手动上传可替换自动文件。

## 运行依赖

官方项目 Docker 镜像随版本安装 Chromium 与 Noto CJK 字体，不需要额外 PDF 服务。升级镜像后才具备该依赖，发布 GitHub Release 不会自动升级生产。

直接运行 Linux 服务端二进制的部署需要在同一主机安装浏览器与中文字体，例如 Debian/Ubuntu：

```sh
sudo apt-get update
sudo apt-get install -y chromium fonts-noto-cjk
export INVOICE_PDF_CHROME_PATH=/usr/bin/chromium
```

`INVOICE_PDF_CHROME_PATH` 是可选的浏览器可执行文件绝对路径；未配置时自动寻找系统 Chrome/Chromium。macOS/Windows 裸二进制同样需要可用的 Chrome/Chromium 与中文字体。字体随操作系统不同可能出现字形差异，版式使用同一打印模板。

浏览器每次生成独立启动，使用临时用户目录并在完成、取消或超时后清理，不读取管理员浏览器配置。每实例最多同时生成两份，单次上限 30 秒，请求体最多 2 MiB，生成 PDF 最多 10 MiB。需要预留浏览器进程的内存和可写临时目录；多实例部署的限制分别计数。以 root 运行时 Chromium 使用其无 sandbox 模式；应在应用容器隔离边界内运行，直接部署优先使用普通系统用户。

## 权限与失败处理

`POST /api/user/invoices/:id/render-pdf` 仅管理员可用，仅接受待审核或已开票申请。JSON 内容为 `{ "html": "…", "kind": "detail_bill" }`，另一种类型为 `service_confirmation`。它只生成 PDF，不审核申请、不保存附件、不发送邮件；既有审批/重发接口负责后续操作。

渲染器仅保留打印所需的 HTML 元素和属性，重建最前的 CSP，禁止页面脚本、导航、框架、外部网络和本地文件加载；印章仅接受内嵌 PNG/JPEG。禁止将远程地址或 SVG 当印章传入。浏览器失败日志不返回文档内容。此接口与管理员上传 PDF 一样允许管理员定制文件内容，不作为账务真实性校验。

没有浏览器、字体/图片损坏、超限或超时会阻止自动提交；管理员可以重试或上传手动打印的 PDF。邮件发送失败保留已保存的附件，之后直接重发，不需要重新生成。SMTP 接受邮件不等于收件人实际收到。

## 验证

```sh
go test ./service ./controller -run 'InvoicePDF|InvoiceAttachments' -count=1
INVOICE_PDF_INTEGRATION=1 go test ./service -run InvoicePDFBrowser -count=1 -v
```

第二条使用真实 Chrome/Chromium，验证打印纸张、可提取文本、脚本/网络隔离与取消。完整模板与服务器打印对照运行 `bun web/scripts/invoice-pdf.browser.mjs`（需要 Go、Chrome、Poppler 和安装 PyMuPDF 的 Python；可用 `PYTHON` 指定解释器）；审核/重发组件交互运行 `bun web/scripts/invoice-send.browser.mjs`。测试不连接真实 SMTP、不读取生产申请、不发送客户邮件。
