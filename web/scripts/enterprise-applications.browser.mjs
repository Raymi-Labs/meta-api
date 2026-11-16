import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';
import { createServer, loadConfigFromFile } from 'vite';
import { stop } from 'esbuild';
const root = fileURLToPath(new URL('..', import.meta.url));
const name = `.enterprise-applications-qa-${process.pid}`;
const files = ['html', 'jsx'].map((ext) => path.join(root, `${name}.${ext}`));
const artifacts = fs.mkdtempSync(
  path.join(os.tmpdir(), 'enterprise-applications-'),
);
let server, browser;
const apps = [],
  errors = [],
  writes = [];
let member = false,
  failGroups = false,
  failSubmit = false;
try {
  const { config } = await loadConfigFromFile(
    { command: 'serve', mode: 'development' },
    path.join(root, 'vite.config.js'),
  );
  server = await createServer({
    ...config,
    configFile: false,
    root,
    server: { host: '127.0.0.1', port: 0, open: false, proxy: {}, hmr: false },
    plugins: config.plugins
      .flat(Infinity)
      .filter((p) => p?.name !== '@code-inspector/vite'),
  });
  fs.writeFileSync(
    files[0],
    `<html><meta name="viewport" content="width=device-width,initial-scale=1"><div id="root"></div><script type="module" src="/${name}.jsx"></script></html>`,
  );
  fs.writeFileSync(
    files[1],
    `import React from 'react';import {createRoot} from 'react-dom/client';import {MemoryRouter} from 'react-router-dom';import Enterprise from './src/pages/Enterprise';import EnterpriseAdmin from './src/pages/EnterpriseAdmin';import {StatusContext} from './src/context/Status';import {UserContext} from './src/context/User';import {initialLocaleReady} from './src/i18n/i18n';import '@douyinfe/semi-ui/dist/css/semi.css';import './src/index.css';await initialLocaleReady;const admin=location.search.includes('admin');createRoot(document.getElementById('root')).render(<MemoryRouter><StatusContext.Provider value={[{status:{enterprise_enabled:true}},()=>{}]}><UserContext.Provider value={[{user:{id:1,role:admin?100:1}},()=>{}]}>{admin?<EnterpriseAdmin/>:<Enterprise/>}</UserContext.Provider></StatusContext.Provider></MemoryRouter>);`,
  );
  await server.listen();
  const base = `http://127.0.0.1:${server.httpServer.address().port}`;
  browser = await puppeteer.launch({
    headless: true,
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  });
  const page = await browser.newPage();
  page.on('pageerror', (e) => errors.push(e.message));
  await page.setViewport({ width: 1440, height: 960 });
  await page.evaluateOnNewDocument(() => {
    localStorage.setItem('site-language', 'zh-CN');
    localStorage.setItem(
      'user',
      JSON.stringify({ id: 1, role: 100, username: 'fixture' }),
    );
  });
  await page.setRequestInterception(true);
  page.on('request', async (req) => {
    const url = new URL(req.url());
    if (url.origin !== base) {
      await req.abort();
      return;
    }
    if (!url.pathname.startsWith('/api/')) {
      await req.continue();
      return;
    }
    let response = { success: true, data: null };
    const p = url.pathname;
    if (p === '/api/status') response.data = { enterprise_enabled: true };
    else if (p === '/api/group/')
      response = failGroups
        ? { success: false, message: '分组读取失败' }
        : { success: true, data: [{ name: 'default', ratio: 1 }] };
    else if (p === '/api/enterprise/self')
      response.data = member
        ? {
            member: { role: 'owner' },
            enterprise: { name: '验收企业', status: 'active' },
            manager: true,
          }
        : null;
    else if (p.endsWith('/applications') && req.method() === 'GET') {
      const filtered = apps
        .filter(
          (a) =>
            (!url.searchParams.get('kind') ||
              a.kind === url.searchParams.get('kind')) &&
            (!url.searchParams.get('status') ||
              a.status === url.searchParams.get('status')),
        )
        .reverse();
      response.data = {
        items: filtered.slice(
          0,
          Number(url.searchParams.get('page_size') || 20),
        ),
        total: filtered.length,
      };
    } else if (
      p === '/api/enterprise/applications' &&
      req.method() === 'POST'
    ) {
      const body = JSON.parse(req.postData());
      writes.push(body);
      if (failSubmit)
        response = { success: false, message: '提交失败，请重试' };
      else {
        const app = {
          ...body,
          id: apps.length + 1,
          user_id: 1,
          status: 'pending',
          created_at: 1790000000,
        };
        apps.push(app);
        response.data = app;
      }
    } else if (p.endsWith('/review')) {
      const body = JSON.parse(req.postData());
      writes.push(body);
      const app = apps.find((a) => a.id === Number(p.split('/').at(-2)));
      app.status =
        body.action === 'approve'
          ? 'approved'
          : body.action === 'contact'
            ? 'contacted'
            : 'rejected';
      app.reason = body.reason;
      app.reviewer_id = 1;
      app.reviewed_at = 1790000100;
      if (body.action === 'approve') member = true;
      response.data = app;
    } else if (p.endsWith('/enterprises') || p.endsWith('/reservations'))
      response.data = { items: [], total: 0 };
    else {
      errors.push(`Unexpected ${req.method()} ${p}`);
      response = { success: false, message: 'unexpected' };
    }
    await req.respond({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(response),
    });
  });
  const click = async (text) => {
    await page.waitForFunction(
      (text) =>
        [...document.querySelectorAll('button,[role=tab],[role=option]')].some(
          (e) => e.textContent.trim() === text,
        ),
      {},
      text,
    );
    const h = await page.evaluateHandle(
      (text) =>
        [...document.querySelectorAll('button,[role=tab],[role=option]')].find(
          (e) => e.textContent.trim() === text,
        ),
      text,
    );
    await h.asElement().click();
    await h.dispose();
  };
  const text = async (value) =>
    page.waitForFunction(
      (value) => document.body.innerText.includes(value),
      {},
      value,
    );
  const visit = async (admin) => {
    await page.goto(`${base}/${name}.html${admin ? '?admin' : ''}`, {
      waitUntil: 'networkidle0',
    });
  };
  await visit(false);
  await click('联系开通');
  await page.waitForSelector('input[aria-label="企业名称"]');
  assert.equal(
    await page.$eval('.semi-modal-footer button:last-child', (e) => e.disabled),
    true,
  );
  await page.type('input[aria-label="企业名称"]', '验收企业');
  await page.type('input[aria-label="联系方式"]', 'wechat-account');
  failSubmit = true;
  await click('提交申请');
  await text('提交失败，请重试');
  assert.equal(apps.length, 0);
  failSubmit = false;
  await click('提交申请');
  await text('申请已提交，请等待管理员处理。');
  assert.equal(apps.length, 1);
  assert.equal(apps[0].name, '验收企业');
  await page.screenshot({
    path: path.join(artifacts, 'opening-pending.png'),
    fullPage: true,
  });
  await visit(true);
  await click('申请审核');
  await text('wechat-account');
  failGroups = true;
  await click('审核并开通');
  await text('重试');
  assert.equal(
    await page.$eval('.semi-modal-footer button:last-child', (e) => e.disabled),
    true,
  );
  failGroups = false;
  await click('重试');
  await page.waitForSelector(
    '.semi-modal .semi-select:not(.semi-select-disabled)',
  );
  await page.click('.semi-modal .semi-select');
  await click('default · 1×');
  await page.waitForFunction(
    () =>
      !document.querySelector('.semi-modal-footer button:last-child')?.disabled,
  );
  await click('确定');
  await page.waitForFunction(() => !document.querySelector('.semi-modal'));
  assert.equal(apps[0].status, 'approved');
  assert.equal(writes.at(-1).billing_group, 'default');
  await visit(false);
  await text('进入工作台');
  await click('填写联系方式');
  await page.waitForSelector('input[aria-label="联系方式"]');
  assert.equal(await page.$('input[aria-label="企业名称"]'), null);
  await page.click('.semi-modal .semi-select');
  await click('Telegram');
  await page.waitForFunction(
    () =>
      document.querySelector('.semi-modal .semi-select-selection-text')
        ?.textContent === 'Telegram',
  );
  await page.type('input[aria-label="联系方式"]', '@custom-contact');
  await click('提交申请');
  await text('申请已提交，请等待管理员处理。');
  assert.equal(apps[1].contact_type, 'telegram');
  await visit(true);
  await click('申请审核');
  await text('@custom-contact');
  await click('标记已联系');
  await page.waitForFunction(
    () =>
      !document.querySelector('.semi-modal-footer button:last-child')?.disabled,
  );
  await click('确定');
  await page.waitForFunction(() => !document.querySelector('.semi-modal'));
  assert.equal(apps[1].status, 'contacted');
  await visit(false);
  await page.setViewport({ width: 390, height: 844 });
  await click('填写联系方式');
  await text('已联系');
  await page.waitForFunction(() => {
    const r = document.querySelector('.semi-modal')?.getBoundingClientRect();
    return r && r.left >= 0 && r.right <= innerWidth;
  });
  await new Promise((r) => setTimeout(r, 350));
  assert.equal(
    await page.$eval(
      '.semi-modal-footer',
      (el) => el.getBoundingClientRect().bottom <= innerHeight,
    ),
    true,
  );
  await page.screenshot({ path: path.join(artifacts, 'custom-mobile.png') });
  assert.equal(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
    true,
  );
  assert.deepEqual(errors, []);
  console.log(`Enterprise application UI passed; screenshots: ${artifacts}`);
} catch (e) {
  const page = (await browser.pages()).at(-1);
  console.error(await page.evaluate(() => document.body.innerText));
  console.error(writes);
  await page.screenshot({
    path: path.join(artifacts, 'failure.png'),
    fullPage: true,
  });
  throw e;
} finally {
  await browser?.close();
  await server?.close();
  stop();
  for (const f of files) fs.rmSync(f, { force: true });
}
