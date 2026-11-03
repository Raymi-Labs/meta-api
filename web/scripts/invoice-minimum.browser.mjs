import assert from 'node:assert/strict';
import { mkdtemp, writeFile, rm } from 'node:fs/promises';
import { basename, dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createServer, loadConfigFromFile } from 'vite';
import puppeteer from 'puppeteer-core';

// Real application component with synthetic API data; no financial writes.
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const fixtureDir = await mkdtemp(join(root, '.invoice-minimum-fixture-'));
let server;
let browser;
try {
  await writeFile(
    join(fixtureDir, 'index.html'),
    '<div id="root"></div><script type="module" src="./fixture.jsx"></script>',
  );
  await writeFile(
    join(fixtureDir, 'fixture.jsx'),
    `
import React from 'react';
import { createRoot } from 'react-dom/client';
import TopupHistoryModal from '../src/components/topup/modals/TopupHistoryModal.jsx';
import { UserContext } from '../src/context/User/index.jsx';
import { API } from '../src/helpers/apiCore.js';
import i18n from '../src/i18n/i18n.js';
import '@douyinfe/semi-ui/dist/css/semi.min.css';
import '../src/index.css';
const user = {id:1,role:1,username:'invoice-minimum',quota:10000000};
const amounts = JSON.parse(new URLSearchParams(location.search).get('amounts'));
const records = amounts.map((money,i)=>({id:i+1,order_type:'topup',record_type:'topup',trade_no:'MINIMUM-'+i,money,amount:100,status:'success',payment_method:'alipay',complete_time:1793060000}));
window.__calls=[];
window.__fail=true;
API.get=async url=>({data:{success:true,data:{items:url.includes('eligible-orders')?records:[],total:url.includes('eligible-orders')?records.length:0}}});
API.post=async (url,body)=>{window.__calls.push({url,body});return {data:window.__fail?{success:false,message:'提交发票申请失败'}:{success:true,data:{id:42,...body,total_money:20,items:[]}}};};
await i18n.changeLanguage('zh-CN');
createRoot(document.getElementById('root')).render(<UserContext.Provider value={[{user},()=>{}]}><TopupHistoryModal visible initialTab="invoices" onCancel={()=>{}} t={(key,options)=>i18n.t(key,options)}/></UserContext.Provider>);
`,
  );
  const loaded = await loadConfigFromFile(
    { command: 'serve', mode: 'test' },
    join(root, 'vite.config.js'),
  );
  server = await createServer({
    ...loaded.config,
    configFile: false,
    root,
    plugins: loaded.config.plugins
      .flat()
      .filter((p) => p.name !== '@code-inspector/vite'),
    server: { host: '127.0.0.1', port: 0 },
    logLevel: 'error',
  });
  await server.listen();
  const base = `http://127.0.0.1:${server.httpServer.address().port}`;
  browser = await puppeteer.launch({
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    headless: true,
  });
  const errors = [];
  const click = async (page, text) => {
    await page.waitForFunction(
      (text) =>
        [...document.querySelectorAll('button')].some(
          (b) => b.textContent.trim() === text && !b.disabled,
        ),
      {},
      text,
    );
    await page.evaluate(
      (text) =>
        [...document.querySelectorAll('button')]
          .filter((b) => b.textContent.trim() === text && !b.disabled)
          .at(-1)
          .click(),
      text,
    );
  };
  const open = async (amounts, width = 1440) => {
    const page = await browser.newPage();
    page.on('pageerror', (e) => errors.push(e.message));
    await page.setViewport({ width, height: 1000 });
    await page.evaluateOnNewDocument(() => {
      localStorage.setItem('user', JSON.stringify({ id: 1, role: 1 }));
      localStorage.setItem(
        'status',
        JSON.stringify({
          quota_per_unit: 500000,
          price: 1,
          invoice_service_fee_rate: 0,
        }),
      );
      localStorage.setItem('site-language', 'zh-CN');
      localStorage.setItem('quota_per_unit', '500000');
    });
    await page.setRequestInterception(true);
    page.on('request', (r) =>
      new URL(r.url()).origin === base ? r.continue() : r.abort(),
    );
    await page.goto(
      `${base}/${basename(fixtureDir)}/index.html?amounts=${encodeURIComponent(JSON.stringify(amounts))}`,
    );
    await click(page, '申请发票');
    await page.waitForSelector('.semi-modal-body table tbody tr');
    return page;
  };
  const disabled = async (page, expected) =>
    page.waitForFunction(
      (expected) => {
        const button = [
          ...document.querySelectorAll('.semi-modal-footer button'),
        ]
          .filter((b) => b.textContent.trim() === '确定')
          .at(-1);
        return button && button.disabled === expected;
      },
      {},
      expected,
    );
  const selectRow = async (page, index) => {
    const rows = await page.$$('.semi-modal-body table tbody tr');
    const input = await rows[index].$('input[type="checkbox"]');
    await input.evaluate((input) => input.click());
  };
  const fill = async (page, selector, value) => {
    await page.waitForSelector(selector);
    await page.$eval(
      selector,
      (input, value) => {
        Object.getOwnPropertyDescriptor(
          HTMLInputElement.prototype,
          'value',
        ).set.call(input, value);
        input.dispatchEvent(new Event('input', { bubbles: true }));
      },
      value,
    );
  };
  for (const [amounts, allowed] of [
    [[19.99], false],
    [[20], true],
    [[20.01], true],
    [[10, 9.99], false],
    [[8.69, 8.69, 2.62], true],
    [[19.999999], false],
  ]) {
    const page = await open(amounts);
    await disabled(page, true);
    assert.ok(
      await page.evaluate(() =>
        document.body.innerText.includes(
          '每次发票申请金额合计最低为人民币 20 元',
        ),
      ),
    );
    for (let i = 0; i < amounts.length; i++) await selectRow(page, i);
    await disabled(page, !allowed);
    if (amounts.length === 1 && amounts[0] === 19.99)
      await page.screenshot({
        path: '/tmp/meta-api-invoice-minimum-desktop.png',
        fullPage: true,
      });
    await selectRow(page, 0);
    await disabled(page, true);
    assert.equal(await page.evaluate(() => window.__calls.length), 0);
    await page.close();
  }
  const page = await open([20], 390);
  await selectRow(page, 0);
  await fill(page, 'input[placeholder="请输入发票抬头"]', '申请测试');
  await fill(page, 'input[placeholder="企业抬头必填"]', 'TEST123');
  await fill(
    page,
    'input[placeholder="用于接收电子发票"]',
    'invoice@example.invalid',
  );
  await page.screenshot({
    path: '/tmp/meta-api-invoice-minimum-mobile.png',
    fullPage: true,
  });
  await click(page, '确定');
  await page.waitForFunction(() => window.__calls.length === 1);
  await disabled(page, false);
  assert.equal(
    await page.$eval('input[placeholder="请输入发票抬头"]', (e) => e.value),
    '申请测试',
  );
  await page.evaluate(() => (window.__fail = false));
  await click(page, '确定');
  await page.waitForFunction(() => window.__calls.length === 2);
  assert.deepEqual(await page.evaluate(() => window.__calls[1].body.orders), [
    { order_type: 'topup', id: 1 },
  ]);
  await page.close();

  const manual = await open([20]);
  await selectRow(manual, 0);
  await disabled(manual, false);
  await manual.$eval('input[placeholder="请输入发票抬头"]', (input) =>
    input.closest('.semi-modal').querySelector('.semi-select').click(),
  );
  await manual.waitForSelector('[role="option"]');
  await manual.evaluate(() =>
    [...document.querySelectorAll('[role="option"]')]
      .find((e) => e.textContent.trim() === '转账')
      .click(),
  );
  await manual.waitForSelector('input[type="number"]');
  await disabled(manual, true);
  for (const [value, blocked] of [
    ['19.99', true],
    ['20', false],
    ['20.01', false],
    ['', true],
  ]) {
    await fill(manual, 'input[type="number"]', value);
    await disabled(manual, blocked);
  }
  await fill(manual, 'input[type="number"]', '10');
  await click(manual, '新增转账记录');
  const inputs = await manual.$$('input[type="number"]');
  await inputs[1].evaluate((input) => {
    Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      'value',
    ).set.call(input, '10');
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
  await disabled(manual, false);
  await click(manual, '删除');
  await disabled(manual, true);
  assert.equal(await manual.evaluate(() => window.__calls.length), 0);
  await manual.close();
  assert.deepEqual(errors, []);
  console.log(
    'PASS: CNY minimum, combined decimal boundary, selection removal, source switching, manual transfer totals, mobile submission and failed-request retry.',
  );
} finally {
  await browser?.close();
  await server?.close();
  await rm(fixtureDir, { recursive: true, force: true });
}
