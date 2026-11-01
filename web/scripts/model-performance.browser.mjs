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
const name = `.performance-p50-qa-${process.pid}`;
const files = ['html', 'jsx'].map((ext) => path.join(root, `${name}.${ext}`));
const artifacts = fs.mkdtempSync(
  path.join(os.tmpdir(), 'performance-p50-browser-'),
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
  tags: '1m,vision,tools,CustomTag,claude-haiku,gpt-mini,gemini-flash,o-pro',
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
    tags: 'audio,8.2k,speech',
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
let measured = false,
  unavailable = false;
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
    else if (u.pathname.startsWith('/api/health/')) {
      const ref = unavailable
        ? undefined
        : {
            source: 'openrouter',
            percentile: 50,
            fetched_at: Math.floor(Date.now() / 1000) - 5,
            expires_at: Math.floor(Date.now() / 1000) + 295,
            ttft_ms: 1915,
            output_tps: 20,
          };
      const stats = {
        request_count: 0,
        success_count: 0,
        avg_ttft_ms: measured ? 300 : null,
        ttft_count: measured ? 1 : 0,
        avg_output_tps: measured ? 77 : null,
        performance_reference: ref,
        series: [],
      };
      body = {
        success: true,
        data: {
          enabled: true,
          groups: ['chat', 'image', 'speech'].map((group) => ({
            group,
            ...stats,
          })),
          models: items.map((m) => ({ model: m.model_name, ...stats })),
          series: [],
        },
      };
    }
    await r.respond({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(body),
    });
  });
  await page.goto(`${base}/${name}.html`, { waitUntil: 'networkidle0' });
  const card =
    '.pricing-model-card[aria-label="视觉助手"] .pricing-card-performance';
  await page.waitForFunction(
    (selector) =>
      document.querySelector(selector)?.textContent.includes('1.92 s'),
    {},
    card,
  );
  let text = await page.$eval(card, (e) => e.textContent);
  assert.ok(text.includes('首字时间') && text.includes('20'));
  assert.ok(!/OpenRouter|P50|参考|首字均值/.test(text));
  await page.screenshot({ path: path.join(artifacts, 'catalog-desktop.png') });
  await page.click('.pricing-model-card a[href="/model/chat-00"]');
  await page.waitForFunction(() =>
    document
      .querySelector('.model-group-health-values')
      ?.textContent.includes('1.92 s'),
  );
  text = await page.$eval('.model-group-health-values', (e) => e.textContent);
  assert.ok(text.includes('首字时间') && text.includes('20'));
  assert.ok(!/OpenRouter|P50|参考/.test(text));
  assert.ok(!text.includes('100.00%'), 'external data must not create uptime');
  await page.setViewport({ width: 390, height: 844 });
  await page.evaluate(() => {
    document.documentElement.classList.add('dark');
    localStorage.setItem('theme', 'dark');
  });
  await page.screenshot({
    path: path.join(artifacts, 'detail-mobile.png'),
    fullPage: true,
  });
  assert.ok(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  );
  measured = true;
  await page.goto(`${base}/${name}.html`, { waitUntil: 'networkidle0' });
  await page.waitForFunction(
    (selector) =>
      document.querySelector(selector)?.textContent.includes('300 ms'),
    {},
    card,
  );
  text = await page.$eval(card, (e) => e.textContent);
  assert.ok(text.includes('首字均值') && text.includes('77'));
  measured = false;
  unavailable = true;
  await page.reload({ waitUntil: 'networkidle0' });
  await page.waitForFunction(
    (selector) =>
      (document.querySelector(selector)?.textContent.match(/暂无数据/g) || [])
        .length === 2,
    {},
    card,
  );
  assert.deepEqual(errors, []);
  assert.deepEqual(writes, []);
  console.log(
    JSON.stringify({
      passed: true,
      artifacts,
      checks: [
        'card P50',
        'detail P50',
        'no source labels',
        'no synthetic uptime',
        '390px dark layout',
        'measured priority',
        'missing reference remains unknown',
      ],
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
