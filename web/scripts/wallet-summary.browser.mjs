import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import { fileURLToPath } from 'node:url';
import { createServer, loadConfigFromFile } from 'vite';
import { stop } from 'esbuild';
import puppeteer from 'puppeteer-core';
const root = fileURLToPath(new URL('..', import.meta.url));
process.chdir(root);
const fixture = await fs.mkdtemp(path.join(root, '.wallet-summary-qa-'));
const output = await fs.mkdtemp(path.join(os.tmpdir(), 'wallet-summary-'));
let server, browser;
try {
  await fs.writeFile(
    path.join(fixture, 'index.html'),
    '<html><meta name="viewport" content="width=device-width,initial-scale=1"><div id="root"></div><script type="module" src="./main.jsx"></script></html>',
  );
  await fs.writeFile(
    path.join(fixture, 'main.jsx'),
    `import React from 'react';import{createRoot}from'react-dom/client';import{MemoryRouter,Routes,Route}from'react-router-dom';import{UserContext}from'../src/context/User';import Header from'../src/components/dashboard/DashboardHeader';import{useTranslation}from'react-i18next';import{initialLocaleReady}from'../src/i18n/i18n';import{changeSiteLanguage}from'../src/helpers/siteLanguage';import'../src/index.css';import'../src/components/dashboard/dashboardOverview.css';await initialLocaleReady;function App(){const{t}=useTranslation();const[id,setId]=React.useState(1);const[version,refresh]=React.useState(0);const[range,setRange]=React.useState('custom');window.walletAccount=setId;window.walletRefresh=()=>refresh(v=>v+1);window.walletLanguage=changeSiteLanguage;return <UserContext.Provider value={[{user:{id,role:1}},()=>{}]}><MemoryRouter initialEntries={['/console']}><Routes><Route path='/console' element={<div className='dashboard-overview'><Header t={t} inputs={{start_timestamp:'2026-11-06T04:22:00',end_timestamp:'2026-11-07T04:22:00'}} rangePreset={range} onRangeChange={setRange} refresh={window.walletRefresh} walletRefreshVersion={version} showSearchModal={()=>{}}/><div style={{padding:24,background:'var(--semi-color-bg-0)',border:'1px solid #ddd',borderRadius:10}}>使用统计 · 本地合成数据</div></div>}/><Route path='/console/topup' element={<p data-wallet-destination>钱包管理</p>}/></Routes></MemoryRouter></UserContext.Provider>}createRoot(document.getElementById('root')).render(<App/>);`,
  );
  const { config } = await loadConfigFromFile(
    { command: 'serve', mode: 'development' },
    path.join(root, 'vite.config.js'),
  );
  server = await createServer({
    ...config,
    configFile: false,
    root,
    plugins: config.plugins
      .flat(Infinity)
      .filter((p) => p?.name !== '@code-inspector/vite'),
    server: { host: '127.0.0.1', port: 0, proxy: {}, open: false },
  });
  await server.listen();
  const origin = `http://127.0.0.1:${server.httpServer.address().port}`;
  browser = await puppeteer.launch({
    headless: true,
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  });
  const page = await browser.newPage();
  page.setDefaultTimeout(12000);
  await page.evaluateOnNewDocument(() => {
    localStorage.setItem('site-language', 'zh-CN');
    localStorage.setItem('user', '{"id":1,"role":1}');
    localStorage.setItem('quota_per_unit', '500000');
    localStorage.setItem('quota_display_type', 'USD');
  });
  await page.setRequestInterception(true);
  let calls = 0,
    mode = 'ok',
    balance = 62775000,
    net = 54265000,
    held = null,
    hold = false;
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  const respond = (req, id) => {
    const asof = 1794040000;
    return req.respond({
      status: mode === 'error' ? 503 : 200,
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: {
          user_id: id,
          balance: id === 1 ? balance : 98765000,
          as_of: asof,
          usage: {
            status: mode === 'error' ? 'ok' : mode,
            net_quota: mode === 'ok' ? net : null,
            start_timestamp: asof - 86400,
            end_timestamp: asof,
          },
        },
      }),
    });
  };
  page.on('request', async (req) => {
    const url = new URL(req.url());
    if (url.pathname === '/api/data/self/wallet') {
      calls++;
      const id = Number(req.headers()['new-api-user']);
      if (hold) {
        held = req;
        return;
      }
      return respond(req, id);
    }
    if (url.origin !== origin) return req.abort();
    return req.continue();
  });
  await page.setViewport({ width: 1440, height: 900 });
  await page.goto(origin + '/' + path.basename(fixture) + '/index.html');
  await page.waitForFunction(() =>
    document
      .querySelector('.dashboard-wallet-trigger strong')
      ?.textContent.includes('125.55'),
  );
  const pos = await page.evaluate(() => {
    const b = document
        .querySelector('.dashboard-wallet')
        .getBoundingClientRect(),
      a = document
        .querySelector('.dashboard-query-actions')
        .getBoundingClientRect();
    return { gap: a.left - b.right, y: a.top - b.top };
  });
  assert(
    pos.gap >= 10 && pos.gap <= 14 && Math.abs(pos.y) < 8,
    'wallet sits next to controls',
  );
  await page.screenshot({ path: path.join(output, 'desktop.png') });
  const before = calls;
  await page.select('.dashboard-range-select', '7d');
  assert.equal(calls, before, 'date filter cannot refetch/change wallet');
  await page.click('.dashboard-wallet-trigger');
  await page.waitForSelector('#dashboard-wallet-details');
  assert(
    (
      await page.$eval('.dashboard-wallet-metrics', (e) => e.textContent)
    ).includes('108.53'),
  );
  await page.screenshot({ path: path.join(output, 'detail.png') });
  await page.keyboard.press('Escape');
  assert.equal(await page.$('#dashboard-wallet-details'), null);
  for (const width of [1280, 1024, 768, 390]) {
    await page.setViewport({ width, height: 900 });
    assert(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
      'overflow ' + width,
    );
  }
  await page.evaluate(() => {
    document.documentElement.classList.add('dark');
    document.body.setAttribute('theme-mode', 'dark');
  });
  await page.screenshot({ path: path.join(output, 'mobile-dark.png') });
  await page.evaluate(() => window.walletLanguage('en'));
  await page.waitForFunction(() =>
    document
      .querySelector('.dashboard-wallet-trigger')
      ?.textContent.includes('Balance'),
  );
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  );
  const refresh = async () => {
    const response = page.waitForResponse((r) =>
      r.url().endsWith('/api/data/self/wallet'),
    );
    await page.evaluate(() => window.walletRefresh());
    await response;
    await page.evaluate(
      () =>
        new Promise((resolve) =>
          requestAnimationFrame(() => requestAnimationFrame(resolve)),
        ),
    );
    await page.waitForFunction(
      () =>
        document
          .querySelector('.dashboard-wallet-trigger strong')
          ?.getAttribute('aria-busy') === 'false',
    );
  };
  for (const state of ['unavailable', 'not_recorded', 'error']) {
    mode = state;
    await refresh();
    await page.waitForFunction(
      () =>
        document
          .querySelector('.dashboard-wallet-trigger strong')
          ?.getAttribute('aria-busy') === 'false',
    );
    await page.click('.dashboard-wallet-trigger');
    assert(await page.$('#dashboard-wallet-details'));
    if (state !== 'error')
      assert(
        (
          await page.$eval('.dashboard-wallet-amount', (e) => e.textContent)
        ).includes('125.55'),
      );
    await page.keyboard.press('Escape');
  }
  mode = 'ok';
  balance = 0;
  net = 0;
  await refresh();
  await page.waitForFunction(
    () =>
      document
        .querySelector('.dashboard-wallet-trigger strong')
        ?.getAttribute('aria-busy') === 'false',
  );
  assert(
    (
      await page.$eval('.dashboard-wallet-status', (e) => e.textContent)
    ).includes('Top up'),
  );
  balance = 62775000;
  net = 54265000;
  hold = true;
  await page.evaluate(() => window.walletRefresh());
  await page.waitForFunction(
    () =>
      document
        .querySelector('.dashboard-wallet-trigger strong')
        ?.getAttribute('aria-busy') === 'true',
  );
  // Wait for the intercepted request before replacing the account.
  for (let i = 0; i < 100 && !held; i++)
    await new Promise((r) => setTimeout(r, 10));
  assert(held);
  hold = false;
  await page.evaluate(() => {
    localStorage.setItem('user', '{"id":2,"role":1}');
    window.walletAccount(2);
  });
  await page.waitForFunction(() =>
    document
      .querySelector('.dashboard-wallet-trigger strong')
      ?.textContent.includes('197.53'),
  );
  await respond(held, 1).catch(() => {});
  assert(
    (
      await page.$eval('.dashboard-wallet-trigger strong', (e) => e.textContent)
    ).includes('197.53'),
  );
  await page.click('.dashboard-wallet-topup');
  await page.screenshot({ path: path.join(output, 'after-link.png') });

  await page.waitForSelector('[data-wallet-destination]');
  assert.deepEqual(errors, []);
  console.log(
    JSON.stringify({
      passed: true,
      output,
      checks:
        'adjacent-controls,independent-date-range,popover,Escape,5-widths,mobile-dark,English,partial-failure,retry,new-user,identity-and-late-response,wallet-destination',
    }),
  );
} finally {
  await browser?.close();
  await server?.close();
  stop();
  await fs.rm(fixture, { recursive: true, force: true });
}
