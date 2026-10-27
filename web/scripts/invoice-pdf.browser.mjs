#!/usr/bin/env node
// Uses synthetic invoices only. Keep the production templates as the source of
// truth: AST extraction lets this test render them without mounting the console.
// Requires Chrome, Go, Python with PyMuPDF, pdfinfo and pdftoppm.
// INVOICE_PDF_VERIFY_SERVER=0 runs only the browser layout portion.
import assert from 'node:assert/strict';
import { readFile, mkdir, writeFile, mkdtemp, rm } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';
import { execFileSync } from 'node:child_process';
import vm from 'node:vm';
import puppeteer from 'puppeteer-core';
import { EPAY_CRYPTO_METHODS } from '../src/helpers/epayMethods.js';
import { createInvoiceTemplateTranslator } from '../src/helpers/invoiceTemplateLanguage.js';

const require = createRequire(import.meta.url);
const { parse } = require('@babel/parser');
const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const output = resolve(webRoot, '../output/pdf');
await mkdir(output, { recursive: true });
const source = await readFile(
  resolve(webRoot, 'src/components/topup/modals/TopupHistoryModal.jsx'),
  'utf8',
);
const ast = parse(source, { sourceType: 'module', plugins: ['jsx'] });
const lastTemplate = ast.program.body.find(
  (node) =>
    node.type === 'FunctionDeclaration' &&
    node.id.name === 'buildInvoiceServiceConfirmationHtml',
);
assert.ok(
  lastTemplate,
  'The current service confirmation template is available',
);
const declarations = ast.program.body.filter(
  (node) =>
    (node.type === 'FunctionDeclaration' && node.end <= lastTemplate.end) ||
    (node.type === 'VariableDeclaration' &&
      node.declarations.some((item) =>
        ['PAYMENT_METHOD_MAP', 'RECORD_TYPE_MAP'].includes(item.id.name),
      )),
);
const context = vm.createContext({
  Intl,
  Date,
  console,
  EPAY_CRYPTO_METHODS,
  i18n: { t: (text) => text, language: 'zh-CN', resolvedLanguage: 'zh-CN' },
  invoiceText: createInvoiceTemplateTranslator((text) => text),
});
vm.runInContext(
  declarations.map((node) => source.slice(node.start, node.end)).join('\n'),
  context,
);
const stamp = `data:image/png;base64,${(await readFile(resolve(webRoot, 'public/invoice-stamp.png'))).toString('base64')}`;
const timestamp = Math.floor(Date.UTC(2026, 9, 27, 1, 30) / 1000);
const fixture = (count, long = false, draft = false) => ({
  id: 987654,
  user_id: 123,
  username: 'PDF-QA-ONLY',
  display_name: '虚构测试账户',
  title: long
    ? '用于打印分页与长中文字段验证的虚构技术服务有限公司'
    : '虚构测试有限公司',
  status: draft ? 'pending' : 'invoiced',
  created_at: timestamp,
  reviewed_at: timestamp,
  total_money: count * 100,
  total_quota: count * 500000,
  source_type: long ? 'manual_transfer' : 'platform',
  items: Array.from({ length: count }, (_, index) => ({
    id: index + 1,
    order_id: index + 1,
    order_type: long ? 'manual_transfer' : 'topup',
    source_type: long ? 'manual_transfer' : 'platform',
    trade_no: `QA-ORDER-${String(index + 1).padStart(3, '0')}${long ? '-LONG-ABCDEFGHIJKLMNOPQRSTUVWXYZ-0123456789' : ''}`,
    product_name: long
      ? '人工核验的企业级人工智能接口技术服务额度'
      : 'AI API 额度',
    payer_name: long ? '用于排版核验的虚构付款方有限公司' : '虚构付款方',
    payee_name: '上海曜算智能科技有限公司',
    payment_method: long ? 'bank_transfer' : 'alipay',
    transfer_bank_name: long ? '虚构测试银行示例支行' : '',
    money: 100,
    amount: 500000,
    complete_time: timestamp + index,
  })),
  product_items: Array.from({ length: count }, (_, index) => ({
    product_name: `QA-PRODUCT-${String(index + 1).padStart(3, '0')}`,
    specification: long
      ? '用于确认长中文规格可换行、跨页后仍然清晰显示的虚构规格说明'
      : '标准额度',
    quantity: 1,
    unit: '项',
    unit_price: 100,
    money: 100,
    quota: 500000,
    service_start_at: timestamp,
    service_end_at: timestamp + 86400 * 30,
    remark: long
      ? '这是合成测试数据，不对应真实客户、真实交易或实际服务交付。'
      : '仅用于自动化验证',
  })),
});
const cases = [
  ['detail-normal', 'detail_bill', fixture(1)],
  ['service-normal', 'service_confirmation', fixture(1)],
  ['detail-long', 'detail_bill', fixture(50, true)],
  ['service-long', 'service_confirmation', fixture(50, true)],
  ['detail-draft', 'detail_bill', fixture(2, true, true)],
  ['service-draft', 'service_confirmation', fixture(2, true, true)],
];
const chrome =
  process.env.INVOICE_PDF_CHROME_PATH ||
  process.env.PUPPETEER_EXECUTABLE_PATH ||
  [
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    '/usr/bin/chromium',
    '/usr/bin/chromium-browser',
    '/usr/bin/google-chrome',
  ].find(existsSync);
