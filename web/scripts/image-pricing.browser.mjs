// All API traffic is synthetic; no generation or production configuration writes.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createServer, loadConfigFromFile } from 'vite';
import { stop as stopEsbuild } from 'esbuild';
import puppeteer from 'puppeteer-core';

const root = fileURLToPath(new URL('..', import.meta.url));
const fixture = await fs.mkdtemp(path.join(root, '.image-price-qa-'));
const output = await fs.mkdtemp(path.join(os.tmpdir(), 'image-price-qa-'));
let server, browser;
try {
  await fs.writeFile(
    path.join(fixture, 'index.html'),
    '<html><div id="root"></div><script type="module" src="./entry.jsx"></script></html>',
  );
  await fs.writeFile(
    path.join(fixture, 'entry.jsx'),
    `
import React,{useState} from 'react';import {createRoot} from 'react-dom/client';import {BrowserRouter} from 'react-router-dom';import i18n from 'i18next';import {initReactI18next} from 'react-i18next';
import Editor from '/src/pages/Setting/Ratio/components/ModelPricingEditor';import {UserContext} from '/src/context/User';import {StatusContext} from '/src/context/Status';import '/src/index.css';
i18n.use(initReactI18next).init({lng:'zh',resources:{zh:{translation:{}}},interpolation:{escapeValue:false}});
const initial={ModelPrice:'{"qwen-image-3.0-pro":0.9,"wan2.7-image-pro":0.1,"z-image-turbo":0.02}',ModelRatio:'{"text-model":1}'};
window.savedOptions=initial;function App(){const [options,setOptions]=useState(initial),[version,setVersion]=useState(0);window.qaReload=()=>{setOptions({...window.savedOptions});setVersion(v=>v+1)};
return <BrowserRouter><UserContext.Provider value={[{user:{id:901,role:100}},()=>{}]}><StatusContext.Provider value={[{status:{quota_per_unit:500000}},()=>{}]}><Editor key={version} options={options} refresh={async()=>{setOptions({...window.savedOptions});window.qaSaves=(window.qaSaves||0)+1}}/></StatusContext.Provider></UserContext.Provider></BrowserRouter>};createRoot(document.getElementById('root')).render(<App/>);`,
  );
  const { config } = await loadConfigFromFile(
    { command: 'serve', mode: 'development' },
    path.join(root, 'vite.config.js'),
  );
  server = await createServer({
    ...config,
    configFile: false,
    root,
    server: { host: '127.0.0.1', port: 0, open: false, proxy: {} },
    plugins: [
      ...config.plugins
        .flat(Infinity)
        .filter((p) => p?.name !== '@code-inspector/vite'),
      {
        name: 'deny-image-qa-api',
        configureServer(vite) {
          vite.middlewares.use((req, res, next) => {
            if (/^\/(api|v1)\//.test(req.url || '')) {
              res.statusCode = 502;
              res.end('unmocked API blocked');
            } else next();
          });
        },
      },
    ],
  });
  await server.listen();
  const base = 'http://127.0.0.1:' + server.httpServer.address().port;
  browser = await puppeteer.launch({
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    headless: true,
  });
  const page = await browser.newPage();
  await page.setViewport({ width: 1440, height: 1000 });
  const errors = [],
    writes = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.evaluateOnNewDocument(() => {
    localStorage.setItem('user', JSON.stringify({ id: 901, role: 100 }));
    localStorage.setItem('quota_per_unit', '500000');
  });
  await page.setRequestInterception(true);
  page.on('request', async (req) => {
    const url = new URL(req.url());
    if (url.origin !== base) {
      errors.push('external request blocked');
      return req.abort();
    }
    if (!/^\/(api|v1)\//.test(url.pathname)) return req.continue();
    if (req.method() === 'PUT') {
      assert.equal(url.pathname, '/api/option/');
      const data = JSON.parse(req.postData());
      writes.push(data);
      await page.evaluate((data) => {
        window.savedOptions[data.key] = data.value;
        if (data.key === 'billing_setting.tiered_bundle') {
          const b = JSON.parse(data.value);
          window.savedOptions['billing_setting.billing_mode'] = JSON.stringify(
            b.billing_mode,
          );
          window.savedOptions['billing_setting.billing_expr'] = JSON.stringify(
            b.billing_expr,
          );
        }
      }, data);
    } else assert.equal(req.method(), 'GET');
    await req.respond({
      status: 200,
      contentType: 'application/json',
      body: '{"success":true,"data":[]}',
    });
  });
  const click = async (text) => {
    await page.waitForFunction(
      (text) =>
        Array.from(document.querySelectorAll('button,label')).some(
          (b) => b.textContent.trim() === text,
        ),
      {},
      text,
    );
    await page.evaluate(
      (text) =>
        Array.from(document.querySelectorAll('button,label'))
          .find((b) => b.textContent.trim() === text)
          .click(),
      text,
    );
  };
  await page.goto(base + '/' + path.basename(fixture) + '/index.html', {
    waitUntil: 'networkidle0',
  });
  const fill = async (label, value) =>
    page.$eval(
      'input[aria-label="' + label + '"]',
      (input, value) => {
        const set = Object.getOwnPropertyDescriptor(
          HTMLInputElement.prototype,
          'value',
        ).set;
        set.call(input, value);
        input.dispatchEvent(new Event('input', { bubbles: true }));
        input.dispatchEvent(new Event('change', { bubbles: true }));
      },
      value,
    );
  await click('qwen-image-3.0-pro');
  assert(
    (await page.evaluate(() => document.body.innerText)).includes('¥0.02'),
  );
  assert.equal(writes.length, 0);
  await click('图片输入 / 输出计费');
  await page.waitForSelector('input[aria-label="1K-input"]');
  await click('应用更改');
  await page.waitForFunction(() =>
    document.body.innerText.includes('请填写完整的价格后再保存'),
  );
  assert.equal(writes.length, 0, 'incomplete prices must not be written');
  for (const [key, value] of [
    ['1K-input', '0.01'],
    ['1K-output', '0.1'],
    ['2K-input', '0.02'],
    ['2K-output', '0.2'],
  ])
    await fill(key, value);
  await page.screenshot({
    path: path.join(output, 'desktop.png'),
    fullPage: true,
  });
  assert.equal(writes.length, 0, 'reference and edited prices never auto-save');
  await click('应用更改');
  await page.waitForFunction(() => window.qaSaves >= 1);
  const expr = await page.evaluate(
    () =>
      JSON.parse(window.savedOptions['billing_setting.billing_expr'])[
        'qwen-image-3.0-pro'
      ],
  );
  assert(expr.includes('image_config_preview'));
  const catalog = await page.evaluate(async (expr) => {
    const { getModelCatalogPricing } = await import(
      '/src/helpers/modelCatalogPricing.js'
    );
    return getModelCatalogPricing(
      {
        record: {
          model_name: 'qwen-image-3.0-pro',
          billing_mode: 'tiered_expr',
          billing_expr: expr,
          enable_groups: ['default'],
        },
        selectedGroup: 'default',
        groupRatio: { default: 2 },
        displayPrice: { currencySymbol: '$', toAmount: (v) => v },
        tokenUnit: 'M',
        currency: 'USD',
      },
      (v) => v,
    ).items;
  }, expr);
  assert.equal(catalog.length, 4);
  assert.equal(catalog[0].value, '$0.0200');
  assert.equal(catalog[0].suffix, '/张');
  await page.evaluate(() => window.qaReload());
  await click('qwen-image-3.0-pro');
  await page.waitForSelector('input[aria-label="2K-output"]');
  assert.equal(
    await page.$eval('input[aria-label="2K-output"]', (e) => e.value),
    '0.2',
  );
  await fill('1K-input', '0');
  await click('应用更改');
  await page.waitForFunction(() => window.qaSaves >= 2);
  assert(
    (
      await page.evaluate(
        () => window.savedOptions['billing_setting.billing_expr'],
      )
    ).includes(' * 0 + '),
  );
  await page.setViewport({ width: 390, height: 1000 });
  await page.waitForFunction(() => document.documentElement.scrollWidth <= 390);
  await page.screenshot({
    path: path.join(output, 'mobile.png'),
    fullPage: true,
  });
  for (const model of ['wan2.7-image-pro', 'z-image-turbo']) {
    await click(model);
    assert.equal(await page.$('input[aria-label="1K-input"]'), null);
    assert(
      !(await page.evaluate(() => document.body.innerText)).includes(
        '视频按秒 / 表达式',
      ),
    );
    assert(
      (await page.evaluate(() => document.body.innerText)).includes(
        '图片官方参考价',
      ),
    );
  }
  assert.equal(
    await page.evaluate(
      () => JSON.parse(window.savedOptions.ModelRatio)['text-model'],
    ),
    1,
  );
  assert.deepEqual(errors, []);
  console.log(
    'PASS image pricing: reference isolation, incomplete-save block, edit/save/reopen/zero, group prices, Wan/Z, desktop/mobile. Artifacts:',
    output,
  );
} catch (error) {
  if (browser) {
    const pages = await browser.pages();
    await pages
      .at(-1)
      ?.screenshot({ path: path.join(output, 'failure.png'), fullPage: true });
    console.log('Failure screenshot:', output);
    console.log(
      await pages.at(-1)?.evaluate(() => ({
        text: document.body.innerText.slice(-1500),
        saved: window.savedOptions['billing_setting.billing_expr'],
        expr: Array.from(document.querySelectorAll('textarea')).map(
          (e) => e.value,
        ),
      })),
    );
  }
  throw error;
} finally {
  if (browser) await browser.close();
  if (server) await server.close();
  stopEsbuild();
  await fs.rm(fixture, { recursive: true, force: true });
  console.log('OWNED_BROWSER_AND_SERVER_CLOSED=yes');
}
