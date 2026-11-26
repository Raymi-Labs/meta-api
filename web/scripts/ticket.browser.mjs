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
import SiderBar from '/src/components/layout/SiderBar';
import {StatusProvider} from '/src/context/Status';
import Appeal from '/src/pages/Ticket/Appeal';
import {useNotifications} from '/src/hooks/common/useNotifications';
import '/src/index.css';
i18n.init({lng:'zh',resources:{zh:{translation:{}}}});
localStorage.setItem('user',JSON.stringify({id:1,role:100}));
window.calls=[];
const item={id:1,user_id:2,subject:'QA ticket',category:'api',status:new URLSearchParams(location.search).get('state')||'open',created_at:1,request_id:'req-test'};
const files=[];
const png = window.testPNG = Uint8Array.from(atob('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9WL5CQsAAAAASUVORK5CYII='), c => c.charCodeAt(0));
const messages=[{id:1,sender_role:'user',body:'**hello**',created_at:1},{id:2,sender_role:'system',sender_id:0,event:'status_waiting_user',body:'hidden system activity',created_at:1}];
API.get=async(url,config)=>{
  window.calls.push({method:'GET',url,config});
  if (url.endsWith('/download')) return {data:new Blob([png],{type:'image/png'})};
  if (url === '/api/admin/ticket/unread' && mode === 'admin') {
    return {data:{success:true,data:{count:0,pending_count:item.status==='open'?1:0}}};
  }
  return {data:{success:true,data:url.endsWith('/session')?{user_id:1,username:'QA',expires_at:9999999999}:url.endsWith('/attachments')?files:url.endsWith('/unread')?{count:2}:/\\/1$/.test(url)?{ticket:item,messages,logs:[{id:1,request_id:'req-test'}]}:{tickets:url==='/api/appeal/tickets'&&!location.search.includes('state=')?[]:[item],total:url==='/api/appeal/tickets'&&!location.search.includes('state=')?0:1}}};};
API.post=async(url,body)=>{
  if (url==='/api/admin/ticket/1/reply') item.status='waiting_user';
  if(body instanceof FormData && url.endsWith('/reply')){
    const file=body.get('file');
    body={body:body.get('body'),filename:file.name,internal:body.get('is_internal')};
    if (window.failReply) {window.failReply=false; return {data:{success:false,message:'retry'}};}
    messages.push({id:3,sender_role:'user',...body,attachments:[{id:3,filename:file.name,mime_type:file.type}],created_at:1});
  }
  if(body instanceof FormData){files.push({id:1,filename:body.get('file').name});item.status='open';body={ticket_id:body.get('ticket_id'),filename:body.get('file').name};}window.calls.push({method:'POST',url,body:JSON.parse(JSON.stringify(body))});return {data:{success:true,data:{id:1}}};};