assert.ok(
  chrome,
  'Set INVOICE_PDF_CHROME_PATH to an installed Chromium browser',
);
const browser = await puppeteer.launch({
  executablePath: chrome,
  headless: true,
  args: ['--no-sandbox'],
});
const results = [];
try {
  for (const [name, kind, invoice] of cases) {
    const landscape = kind === 'detail_bill';
    const official = invoice.status === 'invoiced';
    const html = context[
      landscape
        ? 'buildInvoicePrintHtml'
        : 'buildInvoiceServiceConfirmationHtml'
    ](invoice, official ? stamp : '', { official });
    await writeFile(resolve(output, `invoice-${name}.html`), html);
    const page = await browser.newPage();
    // Match the printable width, since @page margins are applied by printToPDF,
    // while emulateMediaType alone does not apply them to the viewport.
    await page.setViewport({
      width: Math.floor(((landscape ? 269 : 186) * 96) / 25.4),
      height: 1000,
      deviceScaleFactor: 1,
    });
    await page.emulateMediaType('print');
    await page.setContent(html, { waitUntil: 'load' });
    await page.evaluate(async () => {
      await document.fonts.ready;
      await Promise.all(Array.from(document.images, (img) => img.decode()));
    });
    const layout = await page.evaluate(() => {
      const certificate = document.querySelector('.certificate');
      const box = certificate.getBoundingClientRect();
      return {
        certificate: { width: box.width, height: box.height },
        images: Array.from(document.images, (img) => ({
          width: img.naturalWidth,
          height: img.naturalHeight,
        })),
        overflowing: Array.from(
          document.querySelectorAll(
            'table, th, td, .sign-box, .provider-seal, .sign-seal-image',
          ),
        )
          .map((el) => ({
            tag: el.tagName,
            text: el.textContent.slice(0, 45),
            left: el.getBoundingClientRect().left,
            right: el.getBoundingClientRect().right,
          }))
          .filter((el) => el.left < box.left - 1 || el.right > box.right + 1),
        watermark: Boolean(document.querySelector('.draft-watermark')),
      };
    });
    assert.equal(
      layout.watermark,
      !official,
      `${name}: draft/official watermark`,
    );
    assert.equal(
      layout.images.length,
      official ? 1 : 0,
      `${name}: official seal only`,
    );
    assert.ok(
      layout.images.every((img) => img.width > 0 && img.height > 0),
      `${name}: seal is decoded`,
    );
    const filename = resolve(output, `invoice-${name}.pdf`);
    await page.pdf({
      path: filename,
      preferCSSPageSize: true,
      printBackground: true,
      displayHeaderFooter: false,
    });
    await page.close();
    const info = execFileSync('pdfinfo', [filename], { encoding: 'utf8' });
    const size = info.match(/Page size:\s+([\d.]+) x ([\d.]+)/);
    assert.ok(size, `${name}: PDF page dimensions`);
    assert.equal(
      Number(size[1]) > Number(size[2]),
      landscape,
      `${name}: A4 orientation`,
    );
    assert.ok(
      Math.abs(Math.min(Number(size[1]), Number(size[2])) - 595.28) < 2,
      `${name}: A4 short edge`,
    );
    assert.ok(
      Math.abs(Math.max(Number(size[1]), Number(size[2])) - 841.89) < 2,
      `${name}: A4 long edge`,
    );
    const pages = Number(info.match(/Pages:\s+(\d+)/)[1]);
    const text = execFileSync(
      process.env.PYTHON || 'python3',
      [
        '-c',
        'import fitz,sys; doc=fitz.open(sys.argv[1]); print("\\n".join(page.get_text() for page in doc))',
        filename,
      ],
      { encoding: 'utf8' },
    );
    assert.match(
      text.replace(/\s/g, ''),
      /上海曜算智能科技有限公司/,
      `${name}: searchable Chinese text`,
    );
    assert.match(text, /100\.00/, `${name}: monetary value`);
    for (let row = 1; row <= invoice.items.length; row++) {
      const marker = `${landscape ? 'QA-ORDER' : 'QA-PRODUCT'}-${String(row).padStart(3, '0')}`;
      // Text extraction may wrap an identifier at a valid CSS line-break.
      assert.ok(
        text.replace(/\s/g, '').includes(marker),
        `${name}: all rows survive pagination (${marker})`,
      );
    }
    if (name.endsWith('-long'))
      assert.ok(pages > 1, `${name}: multipage fixture`);
    else if (name.endsWith('-normal'))
      assert.equal(pages, 1, `${name}: ordinary invoice fits one page`);
    assert.equal(
      text.includes('待审核'),
      !official,
      `${name}: draft text is retained`,
    );
    execFileSync('pdftoppm', [
      '-f',
      '1',
      '-l',
      '1',
      '-scale-to',
      '1600',
      '-png',
      '-singlefile',
      filename,
      resolve(output, `invoice-${name}-first`),
    ]);
    if (pages > 1)
      execFileSync('pdftoppm', [
        '-f',
        String(pages),
        '-l',
        String(pages),
        '-scale-to',
        '1600',
        '-png',
        '-singlefile',
        filename,
        resolve(output, `invoice-${name}-last`),
      ]);
    results.push({
      name,
      pages,
      size: [Number(size[1]), Number(size[2])],
      ...layout,
    });
  }
} finally {
  await browser.close();
}
await writeFile(
  resolve(output, 'invoice-pdf-verification.json'),
  JSON.stringify(results, null, 2) + '\n',
);
console.log(JSON.stringify(results, null, 2));
for (const result of results)
  assert.deepEqual(
    result.overflowing,
    [],
    `${result.name}: content fits inside the printable certificate`,
  );

