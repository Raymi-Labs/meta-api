import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';
import { createServer, loadConfigFromFile } from 'vite';
import { stop as stopEsbuild } from 'esbuild';
const root = fileURLToPath(new URL('..', import.meta.url));
process.chdir(root);
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
const name = `.catalog-facets-qa-${process.pid}`;
const files = ['html', 'jsx'].map((ext) => path.join(root, `${name}.${ext}`));
const artifacts = fs.mkdtempSync(
  path.join(os.tmpdir(), 'catalog-facets-browser-'),
);
const status = {
  system_name: '元衡 API',
  server_address: base,
  quota_per_unit: 500000,
  usd_exchange_rate: 1,
  price: 1,
  quota_display_type: 'CNY',
};
const common = {
  description: '测试模型',
  name_rule: 0,
  status: 1,
  sync_official: 1,
  model_ratio: 1,
  completion_ratio: 3,
  quota_type: 0,
  function_overrides: {},
  context_length: 128000,
  input_modalities: ['text', 'image'],
  output_modalities: ['text'],
  supported_endpoint_types: ['openai'],
  endpoints: '{"openai":{"path":"/v1/chat/completions","method":"POST"}}',
  capabilities: { reasoning: true, function_calling: true },
  tags: '1m,vision,tools,CustomTag',
};
const items = Array.from({ length: 25 }, (_, i) => ({
  ...common,
  id: i + 1,
  model_name: `chat-${String(i).padStart(2, '0')}`,
  display_name: i === 0 ? '视觉助手' : '',
  vendor_id: 1,
  icon: 'OpenAI',
  enable_groups: ['chat'],
}));
items.push(
  {
    ...common,
    id: 26,
    model_name: 'image-demo',
    vendor_id: 2,
    icon: 'Gemini.Color',
    enable_groups: ['image'],
    input_modalities: ['text', 'image'],
    output_modalities: ['image'],
    supported_endpoint_types: ['image-generation'],
    capabilities: {},
    tags: 'image,1m',
    quota_type: 1,
    model_price: 0.1,
  },
  {
    ...common,
    id: 27,
    model_name: 'speech-demo',
    vendor_id: 3,
    icon: 'Xiaomi.Color',
    enable_groups: ['speech'],
    input_modalities: ['text'],
    output_modalities: ['audio'],
    supported_endpoint_types: ['audio-speech'],
    capabilities: {},
    tags: 'audio,8.2k',
  },
);
const vendors = [
  { id: 1, name: 'OpenAI', icon: 'OpenAI' },
  { id: 2, name: 'Google', icon: 'Gemini.Color' },
  { id: 3, name: '小米 MiMo', icon: 'Xiaomi.Color' },
];
let browser,
  failOnce = false,
  restricted = false;
const errors = [],
  writes = [];
