// Isolated real-header verification. No production login, payment or model calls.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { stop } from 'esbuild';
import puppeteer from 'puppeteer-core';
const root = fileURLToPath(new URL('..', import.meta.url));
process.chdir(root);
const fixture = await fs.mkdtemp(path.join(root, '.header-balance-qa-'));
const output = await fs.mkdtemp(path.join(os.tmpdir(), 'header-balance-qa-'));
let server, browser;
const errors = [],
  requests = [];
console.log('Screenshot directory:', output);
try {
  await fs.writeFile(
    path.join(fixture, 'index.html'),
    '<html><div id="root"></div><script type="module" src="./entry.jsx"></script></html>',
  );
  await fs.writeFile(
    path.join(fixture, 'entry.jsx'),
    `import React,{useReducer} from 'react';
import {createRoot} from 'react-dom/client';
import {BrowserRouter} from 'react-router-dom';
import {UserContext} from '../src/context/User';
import {reducer,initialState} from '../src/context/User/reducer';
import {StatusContext} from '../src/context/Status';
import {ThemeProvider} from '../src/context/Theme';
import {setStatusData} from '../src/helpers/data';
import HeaderBar from '../src/components/layout/headerbar';
import WalletOverview from '../src/components/topup/WalletOverview';
import '../src/index.css';
import '../src/components/topup/wallet.css';
import i18n,{initialLocaleReady} from '../src/i18n/i18n';
await initialLocaleReady;
const user={id:901,username:'fixture-account',role:1,quota:64250000,setting:JSON.stringify({quota_warning_threshold:500000})};
let status={system_name:'UI fixture',logo:'/logo.png',quota_per_unit:500000,quota_display_type:'USD',usd_exchange_rate:7,custom_currency_symbol:'⚡',custom_currency_exchange_rate:2,HeaderNavModules:JSON.stringify({home:true,console:true,docs:true,pricing:true})};
setStatusData(status);localStorage.setItem('user',JSON.stringify(user));localStorage.setItem('newapi:user-onboarding:regular:v1:user:901','done');
function Fixture(){const [userState,dispatch]=useReducer(reducer,{...initialState,user});const [statusState,setStatus]=React.useState({status});window.headerQA={setUser:u=>{if(u)localStorage.setItem('user',JSON.stringify(u));else localStorage.removeItem('user');dispatch({type:u?'login':'logout',payload:u});},setDisplay:type=>{status={...status,quota_display_type:type};setStatusData(status);setStatus({status});},setLoading:loading=>setStatus(loading?{}:{status}),language:lng=>i18n.changeLanguage(lng)};return <StatusContext.Provider value={[statusState,()=>{}]}><UserContext.Provider value={[userState,dispatch]}><BrowserRouter><ThemeProvider><HeaderBar onMobileMenuToggle={()=>{}} drawerOpen={false}/><div style={{padding:20}}><WalletOverview user={userState.user} loading={false} error={false} onRefresh={()=>dispatch({type:'login',payload:{...userState.user,quota:75000000}})} t={i18n.t.bind(i18n)}/></div></ThemeProvider></BrowserRouter></UserContext.Provider></StatusContext.Provider>};createRoot(document.getElementById('root')).render(<Fixture/>);`,
  );
  server = await createServer({
    root,
    server: { host: '127.0.0.1', port: 0 },
    plugins: [
      {
        name: 'header-qa-api',
        configureServer(s) {
          s.middlewares.use((req, res, next) => {
            if (!req.url.startsWith('/api/')) return next();
            requests.push({ method: req.method, path: req.url });
            res.setHeader('Content-Type', 'application/json');
            let data = [];
            if (req.url === '/api/status')
              data = {
                setup: true,
                system_name: 'Header QA',
                logo: '/logo.png',
                quota_per_unit: 500000,
                quota_display_type: 'USD',
                usd_exchange_rate: 7,
                server_address: 'https://example.invalid',
                password_login: true,
                register_enabled: true,
                chats: '[]',
              };
            if (req.url === '/api/user/self')
              data = {
                id: 903,
                username: 'public-fixture',
                role: 1,
                quota: 64250000,
                setting: '{}',
              };
            if (req.url === '/api/home_page_content') data = '';
            res.end(JSON.stringify({ success: true, data }));
          });
        },
      },
    ],
  });
  server.config.server.proxy = undefined;
  await server.listen();
  const origin = 'http://127.0.0.1:' + server.httpServer.address().port;
  browser = await puppeteer.launch({
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    headless: true,
  });
  const page = await browser.newPage();
  page.on('pageerror', (e) => errors.push(e.message));
  await page.setRequestInterception(true);
  page.on('request', (req) =>
    new URL(req.url()).origin === origin ? req.continue() : req.abort(),
  );
  await page.setViewport({ width: 1440, height: 900 });
  await page.goto(origin + '/' + path.basename(fixture) + '/index.html', {
    waitUntil: 'networkidle0',
  });
  await page.waitForSelector('.header-balance');
  const balance = () =>
    page.$eval('.header-balance-amount', (e) => e.textContent);
  assert.equal(await balance(), '$128.50');
  await page.waitForFunction(
    () =>
      document.querySelector('.wallet-statistics strong')?.textContent ===
      '$128.50',
  );
  const pos = await page.evaluate(() =>
    [
      document.querySelector('[aria-label="系统公告"]'),
      document.querySelector('.header-balance'),
      document.querySelector('.header-balance').nextElementSibling,
    ].map((x) => x.getBoundingClientRect().x),
  );
  assert(pos[0] < pos[1] && pos[1] < pos[2]);
  await page.screenshot({ path: path.join(output, 'desktop.png') });
  await page.click('.header-balance');
  assert(new URL(page.url()).pathname === '/console/topup');
  await page.click('.wallet-refresh');
  await page.waitForFunction(
    () =>
      document.querySelector('.header-balance-amount').textContent ===
      '$150.00',
  );
  for (const [type, value] of [
    ['CNY', '¥1050.00'],
    ['CUSTOM', '⚡300.00'],
    ['TOKENS', '75.0M'],
    ['USD', '$150.00'],
  ]) {
    await page.evaluate((type) => window.headerQA.setDisplay(type), type);
    await page.waitForFunction(
      (value) =>
        document.querySelector('.header-balance-amount').textContent === value,
      {},
      value,
    );
    assert.equal(
      await page.$eval('.wallet-statistics strong', (e) => e.textContent),
      value,
    );
  }
  const user = {
    id: 902,
    username: 'second-account',
    role: 1,
    quota: 500000,
    setting: '{}',
  };
  for (const [quota, value, low, setting] of [
    [0, '$0.00', true, '{}'],
    [-500000, '$-1.00', true, '{}'],
    [250000, '$0.50', true, '{"quota_warning_threshold":500000}'],
    [500000, '$1.00', false, '{"quota_warning_threshold":500000}'],
    [250000, '$0.50', false, '{broken'],
    [null, '—', false, '{}'],
  ]) {
    await page.evaluate((u) => window.headerQA.setUser(u), {
      ...user,
      quota,
      setting,
    });
    await page.waitForFunction(
      (value) =>
        document.querySelector('.header-balance-amount').textContent === value,
      {},
      value,
    );
    assert.equal(
      await page.$eval('.header-balance', (e) =>
        e.classList.contains('header-balance-low'),
      ),
      low,
    );
  }
  await page.evaluate((u) => window.headerQA.setUser(u), user);
  await page.waitForFunction(
    () =>
      document.querySelector('.header-balance-amount').textContent === '$1.00',
  );
  await page.evaluate(() => window.headerQA.setLoading(true));
  await page.waitForFunction(
    () => document.querySelector('.header-balance-amount').textContent === '—',
  );
  await page.evaluate(() => window.headerQA.setLoading(false));
  await page.waitForFunction(
    () =>
      document.querySelector('.header-balance-amount').textContent === '$1.00',
  );
  await page.evaluate(() => window.headerQA.setUser(null));
  await page.waitForFunction(() => !document.querySelector('.header-balance'));
  await page.evaluate((u) => window.headerQA.setUser(u), user);
  await page.waitForSelector('.header-balance');
  for (const width of [390, 320]) {
    await page.setViewport({ width, height: 844 });
    for (const quota of [64250000, 617283945000, 0]) {
      await page.evaluate((u) => window.headerQA.setUser(u), {
        ...user,
        quota,
      });
      await page.waitForFunction(
        (q) =>
          document
            .querySelector('.header-balance')
            .getAttribute('aria-label')
            .includes((q / 500000).toFixed(2)),
        {},
        quota,
      );
      const rect = await page.evaluate(() => ({
        width: innerWidth,
        scroll: document.documentElement.scrollWidth,
        right: document
          .querySelector('.public-header-actions')
          .getBoundingClientRect().right,
        balance: document
          .querySelector('.header-balance')
          .getBoundingClientRect().width,
      }));
      assert(rect.right <= width + 1, JSON.stringify(rect));
      assert.equal(rect.scroll, width);
      assert(rect.balance > 30);
    }
    await page.screenshot({
      path: path.join(output, 'mobile-' + width + '.png'),
    });
  }
  await page.evaluate(() => {
    document.documentElement.classList.add('dark');
    document.body.setAttribute('theme-mode', 'dark');
  });
  await page.screenshot({ path: path.join(output, 'mobile-dark.png') });
  await page.evaluate(() => window.headerQA.language('en'));
  await page.waitForFunction(
    () =>
      document.querySelector('.header-balance-label').textContent === 'Balance',
  );
  assert(
    !requests.some((x) => x.path.startsWith('/api/user/self')),
    'Header must not add account fetching',
  );
  assert(!requests.some((x) => x.method !== 'GET'), 'No mutations');
  assert.deepEqual(errors, []);
  console.log(
    'PASS: real header and wallet; 4 display modes; refresh, login/logout/account switch; unknown, threshold, zero/negative; 320/390px and dark. No new balance fetches or writes.',
  );
  // Public pages have a separate lightweight header. Verify its real shell too.
  await page.setViewport({ width: 1440, height: 900 });
  await page.goto(origin + '/', { waitUntil: 'networkidle0' });
  await page.waitForFunction(
    () =>
      document.querySelector('.header-balance-amount')?.textContent ===
      '$128.50',
  );
  assert.equal(
    await page.$eval('.header-balance', (e) => e.getAttribute('href')),
    '/console/topup',
  );
  await page.screenshot({ path: path.join(output, 'public-desktop.png') });
  for (const width of [390, 320]) {
    await page.setViewport({ width, height: 844 });
    const bounds = await page.$eval('.public-header-actions', (e) => ({
      left: e.getBoundingClientRect().left,
      right: e.getBoundingClientRect().right,
      width: innerWidth,
      scroll: document.documentElement.scrollWidth,
    }));
    assert(bounds.right <= width + 1, JSON.stringify(bounds));
    assert.equal(bounds.scroll, width);
    await page.screenshot({
      path: path.join(output, 'public-mobile-' + width + '.png'),
    });
  }
  assert.deepEqual(errors, []);
  assert(!requests.some((x) => x.method !== 'GET'), 'No public header writes');
  console.log(
    'PASS: separate public header, existing account fetch, English labels and 320/390px layout.',
  );
  console.log('Screenshots:', output);
} finally {
  if (browser) await browser.close();
  if (server) await server.close();
  stop();
  await fs.rm(fixture, { recursive: true, force: true });
}

process.exit(0);