API.put=async(url,body)=>{window.calls.push({method:'PUT',url,body});return {data:{success:true}};};
function Notices() {
  const result = useNotifications({status:{}},{user:{id:1,role:100}});
  return <>
    <div id='counts'>{result.ticketUnread}:{result.staffTicketUnread}</div>
    {mode==='admin' && <StatusProvider>
      <SiderBar hasPendingStaffTickets={result.hasPendingStaffTickets}/>
    </StatusProvider>}
  </>;
}
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
  const dot = 'a[href="/console/ticket-admin"] .ticket-pending-dot';
  await page.waitForSelector(dot, { visible: true });
  await page.click('.sidebar-collapse-button button');
  await page.waitForSelector('body.sidebar-collapsed ' + dot, {
    visible: true,
  });

  await click('QA ticket');
  await page.waitForSelector('textarea');
  await click('关联请求日志 (1)');
  await page.type('textarea', 'staff\nreply');
  await click('发送回复');
  await page.waitForFunction(() =>
    window.calls.some((c) => c.url === '/api/admin/ticket/1/reply'),
  );
  await page.waitForSelector(dot, { hidden: true });
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
  await fs.writeFile(path.join(fixture, 'evidence.txt'), 'new evidence');
  for (const mode of ['', 'appeal']) {
    await page.goto(url + '?mode=' + mode + '&id=1&state=resolved');
    if (mode === 'appeal') await click('QA ticket');
    await page.waitForSelector('input[type=file]');
    await page.waitForFunction(() =>
      document
        .querySelector('.semi-modal-body')
        ?.textContent.includes('已解决'),
    );
    const input = await page.$('input[type=file]');
    await input.uploadFile(path.join(fixture, 'evidence.txt'));
    await page.waitForFunction(
      () =>
        document
          .querySelector('.semi-modal-body')
          ?.textContent.includes('evidence.txt') &&
        document
          .querySelector('.semi-modal-body')
          ?.textContent.includes('待处理') &&
        window.calls.filter((c) => c.method === 'GET' && /\/1$/.test(c.url))
          .length >= 2,
    );
    assert.equal(
      await page.evaluate(() =>
        window.calls.some(
          (c) =>
            c.method === 'POST' &&
            c.body.filename === 'evidence.txt' &&
            c.body.ticket_id === '1',
        ),
      ),
      true,
    );
  }
  for (const mode of ['user', 'admin', 'appeal']) {
    await page.goto(url + '?id=1&state=open&mode=' + mode);
    if (mode === 'appeal' || mode === 'admin') await click('QA ticket');
    await page.waitForSelector('textarea');
    await page.waitForSelector('.ticket-message strong');
    assert.equal(
      await page.evaluate(() =>
        document
          .querySelector('.semi-modal-body')
          .innerText.includes('hidden system activity'),
      ),
      false,
    );
    const replyPath = {
      user: '/api/ticket/1/reply',
      admin: '/api/admin/ticket/1/reply',
      appeal: '/api/appeal/tickets/1/reply',
    }[mode];
    if (mode === 'user') {
      const filePath = path.join(fixture, 'picked.png');
      await fs.writeFile(
        filePath,
        Buffer.from(
          'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9WL5CQsAAAAASUVORK5CYII=',
          'base64',
        ),
      );
      const input = await page.$('input[accept^="image/png"]');
      await input.uploadFile(filePath);
      await page.waitForSelector('img[alt="picked.png"]');
      await click('移除');
      await page.waitForSelector('img[alt="picked.png"]', { hidden: true });
    }
    if (mode === 'admin') await click('内部备注（用户不可见）');
    await page.evaluate((mode) => {
      const field = document.querySelector('textarea');
      const data = new DataTransfer();
      data.items.add(
        new File([window.testPNG], 'paste.png', { type: 'image/png' }),
      );
      if (mode !== 'user')
        data.setData(
          'text/html',
          '<p><strong>formatted</strong></p><ul><li>list</li></ul>',
        );
      field.dispatchEvent(
        new ClipboardEvent('paste', {
          clipboardData: data,
          bubbles: true,
          cancelable: true,
        }),
      );
      window.failReply = true;
    }, mode);
    await page.waitForSelector('img[alt="paste.png"]');
    assert.equal(
      await page.evaluate(() => window.calls.some((c) => c.method === 'POST')),
      false,
    );
    const gap = await page.evaluate(() => {
      const button = [...document.querySelectorAll('button')].find(
        (b) => b.innerText === '发送回复',
      );
      button.scrollIntoView({ block: 'end' });
      document.querySelector('.semi-modal-body').scrollTop = 1e6;
      return (
        document.querySelector('.semi-modal-content').getBoundingClientRect()
          .bottom - button.getBoundingClientRect().bottom
      );
    });
    assert.ok(gap >= 20, 'reply footer spacing ' + gap);
    await click('发送回复');
    await page.waitForFunction(() => window.failReply === false);
    await page.waitForSelector('img[alt="paste.png"]');
    await click('发送回复');
    await page.waitForFunction(
      (path) =>
        window.calls.some(
          (c) => c.url === path && c.body.filename === 'paste.png',
        ),
      {},
      replyPath,
    );
    const sent = await page.evaluate(
      (path) =>
        window.calls.find((c) => c.url === path && c.method === 'POST').body,
      replyPath,
    );
    if (mode === 'user') assert.equal(sent.body, '');
    else assert.match(sent.body, /\*\*formatted\*\*/);
    if (mode === 'admin') assert.equal(sent.internal, 'true');
    await page.waitForFunction(
      () =>
        document.querySelector('.ticket-message img[alt="paste.png"]')
          ?.naturalWidth === 1,
    );
  }
  assert.deepEqual(errors, [], 'appeal runtime');
  console.log(
    'Ticket browser flows passed: forms, multiline reply, logs, appeal payload, request ID, unread counts, attachment status refresh, image replies, rich paste, footer spacing, hidden system events.',
  );
} finally {
  await browser?.close();
  await server?.close();
  stopEsbuild();
  await fs.rm(fixture, { recursive: true, force: true });
}