try {
  fs.writeFileSync(
    files[0],
    `<html><meta name="viewport" content="width=device-width, initial-scale=1"><div id="root"></div><script type="module" src="/${name}.jsx"></script></html>`,
  );
  fs.writeFileSync(
    files[1],
    `import React from 'react';import{createRoot}from'react-dom/client';import{MemoryRouter,Routes,Route,Link}from'react-router-dom';import Pricing from './src/pages/Pricing';import ModelDetail from './src/pages/ModelDetail';import EditModelModal from './src/components/table/models/modals/EditModelModal';import{UserContext}from'./src/context/User';import{StatusContext}from'./src/context/Status';import{ThemeProvider}from'./src/context/Theme';import{changeSiteLanguage,SITE_LANGUAGES}from'./src/helpers/siteLanguage';import{initialLocaleReady}from'./src/i18n/i18n';import './src/index.css';function App(){const[open,setOpen]=React.useState(false),[revision,setRevision]=React.useState(0),[user,setUser]=React.useState({id:8,role:100,username:'fixture',group:'default'});return <UserContext.Provider value={[{user},()=>{}]}><div style={{display:'flex',gap:12,padding:12,flexWrap:'wrap'}}><select data-site-language defaultValue="zh-CN" onChange={e=>changeSiteLanguage(e.target.value)}>{SITE_LANGUAGES.map(([code,label])=><option key={code} value={code}>{label}</option>)}</select><button data-edit onClick={()=>setOpen(true)}>模型管理</button><button data-account onClick={()=>setUser({id:9,role:1,username:'other',group:'default'})}>切换账户</button></div><Routes><Route path="/pricing" element={<Pricing key={revision}/>}/><Route path="/model/:name" element={<ModelDetail/>}/></Routes><EditModelModal visiable={open} editingModel={{id:1,model_name:'chat-00'}} handleClose={()=>setOpen(false)} refresh={()=>setRevision(x=>x+1)}/></UserContext.Provider>};await initialLocaleReady;createRoot(document.getElementById('root')).render(<MemoryRouter initialEntries={['/pricing']}><StatusContext.Provider value={[{status:${JSON.stringify(status)}},()=>{}]}><ThemeProvider><App/></ThemeProvider></StatusContext.Provider></MemoryRouter>);`,
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
    if (u.origin !== base) return r.abort();
    if (!u.pathname.startsWith('/api/')) return r.continue();
    let body = { success: true, data: [] };
    if (u.pathname === '/api/pricing') {
      if (failOnce) {
        failOnce = false;
        body = { success: false, message: 'fixture unavailable' };
      } else
        body = {
          success: true,
          data: restricted ? [items[26]] : items,
          vendors,
          usable_group: { chat: 'chat', image: 'image', speech: 'speech' },
          group_ratio: { chat: 1, image: 1, speech: 1 },
          supported_endpoint: {
            openai: { path: '/v1/chat/completions', method: 'POST' },
          },
        };
    } else if (u.pathname === '/api/models/1')
      body = { success: true, data: items[0] };
    else if (u.pathname === '/api/models/' && r.method() === 'PUT') {
      const payload = JSON.parse(r.postData());
      writes.push(payload);
      Object.assign(items[0], payload);
    } else if (u.pathname === '/api/vendors/')
      body = { success: true, data: { items: vendors, total: 3 } };
    else if (u.pathname === '/api/status')
      body = { success: true, data: status };
    else if (u.pathname === '/api/user/self')
      body = { success: true, data: { id: 8, role: 100, username: 'fixture' } };
    else if (u.pathname.startsWith('/api/health/'))
      body = {
        success: true,
        data: {
          enabled: true,
          groups: ['chat', 'image', 'speech'].map((group) => ({
            group,
            request_count: 100,
            success_count: 99,
            success_rate: 99,
          })),
          series: [],
          models: {},
          interval: 3600,
        },
      };
    await r.respond({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(body),
    });
  });
  const count = async (n) =>
    page.waitForFunction(
      (n) =>
        document.querySelector('.pricing-result-count')?.textContent.trim() ===
          String(n) &&
        !document.querySelector('.pricing-model-card:not([aria-label])'),
      {},
      n,
    );
  const clickText = async (text, selector = 'button') => {
    await page.waitForFunction(
      (text, selector) =>
        [...document.querySelectorAll(selector)].some(
          (e) => e.textContent.trim() === text && !e.disabled,
        ),
      {},
      text,
      selector,
    );
    await page.evaluate(
      (text, selector) =>
        [...document.querySelectorAll(selector)]
          .find((e) => e.textContent.trim() === text && !e.disabled)
          .click(),
      text,
      selector,
    );
  };
  const facet = async (
    dim,
    label,
    scope = '.pricing-page > .catalog-four-facets',
  ) => {
    const selector = `${scope} [data-facet="${dim}"] .catalog-facet-options button`;
    await page.evaluate(
      (selector, label) => {
        const button = [...document.querySelectorAll(selector)].find(
          (e) =>
            [...e.childNodes]
              .filter((n) => n.nodeType === Node.TEXT_NODE)
              .map((n) => n.textContent)
              .join('')
              .trim() === label,
        );
        if (!button || button.disabled)
          throw Error(`Unavailable facet ${label}`);
        button.click();
      },
      selector,
      label,
    );
  };
  await page.goto(`${base}/${name}.html`, { waitUntil: 'networkidle0' });
  await count(27);
  await page.waitForSelector('.pricing-model-card[aria-label]');
  assert.equal(
    await page.$$eval('.pricing-model-card', (els) => els.length),
    20,
  );
  await facet('vendor', 'OpenAI');
  await count(25);
  await facet('vendor', 'Google');
  await count(26);
  await facet('function', '文生图');
  await count(1);
  await facet('function', '文本对话');
  await count(26);
  await facet('tag', '工具调用');
  await count(25);
  await facet('tag', '深度思考');
  await count(25);
  await facet('input', '图片');
  await count(25);
  await facet('input', '文本');
  await count(25);
  await page.click('.pricing-model-card a[href="/model/chat-00"]');
  await page.waitForSelector('.model-detail-hero');
  await page.click('a[href="/pricing"]');
  await count(25);
  assert.equal(
    await page.$$eval(
      '.pricing-page > .catalog-four-facets [aria-pressed="true"]',
      (els) => els.length,
    ),
    8,
  );
  await clickText('清空');
  await count(27);
  const search = await page.$('input[placeholder="模糊搜索模型名称"]');
  await search.type('视觉助手');
  await count(1);
  assert.equal(
    await page.$eval('[data-facet="function"] .catalog-facet-options', (e) =>
      e.textContent.includes('文本对话1'),
    ),
    true,
  );
  await search.click({ clickCount: 3 });
  await page.keyboard.press('Backspace');
  await count(27);
  await facet('function', '语音合成');
  await count(1);
  await clickText('分组健康', '[role="tab"]');
  await page.waitForSelector('.group-health-panel');
  const healthText = await page.$eval(
    '.group-health-grid',
    (e) => e.textContent,
  );
  assert.ok(healthText.includes('speech'));
  assert.ok(!healthText.includes('chat'));
  assert.ok(!healthText.includes('image'));
  await clickText('模型列表 1', '[role="tab"]');
  await clickText('清空');
  await count(27);
  await page.screenshot({ path: path.join(artifacts, 'catalog-desktop.png') });
  // Real admin form: exclude automatic text chat and explicitly include image editing.
  await page.click('[data-edit]');
  await page.waitForSelector('[aria-labelledby="function-label-text_chat"]');
  await page.waitForFunction(() =>
    [...document.querySelectorAll('button')].some(
      (b) => b.textContent.trim() === '提交' && !b.disabled,
    ),
  );
  await page.click('[aria-labelledby="function-label-text_chat"]');
  await clickText('排除', '[role="option"]');
  await page.waitForFunction(
    () =>
      document
        .querySelector('[aria-labelledby="function-label-text_chat"]')
        ?.textContent.trim() === '排除',
  );
  await page.click('[aria-labelledby="function-label-image_editing"]');
  await clickText('包含', '[role="option"]');
  await page.waitForFunction(() =>
    document
      .querySelector('[aria-label="广场展示预览"]')
      ?.textContent.includes('图像编辑'),
  );
  assert.ok(
    (
      await page.$eval('[aria-label="广场展示预览"]', (e) => e.textContent)
    ).includes('图像编辑'),
  );
  await page.screenshot({ path: path.join(artifacts, 'admin-functions.png') });
  await clickText('提交');
  await page.waitForFunction(
    () =>
      !document.querySelector('[aria-labelledby="function-label-text_chat"]'),
  );
  assert.deepEqual(writes.at(-1).function_overrides, {
    text_chat: false,
    image_editing: true,
  });
  await count(27);
  await facet('function', '图像编辑');
  await count(1);
  assert.equal(
    await page.$eval('.pricing-model-card', (e) =>
      e.getAttribute('aria-label'),
    ),
    '视觉助手',
  );
  await clickText('清空');
  await page.click('[data-edit]');
  await page.waitForSelector('[aria-labelledby="function-label-text_chat"]');
  await page.waitForFunction(() =>
    [...document.querySelectorAll('button')].some(
      (b) => b.textContent.trim() === '提交' && !b.disabled,
    ),
  );
  await page.click('[aria-labelledby="function-label-text_chat"]');
  await clickText('自动 · 包含', '[role="option"]');
  await page.waitForFunction(
    () =>
      document
        .querySelector('[aria-labelledby="function-label-text_chat"]')
        ?.textContent.trim() === '自动 · 包含',
  );
  await page.click('[aria-labelledby="function-label-image_editing"]');
  await clickText('自动 · 排除', '[role="option"]');
  await page.waitForFunction(
    () =>
      document
        .querySelector('[aria-labelledby="function-label-image_editing"]')
        ?.textContent.trim() === '自动 · 排除',
  );
  await clickText('提交');
  await page.waitForFunction(
    () =>
      !document.querySelector('[aria-labelledby="function-label-text_chat"]'),
  );
  assert.deepEqual(writes.at(-1).function_overrides, {});
  await count(27);
  const locales = fs
    .readdirSync(path.join(root, 'src/i18n/locales'))
    .filter((x) => x.endsWith('.json'));
  for (const file of locales) {
    const lang = file.slice(0, -5),
      t = JSON.parse(
        fs.readFileSync(path.join(root, 'src/i18n/locales', file)),
      ).translation;
    await page.select('[data-site-language]', lang);
    await page.waitForFunction(
      (label) =>
        document.querySelector('[data-facet="function"] .catalog-facet-title')
          ?.textContent === label,
      {},
      t['功能'],
    );
  }
  await page.select('[data-site-language]', 'zh-CN');
  await page.waitForFunction(
    () =>
      document.querySelector('[data-facet="function"] .catalog-facet-title')
        ?.textContent === '功能',
  );
  await page.evaluate(() => document.body.setAttribute('theme-mode', 'dark'));
  await page.screenshot({ path: path.join(artifacts, 'catalog-dark.png') });
  await page.evaluate(() => document.body.removeAttribute('theme-mode'));
  await page.setViewport({ width: 390, height: 844 });
  await clickText('更多筛选');
  await page.waitForSelector('.pricing-filter-drawer .catalog-facets-drawer');
  await facet('function', '文生图', '.catalog-facets-drawer');
  await count(1);
  await page.waitForFunction(
    () =>
      document
        .querySelector('.pricing-filter-drawer .semi-sidesheet-inner')
        ?.getBoundingClientRect().right <=
      innerWidth + 1,
  );
  assert.equal(
    await page.$eval(
      '.pricing-filter-drawer .semi-sidesheet-body',
      (e) => e.scrollWidth <= e.clientWidth,
    ),
    true,
  );
  await page.screenshot({ path: path.join(artifacts, 'mobile-drawer.png') });
  await clickText('关闭');
  assert.equal(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
    true,
  );
  await page.screenshot({ path: path.join(artifacts, 'mobile-selected.png') });
  await clickText('清空');
  await count(27);
  await page.setViewport({ width: 1440, height: 1000 });
  items[0].tags +=
    ',' +
    Array.from({ length: 40 }, (_, i) => `Custom category ${i}`).join(',');
  await page.reload({ waitUntil: 'networkidle0' });
  await count(27);
  await page.waitForSelector('[data-facet="tag"] .catalog-facet-expand');
  assert.ok(
    await page.$$eval(
      '[data-facet="tag"] button[aria-hidden="true"]',
      (els) => els.length > 0 && els.every((e) => e.tabIndex === -1),
    ),
  );
  await page.click('[data-facet="tag"] .catalog-facet-expand');
  await page.waitForFunction(
    () =>
      !document.querySelector('[data-facet="tag"] button[aria-hidden="true"]'),
  );
  await facet('tag', 'Custom category 39');
  await count(1);
  await clickText('清空');
  await count(27);
  failOnce = true;
  await page.reload({ waitUntil: 'networkidle0' });
  await page.waitForSelector('.pricing-catalog-empty[role="alert"]');
  await clickText('重试');
  await count(27);
  restricted = true;
  await page.click('[data-account]');
  await count(1);
  assert.equal(
    await page
      .$$eval('.pricing-model-card', (els) =>
        els.map((e) => e.getAttribute('aria-label')),
      )
      .then((x) => x[0]),
    'speech-demo',
  );
  assert.deepEqual(errors, []);
  console.log(
    JSON.stringify({
      passed: true,
      artifacts,
      locales: locales.length,
      adminWrites: writes.length,
    }),
  );
} catch (error) {
  console.error('Artifacts:', artifacts);
  console.error('Page errors:', errors);
  if (browser) {
    const pages = await browser.pages();
    await pages
      .at(-1)
      .screenshot({ path: path.join(artifacts, 'failure.png') });
    fs.writeFileSync(
      path.join(artifacts, 'failure.html'),
      await pages.at(-1).content(),
    );
  }
  throw error;
} finally {
  await browser?.close();
  await server.close();
  stopEsbuild();
  for (const file of files) fs.rmSync(file, { force: true });
}
