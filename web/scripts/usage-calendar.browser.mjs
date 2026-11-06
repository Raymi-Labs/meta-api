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
const fixture = await fs.mkdtemp(path.join(root, '.usage-calendar-qa-'));
const output =
  process.env.CALENDAR_OUTPUT ||
  (await fs.mkdtemp(path.join(os.tmpdir(), 'usage-calendar-')));
let browser, server;
try {
  await fs.writeFile(
    path.join(fixture, 'index.html'),
    '<html><meta name="viewport" content="width=device-width,initial-scale=1"><div id="root"></div><script type="module" src="./main.jsx"></script></html>',
  );
  await fs.writeFile(
    path.join(fixture, 'main.jsx'),
    `import React from 'react';import{createRoot}from'react-dom/client';import{useTranslation}from'react-i18next';import UsageCalendar from '../src/components/dashboard/UsageCalendar';import{initialLocaleReady}from '../src/i18n/i18n';import '../src/index.css';await initialLocaleReady;function App(){const{t}=useTranslation();const[id,setId]=React.useState(1);return <div style={{padding:'24px',maxWidth:'1200px',margin:'auto'}}><h1 style={{fontSize:24,marginBottom:24}}>我的用量</h1><UsageCalendar key={id} t={t} userId={id}/><button data-switch onClick={()=>setId(id+1)}>Switch fixture account</button></div>}createRoot(document.getElementById('root')).render(<App/>);`,
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
  const base = `http://127.0.0.1:${server.httpServer.address().port}`;
  browser = await puppeteer.launch({
    headless: true,
    executablePath:
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  });
  const page = await browser.newPage();
  page.setDefaultTimeout(10000);
  await page.setViewport({ width: 1440, height: 900 });
  await page.evaluateOnNewDocument(() => {
    localStorage.setItem('site-language', 'zh-CN');
    localStorage.setItem('user', '{"id":1,"role":1}');
  });
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  let calls = 0,
    mode = 'data';
  await page.setRequestInterception(true);
  page.on('request', async (req) => {
    const u = new URL(req.url());
    if (u.protocol === 'data:' || u.protocol === 'blob:') return req.continue();
    if (u.origin !== base) return req.abort();
    if (u.pathname !== '/api/data/self/activity') return req.continue();
    calls++;
    if (mode === 'error')
      return req.respond({
        status: 503,
        contentType: 'application/json',
        body: '{"success":false}',
      });
    const now = new Date();
    const midnight = Date.UTC(
      now.getUTCFullYear(),
      now.getUTCMonth(),
      now.getUTCDate(),
    );
    const days = Array.from({ length: 365 }, (_, i) => ({
      date: new Date(midnight - (364 - i) * 86400000)
        .toISOString()
        .slice(0, 10),
      requests:
        mode === 'empty'
          ? 0
          : i % 11 === 0
            ? 0
            : i < 90
              ? (i % 4) * 3
              : (i % 7) * 14,
    }));
    let streak = 0,
      longest = 0,
      total = 0;
    for (const d of days) {
      total += d.requests;
      streak = d.requests ? streak + 1 : 0;
      longest = Math.max(longest, streak);
    }
    await req.respond({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: {
          source: 'retained_settlement_logs_v1',
          timezone: u.searchParams.get('timezone'),
          as_of: Math.floor(Date.now() / 1000),
          days,
          total,
          longest_streak: longest,
          daily_average: total / 365,
          weekly_average: (total * 7) / 365,
        },
      }),
    });
  });
  await page.goto(`${base}/${path.basename(fixture)}/index.html`, {
    waitUntil: 'networkidle0',
  });
  assert.equal(calls, 0);
  await page.click('.usage-calendar-toggle');
  await page.waitForSelector('.usage-calendar-day');
  assert.equal(calls, 1);
  assert.equal(
    await page.$$eval('.usage-calendar-day', (els) => els.length),
    365,
  );
  await page.focus('.usage-calendar-day');
  await page.keyboard.press('Enter');
  assert(
    await page.$eval('.usage-calendar-footer', (e) =>
      /\d{4}-\d{2}-\d{2}/.test(e.textContent),
    ),
  );
  await page.evaluate(
    () => (document.querySelector('[data-switch]').style.visibility = 'hidden'),
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
    path: path.join(output, 'mobile.png'),
    fullPage: true,
  });
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  );
  mode = 'error';
  await page.click('.usage-calendar-meta button');
  await page.waitForSelector('[role=alert]');
  assert.equal(await page.$('.usage-calendar-stats'), null);
  mode = 'empty';
  await page.click('.usage-calendar-error button');
  await page.waitForSelector('.usage-calendar-empty');
  await page.evaluate(
    () =>
      (document.querySelector('[data-switch]').style.visibility = 'visible'),
  );
  await page.click('[data-switch]');
  await page.waitForFunction(
    () =>
      document
        .querySelector('.usage-calendar-toggle')
        .getAttribute('aria-expanded') === 'false',
  );
  assert.equal(await page.$('.usage-calendar-stats'), null);
  assert.deepEqual(errors, []);
  console.log(
    JSON.stringify({
      passed: true,
      output,
      calls,
      checks:
        'lazy-load,365 dates,keyboard,mobile,dark,error,empty,account-remount',
    }),
  );
} finally {
  await browser?.close();
  await server?.close();
  stop();
  await fs.rm(fixture, { recursive: true, force: true });
}
