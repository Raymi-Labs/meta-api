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
    `import React from 'react';
import {createRoot} from 'react-dom/client';
import {BrowserRouter} from 'react-router-dom';
import i18n from 'i18next';
import {API} from '/src/helpers/apiCore';
import Ticket from '/src/pages/Ticket';
import Admin from '/src/pages/Ticket/TicketAdmin';
import Appeal from '/src/pages/Ticket/Appeal';
import {useNotifications} from '/src/hooks/common/useNotifications';
import '/src/index.css';
i18n.init({lng:'zh',resources:{zh:{translation:{}}}});
localStorage.setItem('user',JSON.stringify({id:1,role:100}));
window.calls=[];
const item={id:1,user_id:2,subject:'QA ticket',category:'api',status:'open',created_at:1,request_id:'req-test'};
API.get=async(url,config)=>{window.calls.push({method:'GET',url,config});return {data:{success:true,data:url.endsWith('/session')?{user_id:1,username:'QA',expires_at:9999999999}:url.endsWith('/attachments')?[]:url.endsWith('/unread')?{count:2}:/\\/1$/.test(url)?{ticket:item,messages:[{id:1,sender_role:'user',body:'hello',created_at:1}],logs:[{id:1,request_id:'req-test'}]}:{tickets:url==='/api/appeal/tickets'?[]:[item],total:url==='/api/appeal/tickets'?0:1}}};};
API.post=async(url,body)=>{window.calls.push({method:'POST',url,body:JSON.parse(JSON.stringify(body))});return {data:{success:true,data:{id:1}}};};
API.put=async(url,body)=>{window.calls.push({method:'PUT',url,body});return {data:{success:true}};};
function Notices(){const result=useNotifications({status:{}},{user:{id:1,role:100}});return <div id='counts'>{result.ticketUnread}:{result.staffTicketUnread}</div>;}
const mode=new URLSearchParams(location.search).get('mode');
createRoot(document.getElementById('root')).render(<BrowserRouter><Notices/>{mode==='admin'?<Admin/>:mode==='appeal'?<Appeal/>:<Ticket/>}</BrowserRouter>);
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
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  });
  const page = await browser.newPage();
  page.setDefaultTimeout(30000);
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  const url =
    server.resolvedUrls.local[0] + path.basename(fixture) + '/index.html';
  const click = async (text) => {
    console.log('click', text);
    await page.waitForFunction(
      (text) =>
        [...document.querySelectorAll('button,a,span')].some(
          (e) => e.textContent.trim() === text,
        ),
      {},
      text,
    );
    await page.evaluate((text) => {
      const e = [...document.querySelectorAll('button,a,span')].find(
        (e) => e.textContent.trim() === text,
      );
      e.click();
    }, text);
  };
  await page.goto(url + '?create=1&request_id=req-test');
  await page.waitForSelector('input');
  await page.waitForFunction(
    () => document.querySelector('#counts')?.textContent === '2:2',
  );
  await page.waitForFunction(() =>
    [...document.querySelectorAll('input')].some((e) => e.value === 'req-test'),
  );
  assert.deepEqual(errors, [], 'user form runtime');
  await page.type('input', 'normal subject');
  await page.click('.semi-select');
  await page.waitForSelector('.semi-select-option');
  await page.$eval('.semi-select-option', (el) => el.click());
  await page.waitForFunction(() =>
    document.querySelector('.semi-select').textContent.includes('计费问题'),
  );
  await page.type('textarea', 'normal body\nsecond line');
  await page.click('.semi-modal-footer button:last-child');

  await page.waitForFunction(() =>
    window.calls.some(
      (c) =>
        c.url === '/api/ticket' &&
        c.method === 'POST' &&
        !!c.body.category &&
        c.body.request_id === 'req-test',
    ),
  );

  await page.goto(url + '?id=1');
  await page.waitForSelector('textarea');
  await page.type('textarea', 'first line\nsecond line');
  await click('发送回复');
  await page.waitForFunction(() =>
    window.calls.some(
      (c) => c.url === '/api/ticket/1/reply' && c.body.body.includes('\n'),
    ),
  );
  await page.goto(url + '?mode=admin');
  await click('QA ticket');
  await page.waitForSelector('textarea');
  await click('关联请求日志 (1)');
  await page.type('textarea', 'staff\nreply');
  await click('发送回复');
  await page.waitForFunction(() =>
    window.calls.some((c) => c.url === '/api/admin/ticket/1/reply'),
  );
  assert.deepEqual(errors, [], 'admin details runtime including logs');
  await page.goto(url + '?mode=appeal');
  await click('创建申诉工单');
  await page.waitForSelector('textarea');
  await page.type('input', 'appeal subject');
  await page.type('textarea', 'appeal body');
  await page.click('.semi-modal-footer button:last-child');
  await page.waitForFunction(() =>
    window.calls.some(
      (c) =>
        c.url === '/api/appeal/tickets' &&
        c.method === 'POST' &&
        c.body.subject === 'appeal subject',
    ),
  );
  assert.deepEqual(errors, [], 'appeal runtime');
  console.log(
    'Ticket browser flows passed: forms, multiline reply, logs, appeal payload, request ID, unread counts.',
  );
} finally {
  await browser?.close();
  await server?.close();
  stopEsbuild();
  await fs.rm(fixture, { recursive: true, force: true });
}
