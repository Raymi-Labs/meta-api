import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';
import { createServer, loadConfigFromFile } from 'vite';
import { stop as stopEsbuild } from 'esbuild';
import { SITE_LANGUAGES } from '../src/helpers/localeNormalize.js';
const root = fileURLToPath(new URL('..', import.meta.url));
const seed = JSON.parse(
  fs.readFileSync(
    path.join(root, '../model/data/model_description_translations.json'),
    'utf8',
  ),
)[0];
const { config } = await loadConfigFromFile(
  { command: 'serve', mode: 'development' },
  path.join(root, 'vite.config.js'),
);
const server = await createServer({
  ...config,
  configFile: false,
  root,
  server: { host: '127.0.0.1', port: 0, open: false, proxy: {} },
  plugins: config.plugins
    .flat(Infinity)
    .filter((p) => p?.name !== '@code-inspector/vite'),
});
await server.listen();
const base = `http://127.0.0.1:${server.httpServer.address().port}`;
const name = `.model-description-qa-${process.pid}`;
const files = ['html', 'jsx'].map((ext) => path.join(root, `${name}.${ext}`));
const artifacts = fs.mkdtempSync(
  path.join(os.tmpdir(), 'model-description-browser-'),
);
const item = {
  id: 1,
  model_name: 'gpt-6-astra',
  display_name: 'GPT-6 Astra',
  description: seed.source,
  description_translations: seed.translations,
  description_automatic: seed.translations,
  description_overrides: {},
  name_rule: 0,
  status: 1,
  sync_official: 1,
  vendor_id: 1,
  icon: 'OpenAI',
  quota_type: 0,
  model_ratio: 10,
  completion_ratio: 5,
  enable_groups: ['default'],
  supported_endpoint_types: ['openai'],
  context_length: 1050000,
  max_output_tokens: 128000,
  input_modalities: ['text', 'image'],
  output_modalities: ['text'],
};
const status = {
  system_name: '元衡 API',
  server_address: base,
  quota_per_unit: 500000,
  usd_exchange_rate: 1,
  price: 1,
  quota_display_type: 'CNY',
};
let browser;
const writes = [];
const errors = [];
let external = 0;
let pricingReads = 0;
try {
  fs.writeFileSync(
    files[0],
    `<html><meta name="viewport" content="width=device-width, initial-scale=1"><div id="root"></div><script type="module" src="/${name}.jsx"></script></html>`,
  );
  fs.writeFileSync(
    files[1],
    `import React from 'react';import {createRoot} from 'react-dom/client';import {MemoryRouter,Routes,Route,Link} from 'react-router-dom';import Pricing from './src/pages/Pricing';import ModelDetail from './src/pages/ModelDetail';import EditModelModal from './src/components/table/models/modals/EditModelModal';import {UserContext} from './src/context/User';import {StatusContext} from './src/context/Status';import {ThemeProvider} from './src/context/Theme';import {changeSiteLanguage,SITE_LANGUAGES} from './src/helpers/siteLanguage';import {initialLocaleReady} from './src/i18n/i18n';import './src/index.css';function App(){const[open,setOpen]=React.useState(false);return <><div style={{padding:16,display:'flex',gap:16,flexWrap:'wrap',borderBottom:'1px solid #eee'}}><b>元衡 API · 本地多语言验收</b><select data-site-language defaultValue="zh-CN" onChange={e=>changeSiteLanguage(e.target.value)}>{SITE_LANGUAGES.map(([code,label])=><option key={code} value={code}>{label}</option>)}</select><Link to="/pricing">模型广场</Link><button onClick={()=>setOpen(true)} data-edit>模型管理</button></div><Routes><Route path="/pricing" element={<Pricing/>}/><Route path="/model/:name" element={<ModelDetail/>}/></Routes><EditModelModal visiable={open} editingModel={{id:1,model_name:'gpt-6-astra'}} handleClose={()=>setOpen(false)} refresh={()=>{}}/></>};await initialLocaleReady;createRoot(document.getElementById('root')).render(<MemoryRouter initialEntries={['/pricing']}><UserContext.Provider value={[{user:{id:8,role:100,username:'fixture',group:'default'}},()=>{}]}><StatusContext.Provider value={[{status:${JSON.stringify(status)}},()=>{}]}><ThemeProvider><App/></ThemeProvider></StatusContext.Provider></UserContext.Provider></MemoryRouter>);`,
  );
  browser = await puppeteer.launch({
    headless: true,
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  });
  const page = await browser.newPage();
  await page.setViewport({ width: 1440, height: 1000 });
  page.on('pageerror', (e) => errors.push(e.message));
  await page.evaluateOnNewDocument((status) => {
    localStorage.setItem('site-language', 'zh-CN');
    localStorage.setItem('status', JSON.stringify(status));
    localStorage.setItem(
      'user',
      JSON.stringify({
        id: 8,
        role: 100,
        username: 'fixture',
        group: 'default',
      }),
    );
  }, status);
  await page.setRequestInterception(true);
  page.on('request', async (r) => {
    const u = new URL(r.url());
    if (u.origin !== base) {
      external++;
      return r.abort();
    }
    if (!u.pathname.startsWith('/api/')) return r.continue();
    let body = { success: true, data: [] };
    if (u.pathname === '/api/pricing') {
      pricingReads++;
      body = {
        success: true,
        data: [
          {
            ...item,
            description_translations: {
              ...seed.translations,
              ...item.description_overrides,
            },
          },
        ],
        vendors: [{ id: 1, name: 'OpenAI', icon: 'OpenAI' }],
        usable_group: { default: 'default' },
        group_ratio: { default: 1 },
        supported_endpoint: {
          openai: { path: '/v1/chat/completions', method: 'POST' },
        },
      };
    } else if (u.pathname === '/api/status')
      body = { success: true, data: status };
    else if (u.pathname === '/api/user/self')
      body = { success: true, data: { id: 8, role: 100, username: 'fixture' } };
    else if (u.pathname === '/api/models/1')
      body = { success: true, data: item };
    else if (u.pathname === '/api/models/' && r.method() === 'PUT') {
      const payload = JSON.parse(r.postData());
      writes.push(payload);
      for (const [lang, text] of Object.entries(
        payload.description_overrides || {},
      )) {
        if (text === null) delete item.description_overrides[lang];
        else item.description_overrides[lang] = text;
      }
      body = { success: true, data: item };
    } else if (u.pathname === '/api/vendors/')
      body = {
        success: true,
        data: { items: [{ id: 1, name: 'OpenAI', icon: 'OpenAI' }], total: 1 },
      };
    else if (u.pathname.startsWith('/api/health/'))
      body = {
        success: true,
        data: { groups: [], series: [], models: {}, interval: 3600 },
      };
    return r.respond({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(body),
    });
  });
  await page.goto(`${base}/${name}.html`, { waitUntil: 'networkidle0' });
  const selectLanguage = async (locale) => {
    await page.select('[data-site-language]', locale);
    await page.waitForFunction(
      (locale) => localStorage.getItem('site-language') === locale,
      {},
      locale,
    );
  };
  for (const [locale] of SITE_LANGUAGES) {
    await selectLanguage(locale);
    await page.waitForFunction(
      (text) =>
        document.querySelector('.pricing-model-description')?.textContent ===
        text,
      {},
      seed.translations[locale],
    );
  }
  // Switching display languages reuses the original API snapshot.
  assert.equal(pricingReads, 1);
  await selectLanguage('zh-CN');
  await page.waitForFunction(
    (text) =>
      document.querySelector('.pricing-model-description')?.textContent ===
      text,
    {},
    seed.translations['zh-CN'],
  );
  const search = await page.$(
    '.pricing-search input, input[placeholder="模糊搜索模型名称"]',
  );
  assert.ok(search, 'search input');
  await search.type('复杂推理');
  await page.waitForSelector('.pricing-model-description');
  assert.equal(
    await page.$$('.pricing-model-description').then((x) => x.length),
    1,
  );
  await page.screenshot({ path: path.join(artifacts, 'catalog-zh.png') });
  await search.click({ clickCount: 3 });
  await page.keyboard.press('Backspace');
  await page.click('.pricing-model-card a[href="/model/gpt-6-astra"]');
  await page.waitForSelector(
    '.model-detail-title-block p, .model-detail-hero p',
  );
  for (const [locale] of SITE_LANGUAGES) {
    await selectLanguage(locale);
    await page.waitForFunction(
      (text) =>
        document
          .querySelector('.model-detail-hero')
          ?.textContent.includes(text),
      {},
      seed.translations[locale],
    );
  }
  await selectLanguage('ja');
  await page.screenshot({ path: path.join(artifacts, 'detail-ja.png') });
  await page.setViewport({ width: 390, height: 844 });
  await selectLanguage('zh-CN');
  await page.waitForFunction(
    (text) =>
      document.querySelector('.model-detail-hero')?.textContent.includes(text),
    {},
    seed.translations['zh-CN'],
  );
  assert.equal(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
    true,
  );
  await page.screenshot({ path: path.join(artifacts, 'detail-mobile-zh.png') });
  await page.setViewport({ width: 1440, height: 1000 });
  await page.click('[data-edit]');
  await page.waitForSelector('textarea[aria-label="多语言介绍"]');
  await page.waitForFunction(
    (text) =>
      document.querySelector('textarea[aria-label="多语言介绍"]')?.value ===
      text,
    {},
    seed.translations['zh-CN'],
  );
  const textarea = await page.$('textarea[aria-label="多语言介绍"]');
  await textarea.click({ clickCount: 3 });
  await page.keyboard.press('Backspace');
  await textarea.type('人工校正的中文模型介绍');
  await page.screenshot({ path: path.join(artifacts, 'editor-zh.png') });
  const clickText = async (text) => {
    await page.waitForFunction(
      (text) =>
        [...document.querySelectorAll('button')].some(
          (x) => x.textContent.trim() === text && !x.disabled,
        ),
      {},
      text,
    );
    await page.evaluate(
      (text) =>
        [...document.querySelectorAll('button')]
          .find((x) => x.textContent.trim() === text && !x.disabled)
          .click(),
      text,
    );
  };
  await clickText('提交');
  await page.waitForFunction(
    () => !document.querySelector('textarea[aria-label="多语言介绍"]'),
  );
  assert.deepEqual(writes.at(-1).description_overrides, {
    'zh-CN': '人工校正的中文模型介绍',
  });
  assert.ok(!('description_translations' in writes.at(-1)));
  assert.ok(!('description' in writes.at(-1)));
  await page.click('[data-edit]');
  await page.waitForFunction(
    () =>
      document.querySelector('textarea[aria-label="多语言介绍"]')?.value ===
      '人工校正的中文模型介绍',
  );
  await clickText('恢复自动');
  await page.waitForFunction(
    (text) =>
      document.querySelector('textarea[aria-label="多语言介绍"]')?.value ===
      text,
    {},
    seed.translations['zh-CN'],
  );
  await clickText('提交');
  await page.waitForFunction(
    () => !document.querySelector('textarea[aria-label="多语言介绍"]'),
  );
  assert.deepEqual(writes.at(-1).description_overrides, { 'zh-CN': null });
  assert.deepEqual(errors, []);
  assert.equal(external, 0);
  console.log(
    JSON.stringify({
      result: 'passed',
      locales: SITE_LANGUAGES.length,
      pricingReads,
      external,
      writes: writes.length,
      artifacts,
    }),
  );
} finally {
  await browser?.close();
  await server.close();
  stopEsbuild();
  for (const file of files) fs.rmSync(file, { force: true });
}
