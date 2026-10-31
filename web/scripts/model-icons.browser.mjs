import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';
import { createServer, loadConfigFromFile } from 'vite';
import { stop } from 'esbuild';

const root = fileURLToPath(new URL('..', import.meta.url));
const artifacts = fs.mkdtempSync(path.join(os.tmpdir(), 'model-icons-'));
const name = `.model-icons-${process.pid}`;
const files = ['html', 'jsx'].map((ext) => path.join(root, `${name}.${ext}`));
const vendor = { id: 20, name: '小米 MiMo', icon: "Xiaomi.color='#FF6900'" };
const model = {
  model_name: 'mimo-v2.6-pro',
  icon: 'XiaomiMiMo',
  vendor_id: 20,
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 2,
  enable_groups: ['Public'],
  supported_endpoint_types: ['openai'],
};
const asrModel = { ...model, model_name: 'mimo-v2.5-asr', icon: null };
const pricing = {
  success: true,
  data: [model, asrModel],
  vendors: [vendor],
  group_ratio: { Public: 1 },
  usable_group: { Public: 'Public' },
  supported_endpoint: {
    openai: { path: '/v1/chat/completions', method: 'POST' },
  },
};
let browser, server;
try {
  fs.writeFileSync(
    files[0],
    `<html><body><div id="root"></div><script type="module" src="/${name}.jsx"></script></body></html>`,
  );
  fs.writeFileSync(
    files[1],
    `import React from 'react';import {createRoot} from 'react-dom/client';import {getLobeHubIcon} from './src/helpers/render';import ModelHeader from './src/components/table/model-pricing/modal/components/ModelHeader';import {getModelsColumns} from './src/components/table/models/ModelsColumnDefs';import './src/i18n/i18n';import './src/index.css';
    const cases=[['mimo','XiaomiMiMo',32],['brand','Xiaomi',24],['color','XiaomiMiMo.Color',24],['custom',"XiaomiMiMo.color='#336699'",24],['mono','XiaomiMiMo.Mono',24],['inherit',"XiaomiMiMo.color='currentColor'",24],['variant',"XiaomiMiMo.Color.color='#FF6900'.size=40",20],['vendor','FutureUnsupported',24,"Xiaomi.color='#FF6900'"],['missing','',24,'Xiaomi'],['unknown','UnknownModel',24,'UnknownVendor'],['valid','OpenAI',24,'Xiaomi'],['legacy',"Xiaomi.color='#FF6900'",24]];
    const record={model_name:'mimo-v2.6-pro',icon:'FutureUnsupported',vendor_icon:"Xiaomi.color='#FF6900'",vendor_id:20};const iconColumn=getModelsColumns({t:text=>text,vendorMap:{20:{icon:record.vendor_icon}}})[0];createRoot(document.getElementById('root')).render(<main>{cases.map(([id,...args])=><div id={id} key={id}>{getLobeHubIcon(...args)}</div>)}<div id='sheet'><ModelHeader modelData={record} t={text=>text}/></div><div id='admin'>{iconColumn.render(null,record)}</div></main>);`,
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
    plugins: config.plugins
      .flat(Infinity)
      .filter((p) => p?.name !== '@code-inspector/vite'),
  });
  await server.listen();
  const base = `http://127.0.0.1:${server.httpServer.address().port}`;
  browser = await puppeteer.launch({
    headless: true,
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  });
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.setViewport({ width: 1440, height: 900 });
  await page.goto(`${base}/${name}.html`);
  await page.waitForSelector('#legacy svg');
  assert.equal(
    await page.$eval('#mimo', (el) => !!el.querySelector('svg')),
    true,
    'synchronized XiaomiMiMo must resolve to an SVG, not a letter Avatar',
  );
  await page.waitForSelector('#valid svg');
  const paths = await page.evaluate(() =>
    Object.fromEntries(
      [
        'mimo',
        'variant',
        'vendor',
        'missing',
        'valid',
        'legacy',
        'sheet',
        'admin',
      ].map((id) => [
        id,
        document.querySelector(`#${id} svg path`)?.getAttribute('d'),
      ]),
    ),
  );
  for (const id of ['mimo', 'variant', 'vendor', 'missing', 'sheet', 'admin'])
    assert.equal(paths[id], paths.legacy, id);
  assert.notEqual(
    paths.valid,
    paths.legacy,
    'valid custom provider remains preferred',
  );
  assert.equal(
    await page.$eval('#variant svg', (el) => el.getBoundingClientRect().width),
    40,
  );
  assert.equal(
    await page.$eval('#variant svg', (el) => getComputedStyle(el).color),
    'rgb(255, 105, 0)',
  );
  for (const id of [
    'mimo',
    'brand',
    'color',
    'missing',
    'vendor',
    'legacy',
    'sheet',
    'admin',
  ]) {
    assert.equal(
      await page.$eval(`#${id} svg`, (el) => getComputedStyle(el).color),
      'rgb(255, 105, 0)',
      `${id} uses the brand default`,
    );
  }
  assert.equal(
    await page.$eval('#custom svg', (el) => getComputedStyle(el).color),
    'rgb(51, 102, 153)',
    'explicit color wins',
  );
  for (const id of ['mono', 'inherit']) {
    assert.equal(
      await page.$eval(
        `#${id}`,
        (el) =>
          getComputedStyle(el.querySelector('svg')).color ===
          getComputedStyle(el).color,
      ),
      true,
      `${id} preserves explicit monochrome`,
    );
  }
  assert.equal(await page.$eval('#unknown', (el) => el.textContent), 'U');

  await page.evaluateOnNewDocument(() => {
    localStorage.setItem('i18nextLng', 'zh-CN');
    localStorage.setItem('theme-mode', 'light');
  });
  await page.setRequestInterception(true);
  page.on('request', (request) => {
    const url = new URL(request.url());
    if (url.origin !== base) return request.abort();
    if (!url.pathname.startsWith('/api/')) return request.continue();
    let body = { success: true, data: [] };
    if (url.pathname === '/api/status')
      body = {
        success: true,
        data: {
          system_name: '元衡 API',
          quota_per_unit: 500000,
          quota_display_type: 'USD',
          usd_exchange_rate: 1,
          price: 1,
          HeaderNavModules: JSON.stringify({
            pricing: { enabled: true, requireAuth: false },
          }),
        },
      };
    if (url.pathname === '/api/pricing') body = pricing;
    if (url.pathname === '/api/pricing/exchange-rate')
      body = { success: false };
    if (url.pathname === '/api/user/self') body = { success: false };
    return request.respond({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(body),
    });
  });
  await page.goto(`${base}/pricing`);
  await page.waitForSelector('.pricing-model-card .pricing-model-icon svg');
  assert.equal(
    await page.$eval('.pricing-model-icon svg path', (el) =>
      el.getAttribute('d'),
    ),
    paths.legacy,
  );
  assert.deepEqual(
    await page.$$eval('.pricing-model-icon svg', (icons) =>
      icons.map((el) => getComputedStyle(el).color),
    ),
    ['rgb(255, 105, 0)', 'rgb(255, 105, 0)'],
    'synchronized and vendor-fallback cards agree',
  );
  await page.screenshot({
    path: path.join(artifacts, 'catalog.png'),
    fullPage: true,
  });
  await page.click('.pricing-model-card h2 a[href="/model/mimo-v2.6-pro"]');
  await page.waitForSelector('.model-detail-hero .model-detail-icon svg');
  assert.ok(page.url().includes('/model/mimo-v2.6-pro'));
  assert.equal(
    await page.$eval(
      '.model-detail-icon svg',
      (el) => getComputedStyle(el).color,
    ),
    'rgb(255, 105, 0)',
  );
  assert.equal(
    await page.$eval('.model-detail-icon svg path', (el) =>
      el.getAttribute('d'),
    ),
    paths.legacy,
  );
  await page.screenshot({
    path: path.join(artifacts, 'detail.png'),
    fullPage: true,
  });
  await page.$eval('#pricing', (el) => el.scrollIntoView());
  await page.waitForFunction(
    () => !document.querySelector('.model-sticky-bar').hidden,
  );
  assert.equal(
    await page.$eval('.model-sticky-icon svg path', (el) =>
      el.getAttribute('d'),
    ),
    paths.legacy,
  );
  assert.equal(
    await page.$eval(
      '.model-sticky-icon svg',
      (el) => getComputedStyle(el).color,
    ),
    'rgb(255, 105, 0)',
  );
  await page.goto(`${base}/compare?model=mimo-v2.6-pro`);
  await page.waitForSelector('.model-compare-logo svg');
  assert.equal(
    await page.$eval('.model-compare-logo svg path', (el) =>
      el.getAttribute('d'),
    ),
    paths.legacy,
  );

  assert.equal(
    await page.$eval(
      '.model-compare-logo svg',
      (el) => getComputedStyle(el).color,
    ),
    'rgb(255, 105, 0)',
  );

  // Unknown model icons still use the supplied vendor; no production metadata edits.
  model.icon = 'FutureUnsupported';
  await page.goto(`${base}/pricing`);
  await page.waitForSelector('.pricing-model-card .pricing-model-icon svg');
  assert.equal(
    await page.$eval('.pricing-model-icon svg path', (el) =>
      el.getAttribute('d'),
    ),
    paths.legacy,
  );
  await page.setViewport({ width: 390, height: 850 });
  await page.evaluate(() => {
    document.documentElement.classList.add('dark');
    document.body.setAttribute('theme-mode', 'dark');
  });
  assert.ok(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  );
  assert.deepEqual(
    await page.$$eval('.pricing-model-icon svg', (icons) =>
      icons.map((el) => getComputedStyle(el).color),
    ),
    ['rgb(255, 105, 0)', 'rgb(255, 105, 0)'],
  );
  await page.screenshot({
    path: path.join(artifacts, 'mobile-dark.png'),
    fullPage: true,
  });
  assert.deepEqual(errors, []);
  console.log(`Model icon checks passed; screenshots: ${artifacts}`);
} finally {
  await browser?.close();
  await server?.close();
  files.forEach((file) => fs.rmSync(file, { force: true }));
  stop();
}
