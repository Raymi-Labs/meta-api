import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';
import { createServer, loadConfigFromFile } from 'vite';
import { stop as stopEsbuild } from 'esbuild';

const root = fileURLToPath(new URL('..', import.meta.url));
const name = `.model-discount-qa-${process.pid}`;
const files = ['html', 'jsx'].map((ext) => path.join(root, `${name}.${ext}`));
const artifacts = fs.mkdtempSync(path.join(os.tmpdir(), 'model-discount-'));
let server, browser;
const errors = [],
  writes = [];
let failSave = false;
let saved = [];
try {
  const { config } = await loadConfigFromFile(
    { command: 'serve', mode: 'development' },
    path.join(root, 'vite.config.js'),
  );
  server = await createServer({
    ...config,
    configFile: false,
    root,
    server: { host: '127.0.0.1', port: 0, open: false, proxy: {}, hmr: false },
    plugins: config.plugins
      .flat(Infinity)
      .filter((p) => p?.name !== '@code-inspector/vite'),
  });
  fs.writeFileSync(
    files[0],
    `<!doctype html><html><meta name="viewport" content="width=device-width, initial-scale=1"><div id="root"></div><script type="module" src="/${name}.jsx"></script></html>`,
  );
  fs.writeFileSync(
    files[1],
    `import React,{useState} from 'react';import {createRoot} from 'react-dom/client';import {MemoryRouter} from 'react-router-dom';import ModelDiscountSettings from './src/pages/Setting/Ratio/ModelDiscountSettings';import ModelGroupRows from './src/pages/ModelDetail/ModelGroupRows';import {API} from './src/helpers';import {initialLocaleReady} from './src/i18n/i18n';import '@douyinfe/semi-ui/dist/css/semi.css';import './src/index.css';import './src/components/table/model-pricing/layout/pricingCatalog.css';import './src/pages/ModelDetail/modelDetail.css';await initialLocaleReady;
const model={model_name:'gpt-luna',quota_type:0,model_ratio:2,completion_ratio:5,supports_cache_read:true,cache_ratio:0.1,enable_groups:['三折','免费']};const money=Object.assign(v=>'$'+v,{currencySymbol:'$',toAmount:v=>v});
function App(){const [options,setOptions]=useState({NoDiscountModels:'[]',ModelRatio:'{"gpt-luna":2,"other":1}'});const refresh=async()=>{const {data}=await API.get('/api/option/');setOptions(data.data)};const noDiscount=JSON.parse(options.NoDiscountModels).includes('gpt-luna');return <MemoryRouter><main style={{padding:24,maxWidth:1000,margin:'auto'}}><ModelDiscountSettings options={options} refresh={refresh}/><ModelGroupRows model={{...model,no_discount:noDiscount}} groups={model.enable_groups} selectedGroup="all" data={{t:s=>s,groupRatio:{'三折':0.3,'免费':0},timeRatioMap:{'gpt-luna':{'三折':{ratio:0.5}}},currency:'USD',siteDisplayType:'USD',tokenUnit:'M',displayPrice:money,copyText:()=>{}}} health={{data:{groups:[]},isAuthenticated:true}}/></main></MemoryRouter>}createRoot(document.getElementById('root')).render(<App/>);`,
  );
  await server.listen();
  const base = `http://127.0.0.1:${server.httpServer.address().port}`;
  browser = await puppeteer.launch({
    headless: true,
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  });
  const page = await browser.newPage();
  page.on('pageerror', (e) => errors.push(e.message));
  await page.setViewport({ width: 1280, height: 1000 });
  await page.evaluateOnNewDocument(() => {
    localStorage.setItem('site-language', 'zh-CN');
    localStorage.setItem('quota_per_unit', '500000');
    localStorage.setItem('user', JSON.stringify({ id: 1, role: 100 }));
  });
  await page.setRequestInterception(true);
  page.on('request', async (req) => {
    const url = new URL(req.url());
    if (url.origin !== base) {
      await req.abort();
      return;
    }
    if (!url.pathname.startsWith('/api/')) {
      await req.continue();
      return;
    }
    let response = { success: true, data: null };
    if (url.pathname === '/api/option/' && req.method() === 'PUT') {
      const payload = JSON.parse(req.postData());
      writes.push(payload);
      if (failSave) response = { success: false, message: '模拟保存失败' };
      else saved = JSON.parse(payload.value);
    } else if (url.pathname === '/api/option/')
      response = {
        success: true,
        data: {
          NoDiscountModels: JSON.stringify(saved),
          ModelRatio: '{"gpt-luna":2,"other":1}',
        },
      };
    await req.respond({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(response),
    });
  });
  await page.goto(base + '/' + name + '.html', { waitUntil: 'networkidle0' });
  const prices = () =>
    page.$$eval('[data-price-key="input"] strong', (els) =>
      els.map((e) => e.textContent),
    );
  assert.deepEqual(await prices(), ['$0.6000', '$0.0000']);
  await page.click('.semi-select');
  await page.waitForSelector('.semi-select-option');
  await page.evaluate(() =>
    [...document.querySelectorAll('.semi-select-option')]
      .find((e) => e.textContent === 'gpt-luna')
      .click(),
  );
  await page.keyboard.press('Escape');
  const clickSave = () =>
    page.evaluate(() =>
      [...document.querySelectorAll('button')]
        .find((e) => e.textContent === '保存')
        .click(),
    );
  failSave = true;
  await clickSave();
  await page.waitForFunction(() =>
    document.body.innerText.includes('模拟保存失败'),
  );
  assert.deepEqual(await prices(), ['$0.6000', '$0.0000']);
  failSave = false;
  await clickSave();
  await page.waitForFunction(
    () =>
      document.querySelector('[data-price-key="input"] strong')?.textContent ===
      '$4.0000',
  );
  assert.deepEqual(await prices(), ['$4.0000', '$4.0000']);
  assert.deepEqual(writes.at(-1), {
    key: 'NoDiscountModels',
    value: '["gpt-luna"]',
  });
  assert.ok(
    await page.evaluate(() =>
      document.body.innerText.includes('不参与折扣，按模型基础价计费'),
    ),
  );
  await page.waitForFunction(
    () => !document.querySelector('.semi-toast-content'),
    { timeout: 15000 },
  );
  await page.screenshot({
    path: path.join(artifacts, 'desktop.png'),
    fullPage: true,
  });
  await page.setViewport({ width: 390, height: 844 });
  await page.screenshot({
    path: path.join(artifacts, 'mobile.png'),
    fullPage: true,
  });
  assert.ok(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  );
  assert.deepEqual(errors, []);
  console.log(JSON.stringify({ passed: true, writes, artifacts }));
} finally {
  await browser?.close();
  await server?.close();
  stopEsbuild();
  files.forEach((file) => fs.rmSync(file, { force: true }));
}
