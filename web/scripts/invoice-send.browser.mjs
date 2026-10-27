import assert from 'node:assert/strict';
import { mkdtemp, writeFile, rm } from 'node:fs/promises';
import { basename, dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createServer, loadConfigFromFile } from 'vite';
import puppeteer from 'puppeteer-core';

// Mount the real component, stub only API I/O. This test never sends email.
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const fixtureDir = await mkdtemp(join(root, '.invoice-send-fixture-'));
let server;
let browser;
const errors = [];
try {
  await writeFile(
    join(fixtureDir, 'index.html'),
    '<!doctype html><html><body><div id="root"></div><script type="module" src="./fixture.jsx"></script></body></html>',
  );
  await writeFile(
    join(fixtureDir, 'invoice.pdf'),
    '%PDF-1.4\n% original issued invoice fixture\n%%EOF',
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
import '../src/index.css';
const scenario = new URLSearchParams(location.search).get('scenario');
const user = {id:1, role:100, username:'invoice-fixture'};
let record = {id:42,user_id:7,username:'test-user',title:'PDF 流程测试',email:'fixture@example.invalid',
status:scenario === 'resend' ? 'invoiced':'pending',invoice_type:'enterprise',
invoice_file_name:scenario === 'resend' ? 'invoice.pdf':'',
detail_bill_file_name:scenario === 'resend' ? 'saved-detail.pdf':'',
service_confirmation_file_name:scenario === 'resend' ? 'saved-service.pdf':'',
need_detail_bill:true,need_service_confirmation:true,total_money:100,total_quota:500000,
created_at:1793060000,items:[{id:1,order_id:1,order_type:'topup',trade_no:'fixture-order',money:100,quota:500000,complete_time:1793060000}],
product_items:[{product_name:'AI API',quantity:1,unit:'项',unit_price:100,money:100,quota:500000}]};
window.__invoiceCalls=[];
window.__invoiceFail = scenario === 'failure';
API.get = async url => {
  window.__invoiceCalls.push({method:'get',url});
  if (url.endsWith('/invoices/42')) return {data:{success:true,data:{...record}}};
  return {data:{success:true,data:{items:url.includes('/invoices') ? [record]:[],total:url.includes('/invoices') ? 1:0}}};
};
API.post = async (url, body, options) => {
  if (url.endsWith('/render-pdf')) {
    window.__invoiceCalls.push({method:'render',url,kind:body.kind,html:body.html,timeout:options.timeout});
    await new Promise(resolve => setTimeout(resolve, 30));
    if (window.__invoiceFail) return {data:new Blob([JSON.stringify({success:false,message:'自动生成失败测试'})],{type:'application/json'})};
    return {data:new Blob(['%PDF-1.4\\n'+body.kind],{type:'application/pdf'})};
  }
  const fields={};
  for (const [key,value] of body.entries()) fields[key] = value instanceof File ? {name:value.name,type:value.type,text:await value.text()} : value;
  window.__invoiceCalls.push({method:'submit',url,fields});
  record={...record,status:'invoiced',invoice_file_name:'invoice.pdf',invoice_send_status:'sent'};
  return {data:{success:true,data:record}};
};
await i18n.changeLanguage('zh-CN');
const t = (key, options) => i18n.t(key, options);
createRoot(document.getElementById('root')).render(<UserContext.Provider value={[{user},()=>{}]}><TopupHistoryModal visible={true} onCancel={()=>{}} initialTab="invoices" t={t}/></UserContext.Provider>);
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
    // The interactive inspector keeps a separate server alive after Vite closes.
    plugins: loaded.config.plugins
      .flat()
      .filter((plugin) => plugin.name !== '@code-inspector/vite'),
    server: { host: '127.0.0.1', port: 0, strictPort: false },
    logLevel: 'error',
  });
  await server.listen();
  const address = server.httpServer.address();
  const base = `http://127.0.0.1:${address.port}`;
  browser = await puppeteer.launch({
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    headless: true,
  });
  const open = async (scenario) => {
    const page = await browser.newPage();
    page.setDefaultTimeout(30000);
    await page.setViewport({ width: 1500, height: 1100 });
    page.on('pageerror', (error) => errors.push(error.message));
    await page.evaluateOnNewDocument(() => {
      localStorage.setItem(
        'user',
        JSON.stringify({ id: 1, role: 100, username: 'invoice-fixture' }),
      );
      localStorage.setItem('site-language', 'zh-CN');
      localStorage.setItem('i18nextLng', 'zh-CN');
      localStorage.setItem(
        'status',
        JSON.stringify({
          quota_per_unit: 500000,
          price: 1,
          invoice_service_fee_rate: 0,
        }),
      );
    });
    await page.setRequestInterception(true);
    page.on('request', (request) => {
      if (new URL(request.url()).origin === base) request.continue();
      else request.abort();
    });
    await page.goto(
      `${base}/${basename(fixtureDir)}/index.html?scenario=${scenario}`,
    );
    await click(page, scenario === 'resend' ? '重新发送' : '通过');
    return page;
  };
  async function click(page, text) {
    await page.waitForFunction(
      (text) =>
        [...document.querySelectorAll('button')].some(
          (button) => button.textContent.trim() === text && !button.disabled,
        ),
      {},
      text,
    );
    await page.evaluate(
      (text) =>
        [...document.querySelectorAll('button')]
          .filter(
            (button) => button.textContent.trim() === text && !button.disabled,
          )
          .at(-1)
          .click(),
      text,
    );
  }
  const upload = async (page) => {
    const inputs = await page.$$('input[type="file"]');
    await inputs[0].uploadFile(join(fixtureDir, 'invoice.pdf'));
  };
  const calls = (page) => page.evaluate(() => window.__invoiceCalls);
  const waitSubmit = (page) =>
    page.waitForFunction(() =>
      window.__invoiceCalls.some((call) => call.method === 'submit'),
    );

  const automatic = await open('automatic');
  await upload(automatic);
  await click(automatic, '确定');
  await waitSubmit(automatic);
  const autoCalls = await calls(automatic);
  assert.deepEqual(
    autoCalls
      .filter((call) => call.method === 'render')
      .map((call) => call.kind),
    ['detail_bill', 'service_confirmation'],
  );
  assert.equal(
    autoCalls.filter(
      (call) => call.method === 'get' && call.url.endsWith('/42'),
    ).length,
    1,
  );
  for (const call of autoCalls.filter((call) => call.method === 'render')) {
    assert.match(call.html, /data:image\/png;base64,/);
    assert.match(call.html, /@page/);
    assert.equal(call.timeout, 60000);
  }
  const autoSubmit = autoCalls.find((call) => call.method === 'submit');
  assert.equal(autoSubmit.url, '/api/user/invoices/42/approve');
  assert.equal(autoSubmit.fields.invoice_file.name, 'invoice.pdf');
  assert.equal(
    autoSubmit.fields.detail_bill_file.text,
    '%PDF-1.4\ndetail_bill',
  );
  assert.equal(
    autoSubmit.fields.service_confirmation_file.text,
    '%PDF-1.4\nservice_confirmation',
  );
  assert.equal(autoSubmit.fields.send_email, 'true');
  await automatic.close();

  const failure = await open('failure');
  await upload(failure);
  await click(failure, '确定');
  await failure.waitForFunction(() =>
    document.body.textContent.includes('自动生成失败测试'),
  );
  assert.equal(
    (await calls(failure)).filter((call) => call.method === 'submit').length,
    0,
  );
  assert.equal(
    await failure.$eval('input[type="file"]', (input) => input.files[0].name),
    'invoice.pdf',
  );
  assert.equal(
    await failure.$$eval(
      'fieldset input[type="checkbox"]',
      (inputs) => inputs.filter((input) => input.checked).length,
    ),
    3,
  );
  await failure.evaluate(() => {
    window.__invoiceFail = false;
  });
  await click(failure, '确定');
  await waitSubmit(failure);
  await failure.close();

  const unchecked = await open('unchecked');
  await upload(unchecked);
  await unchecked.evaluate(() => {
    const inputs = [
      ...document.querySelectorAll('fieldset input[type="checkbox"]'),
    ];
    for (const input of inputs.slice(1)) input.click();
  });
  await click(unchecked, '确定');
  await waitSubmit(unchecked);
  const uncheckedCalls = await calls(unchecked);
  assert.equal(
    uncheckedCalls.filter((call) => call.method === 'render').length,
    0,
  );
  const uncheckedFields = uncheckedCalls.find(
    (call) => call.method === 'submit',
  ).fields;
  assert.equal(uncheckedFields.send_detail_bill, 'false');
  assert.equal(uncheckedFields.send_service_confirmation, 'false');
  assert.equal(uncheckedFields.detail_bill_file, undefined);
  assert.equal(uncheckedFields.service_confirmation_file, undefined);
  await unchecked.close();

  const resend = await open('resend');
  await click(resend, '重新发送');
  await waitSubmit(resend);
  const resendCalls = await calls(resend);
  assert.equal(
    resendCalls.filter((call) => call.method === 'render').length,
    0,
  );
  const resendSubmit = resendCalls.find((call) => call.method === 'submit');
  assert.equal(resendSubmit.url, '/api/user/invoices/42/resend-email');
  assert.equal(resendSubmit.fields.send_detail_bill, 'true');
  assert.equal(resendSubmit.fields.send_service_confirmation, 'true');
  assert.equal(resendSubmit.fields.detail_bill_file, undefined);
  assert.equal(resendSubmit.fields.service_confirmation_file, undefined);
  await resend.close();
  assert.deepEqual(errors, []);
  console.log(
    'PASS: invoice approval auto-generation, retained input after failure and retry, deselection, and saved-attachment resend (mock API; no email sent).',
  );
} finally {
  await browser?.close();
  await server?.close();
  await rm(fixtureDir, { recursive: true, force: true });
}
