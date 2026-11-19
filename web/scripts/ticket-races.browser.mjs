import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';
import { createServer, loadConfigFromFile } from 'vite';
import { stop as stopEsbuild } from 'esbuild';
const root = fileURLToPath(new URL('..', import.meta.url));
const fixture = await fs.mkdtemp(path.join(root, '.ticket-qa-'));
let server, browser;
try {
  await fs.writeFile(
    path.join(fixture, 'index.html'),
    '<html><div id="root"></div><script type="module" src="./entry.jsx"></script></html>',
  );
  await fs.writeFile(
    path.join(fixture, 'entry.jsx'),
    `import React,{useState} from 'react';
import {createRoot} from 'react-dom/client';
import {BrowserRouter} from 'react-router-dom';
import i18n from 'i18next';
import {API} from '/src/helpers/apiCore';
import Admin from '/src/pages/Ticket/TicketAdmin';
import {useNotifications} from '/src/hooks/common/useNotifications';
import '/src/index.css';
i18n.init({lng:'zh',resources:{zh:{translation:{}}}});
localStorage.setItem('user',JSON.stringify({id:1,role:10}));
window.unread=[];window.staff=[];window.unreadConfig=[];window.lists=[];
API.get=(url,config)=>{
 if(url==='/api/ticket/unread'){window.unreadConfig.push(config);return new Promise(resolve=>window.unread.push(resolve));}
 if(url==='/api/admin/ticket/unread')return new Promise(resolve=>window.staff.push(resolve));
 if(url==='/api/admin/ticket')return new Promise(resolve=>window.lists.push({status:config.params.status,resolve}));
 return Promise.resolve({data:{success:true,data:{count:0,items:[]}}});
};
function Notices(){const [user,setUser]=useState({id:1,role:1});window.switchUser=setUser;const r=useNotifications({status:{}},{user});return <div id='counts'>{r.ticketUnread}:{r.staffTicketUnread}</div>;}
createRoot(document.getElementById('root')).render(<BrowserRouter>{location.search.includes('filter')?<Admin/>:<Notices/>}</BrowserRouter>);
`,
  );
  const { config } = await loadConfigFromFile(
    { command: 'serve', mode: 'development' },
    path.join(root, 'vite.config.js'),
  );
  server = await createServer({
    ...config,
    configFile: false,
    root,
    server: {
      host: '127.0.0.1',
      port: 0,
      open: false,
      proxy: {},
      fs: { allow: [root, path.resolve(root, 'node_modules')] },
    },
    plugins: config.plugins
      .flat(Infinity)
      .filter((p) => p?.name !== '@code-inspector/vite'),
  });
  await server.listen();
  browser = await puppeteer.launch({
    headless: true,
    args: process.env.CI ? ['--no-sandbox'] : [],
    executablePath:
      process.env.CHROME_PATH ||
      process.env.PUPPETEER_EXECUTABLE_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  });
  const page = await browser.newPage();
  page.setDefaultTimeout(30000);
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  const url =
    server.resolvedUrls.local[0] + path.basename(fixture) + '/index.html';
  await page.goto(url);
  await page.waitForFunction(() => window.unread.length === 1);
  await page.evaluate(() => window.switchUser({ id: 2, role: 1 }));
  await page.waitForFunction(() => window.unread.length === 2);
  await page.evaluate(() =>
    window.unread[1]({ data: { success: true, data: { count: 2 } } }),
  );
  await page.waitForFunction(
    () => document.querySelector('#counts').textContent === '2:0',
  );
  await page.evaluate(async () => {
    window.unread[0]({ data: { success: true, data: { count: 99 } } });
    await new Promise((r) =>
      requestAnimationFrame(() => requestAnimationFrame(r)),
    );
  });
  assert.equal(
    await page.$eval('#counts', (e) => e.textContent),
    '2:0',
    'old identity response ignored',
  );
  assert.equal(
    await page.evaluate(() =>
      window.unreadConfig.every((c) => c.disableDuplicate),
    ),
    true,
    'identity requests cannot share the API GET cache',
  );
  await page.evaluate(() => window.switchUser({ id: 3, role: 1 }));
  await page.waitForFunction(() => window.unread.length === 3);
  await page.evaluate(() => window.switchUser(null));
  await page.waitForFunction(
    () => document.querySelector('#counts').textContent === '0:0',
  );
  await page.evaluate(async () => {
    window.unread[2]({ data: { success: true, data: { count: 88 } } });
    await new Promise((r) =>
      requestAnimationFrame(() => requestAnimationFrame(r)),
    );
  });
  assert.equal(
    await page.$eval('#counts', (e) => e.textContent),
    '0:0',
    'logout invalidates in-flight requests',
  );
  await page.evaluate(() => window.switchUser({ id: 4, role: 10 }));
  await page.waitForFunction(() => window.unread.length === 4);
  await page.evaluate(() =>
    window.unread[3]({ data: { success: true, data: { count: 4 } } }),
  );
  await page.waitForFunction(() => window.staff.length === 1);
  await page.evaluate(() => window.switchUser({ id: 5, role: 1 }));
  await page.waitForFunction(() => window.unread.length === 5);
  await page.evaluate(() =>
    window.unread[4]({ data: { success: true, data: { count: 5 } } }),
  );
  await page.waitForFunction(
    () => document.querySelector('#counts').textContent === '5:0',
  );
  await page.evaluate(async () => {
    window.staff[0]({ data: { success: true, data: { count: 77 } } });
    await new Promise((r) =>
      requestAnimationFrame(() => requestAnimationFrame(r)),
    );
  });
  assert.equal(
    await page.$eval('#counts', (e) => e.textContent),
    '5:0',
    'late administrator response ignored',
  );
  await page.goto(url + '?filter=1');
  await page.waitForFunction(() => window.lists.length === 1);
  await page.evaluate(() =>
    window.lists[0].resolve({
      data: { success: true, data: { tickets: [], total: 0 } },
    }),
  );
  const choose = async (label) => {
    await page.click('.semi-select');
    await page.waitForSelector('.semi-select-option');
    await page.$$eval(
      '.semi-select-option',
      (options, label) =>
        options.find((o) => o.textContent.trim() === label).click(),
      label,
    );
  };
  await choose('已关闭');
  await page.waitForFunction(() =>
    window.lists.some((r) => r.status === 'closed'),
  );
  await choose('待处理');
  await page.waitForFunction(() =>
    window.lists.some((r) => r.status === 'open'),
  );
  await page.evaluate(() =>
    window.lists
      .find((r) => r.status === 'open')
      .resolve({
        data: {
          success: true,
          data: {
            tickets: [
              { id: 2, subject: 'OPEN-RESULT', status: 'open', created_at: 1 },
            ],
            total: 1,
          },
        },
      }),
  );
  await page.waitForFunction(() =>
    document.querySelector('body').textContent.includes('OPEN-RESULT'),
  );
  await page.evaluate(async () => {
    window.lists
      .find((r) => r.status === 'closed')
      .resolve({
        data: {
          success: true,
          data: {
            tickets: [
              {
                id: 1,
                subject: 'CLOSED-RESULT',
                status: 'closed',
                created_at: 1,
              },
            ],
            total: 1,
          },
        },
      });
    await new Promise((r) =>
      requestAnimationFrame(() => requestAnimationFrame(r)),
    );
  });
  assert.equal(
    await page.evaluate(() =>
      document.body.textContent.includes('OPEN-RESULT'),
    ),
    true,
  );
  assert.equal(
    await page.evaluate(() =>
      document.body.textContent.includes('CLOSED-RESULT'),
    ),
    false,
    'old filter response ignored',
  );
  assert.equal(
    await page.$eval('.semi-select', (e) => e.textContent.includes('待处理')),
    true,
  );
  console.log(
    'Ticket async regressions passed: account switch, logout, late admin response, filter response ordering.',
  );
  assert.deepEqual(errors, []);
} finally {
  await browser?.close();
  await server?.close();
  stopEsbuild();
  await fs.rm(fixture, { recursive: true, force: true });
}
