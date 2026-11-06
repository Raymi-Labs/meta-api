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
const fixture = await fs.mkdtemp(path.join(root, '.personal-overview-qa-'));
const output =
  process.env.OVERVIEW_OUTPUT ||
  (await fs.mkdtemp(path.join(os.tmpdir(), 'personal-overview-')));
let server, browser;
try {
  await fs.writeFile(
    path.join(fixture, 'index.html'),
    '<html><meta name="viewport" content="width=device-width,initial-scale=1"><div id="root"></div><script type="module" src="./main.jsx"></script></html>',
  );
  await fs.writeFile(
    path.join(fixture, 'main.jsx'),
    `import React from'react';import{createRoot}from'react-dom/client';import{MemoryRouter,Routes,Route}from'react-router-dom';import{UserContext}from'../src/context/User';import{StatusContext}from'../src/context/Status';import{ThemeProvider}from'../src/context/Theme';import{Layout}from'@douyinfe/semi-ui';import{changeSiteLanguage}from'../src/helpers/siteLanguage';import Sidebar from'../src/components/layout/SiderBar';import Overview from'../src/pages/PersonalOverview';import{initialLocaleReady}from'../src/i18n/i18n';import'../src/index.css';await initialLocaleReady;function App(){const[user,setUser]=React.useState({id:1,username:'demo-user',role:1});window.fixtureAccount=()=>{localStorage.setItem('user',JSON.stringify({id:2,role:1}));setUser({id:2,username:'second-user',role:1})};window.fixtureLanguage=changeSiteLanguage;return <UserContext.Provider value={[{user},()=>{}]}><StatusContext.Provider value={[{status:{system_name:'元衡',SidebarModulesAdmin:JSON.stringify({personal:{enabled:true,topup:true,personal:true,pulse:true}})}},()=>{}]}><ThemeProvider><MemoryRouter initialEntries={['/console/overview']}><Layout.Sider className="fixture-sidebar app-sider"><Sidebar/></Layout.Sider><div className="fixture-main"><Routes><Route path="/console/overview" element={<Overview/>}/><Route path="*" element={<div data-destination>Existing destination</div>}/></Routes></div></MemoryRouter></ThemeProvider></StatusContext.Provider></UserContext.Provider>};createRoot(document.getElementById('root')).render(<App/>);`,
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
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  });
  const page = await browser.newPage();
  page.setDefaultTimeout(12000);
  await page.setViewport({ width: 1440, height: 1100 });
  await page.evaluateOnNewDocument(() => {
    localStorage.setItem('site-language', 'zh-CN');
    localStorage.setItem('user', '{"id":1,"role":1}');
    localStorage.setItem('enable_data_export', 'true');
    localStorage.setItem('default_collapse_sidebar', 'false');
    localStorage.setItem('quota_per_unit', '500000');
    localStorage.setItem('quota_display_type', 'CNY');
    localStorage.setItem('price', '1');
  });
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  let monthlyFail = false;
  let held = null;
  let holdMonth = false;
  await page.setRequestInterception(true);
  page.on('request', async (req) => {
    const u = new URL(req.url());
    if (u.protocol === 'data:' || u.protocol === 'blob:') return req.continue();
    if (u.origin !== origin) return req.abort();
    if (!u.pathname.startsWith('/api/')) return req.continue();
    const id = Number(req.headers()['new-api-user'] || 1);
    let data = {};
    const now = new Date();
    const asof = Math.floor(Date.now() / 1000);
    if (u.pathname === '/api/user/self')
      data = {
        id,
        username: id === 1 ? 'demo-user' : 'second-user',
        display_name: id === 1 ? '元衡体验用户' : '第二个账号',
        email: 'demo-person@example.test',
        role: 1,
        sidebar_modules: JSON.stringify({
          personal: { enabled: true, topup: true, personal: true, pulse: true },
        }),
      };
    if (u.pathname === '/api/data/self/month') {
      if (holdMonth) {
        held = req;
        return;
      }
      if (monthlyFail)
        return req.respond({
          status: 503,
          contentType: 'application/json',
          body: '{"success":false}',
        });
      data = {
        month: '2026-11',
        timezone: u.searchParams.get('timezone'),
        as_of: asof,
        active_days: 5,
        summary: {
          source: 'retained_settlement_logs_v1',
          quota: 564773,
          requests: 1248,
          tokens: 2834190,
        },
      };
    }
    if (u.pathname === '/api/data/self/tokens')
      data =
        id === 1
          ? {
              total: 8,
              enabled: 6,
              other: 2,
              as_of: asof,
              recent: [
                {
                  id: 8,
                  name: '日常编程',
                  masked_key: '••••F7Db',
                  status: 'enabled',
                  created_time: asof - 86400,
                },
                {
                  id: 7,
                  name: '图片创作',
                  masked_key: '••••m3Qa',
                  status: 'enabled',
                  created_time: asof - 86400 * 3,
                },
                {
                  id: 6,
                  name: '旧测试令牌',
                  masked_key: '••••r8Ke',
                  status: 'disabled',
                  created_time: asof - 86400 * 7,
                },
              ],
            }
          : { total: 0, enabled: 0, other: 0, as_of: asof, recent: [] };
    if (u.pathname === '/api/data/self/activity') {
      const midnight = Date.UTC(
        now.getUTCFullYear(),
        now.getUTCMonth(),
        now.getUTCDate(),
      );
      let total = 0,
        streak = 0,
        longest = 0;
      const days = Array.from({ length: 365 }, (_, i) => ({
        date: new Date(midnight - (364 - i) * 86400000)
          .toISOString()
          .slice(0, 10),
        requests: i % 9 === 0 ? 0 : i < 150 ? (i % 3) * 2 : (i % 7) * 15,
      }));
      for (const d of days) {
        total += d.requests;
        streak = d.requests ? streak + 1 : 0;
        longest = Math.max(longest, streak);
      }
      data = {
        source: 'retained_settlement_logs_v1',
        timezone: u.searchParams.get('timezone'),
        as_of: asof,
        days,
        total,
        longest_streak: longest,
        daily_average: total / 365,
        weekly_average: (total * 7) / 365,
      };
    }
    return req.respond({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, data }),
    });
  });
  const url = `${origin}/${path.basename(fixture)}/index.html`;
  await page.goto(url, { waitUntil: 'networkidle0' });
  await page.addStyleTag({
    content:
      '.fixture-sidebar{position:fixed;inset:0 auto 0 0;width:210px;border-right:1px solid var(--semi-color-border)}.fixture-sidebar .semi-navigation{width:210px!important}.fixture-main{margin-left:210px;background:var(--semi-color-bg-0);min-height:100vh}@media(max-width:700px){.fixture-sidebar{display:none}.fixture-main{margin-left:0}}',
  });
  await page.waitForSelector('.personal-overview tbody tr');
  await page.waitForSelector('.usage-calendar-day');
  assert.equal(
    await page.$$eval('.personal-overview tbody tr', (e) => e.length),
    3,
  );
  const text = await page.$eval('.personal-overview', (e) => e.textContent);
  assert(text.includes('d***@example.test'));
  assert(!text.includes('demo-person@'));
  assert.equal(
    await page.$$eval(
      '.personal-overview-metrics strong',
      (els) => els[1].textContent,
    ),
    '1,248',
  );
  assert(
    await page.$eval('.fixture-sidebar', (el) =>
      el.textContent.includes('个人总览'),
    ),
    'old sidebar config should show new default item',
  );
  const overviewLink = await page.$(
    '.fixture-sidebar a[href="/console/overview"]',
  );
  assert(overviewLink);
  const overviewBox = await overviewLink.boundingBox();
  assert(
    overviewBox && overviewBox.height <= 60 && overviewBox.y < 1000,
    'overview entry must be visible, not stretched below viewport',
  );

  await page.click('.sidebar-collapse-button');
  await page.waitForFunction(() =>
    document.body.classList.contains('sidebar-collapsed'),
  );
  assert(
    await page.$eval(
      '.sidebar-nav .semi-navigation-item-collapsed',
      (el) => el.getBoundingClientRect().height <= 50,
    ),
  );
  await page.click('.sidebar-collapse-button');
  await page.waitForFunction(
    () => !document.body.classList.contains('sidebar-collapsed'),
  );
  await page.screenshot({
    path: path.join(output, 'desktop.png'),
    fullPage: true,
  });
  await page.setViewport({ width: 390, height: 844 });
  await page.evaluate(() => {
    document.documentElement.classList.add('dark');
    document.body.setAttribute('theme-mode', 'dark');
  });
  await page.screenshot({
    path: path.join(output, 'mobile-dark.png'),
    fullPage: true,
  });
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  );
  await page.evaluate(() => window.fixtureLanguage('en'));
  await page.waitForFunction(
    () => document.querySelector('h1')?.textContent === 'Personal overview',
  );
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  );
  monthlyFail = true;
  holdMonth = true;
  await page.evaluate(() => window.fixtureAccount());
  await page.waitForSelector('.personal-overview-empty');
  assert(
    !(await page.$eval('.personal-overview', (e) =>
      e.textContent.includes('F7Db'),
    )),
  );
  const deadline = Date.now() + 10000;
  while (!held && Date.now() < deadline)
    await new Promise((r) => setTimeout(r, 20));
  assert(held);
  await held.respond({
    status: 503,
    contentType: 'application/json',
    body: '{"success":false}',
  });
  holdMonth = false;
  await page.waitForSelector('.personal-overview-month [role=alert]');
  assert(
    await page.$('.usage-calendar-day'),
    'monthly error must not erase calendar',
  );
  monthlyFail = false;
  await page.click('.personal-overview-month [role=alert] button');
  await page.waitForSelector('.personal-overview-metrics');
  await page.click('.personal-overview-tokens a[href="/console/token"]');
  await page.waitForSelector('[data-destination]');
  assert.deepEqual(errors, []);
  console.log(
    JSON.stringify({
      passed: true,
      output,
      checks:
        'old-menu-default,month,masked-email,tokens,calendar,mobile,dark,english,identity-reset,isolated-error,retry,existing-token-route',
    }),
  );
} finally {
  await browser?.close();
  await server?.close();
  stop();
  await fs.rm(fixture, { recursive: true, force: true });
}