if (process.env.INVOICE_PDF_VERIFY_SERVER !== '0') {
  const tempRoot = resolve(webRoot, '../tmp/pdfs');
  await mkdir(tempRoot, { recursive: true });
  const runnerDir = await mkdtemp(resolve(tempRoot, 'invoice-render-'));
  const runner = resolve(runnerDir, 'main.go');
  try {
    await writeFile(
      runner,
      `package main
import (
  "context"
  "fmt"
  "os"
  "strings"
  "github.com/QuantumNous/new-api/service"
)
func main() {
  for _, input := range os.Args[1:] {
    source, err := os.ReadFile(input)
    if err != nil { panic(err) }
    pdf, err := service.RenderInvoicePDF(context.Background(), string(source))
    if err != nil { panic(fmt.Sprintf("%s: %v", input, err)) }
    output := strings.TrimSuffix(input, ".html") + "-server.pdf"
    if err := os.WriteFile(output, pdf, 0600); err != nil { panic(err) }
  }
}
`,
    );
    execFileSync(
      'go',
      [
        'run',
        runner,
        ...cases.map(([name]) => resolve(output, `invoice-${name}.html`)),
      ],
      {
        cwd: resolve(webRoot, '..'),
        env: { ...process.env, INVOICE_PDF_CHROME_PATH: chrome },
        timeout: 180000,
        stdio: ['ignore', 'pipe', 'pipe'],
      },
    );
    const report = execFileSync(
      process.env.PYTHON || 'python3',
      [
        '-c',
        `
import fitz, json, sys
from pathlib import Path
output = Path(sys.argv[1])
results = []
for name in sys.argv[2:]:
    browser = fitz.open(output / ("invoice-" + name + ".pdf"))
    server = fitz.open(output / ("invoice-" + name + "-server.pdf"))
    assert len(browser) == len(server), name + ": page count differs"
    checks = []
    for index, (a, b) in enumerate(zip(browser, server)):
        check = {
            "page": index + 1,
            "same_dimensions": a.rect == b.rect,
            "same_text": a.get_text() == b.get_text(),
            "same_pixels": a.get_pixmap().samples == b.get_pixmap().samples,
        }
        assert all(value for key, value in check.items() if key != "page"), (name, check)
        checks.append(check)
    results.append({"name": name, "pages": len(server), "page_checks": checks})
print(json.dumps(results, indent=2))
`,
        output,
        ...cases.map(([name]) => name),
      ],
      { encoding: 'utf8' },
    );
    await writeFile(resolve(output, 'invoice-server-comparison.json'), report);
    const comparisons = JSON.parse(report);
    console.log(
      `Native browser and server PDF match exactly across ${comparisons.length} documents / ${comparisons.reduce((sum, item) => sum + item.pages, 0)} pages (dimensions, text and pixels).`,
    );
  } finally {
    await rm(runnerDir, { recursive: true, force: true });
  }
}
