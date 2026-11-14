import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';
import { createServer, loadConfigFromFile } from 'vite';
import { stop as stopEsbuild } from 'esbuild';
const root = fileURLToPath(new URL('..', import.meta.url));
const name = `.enterprise-group-qa-${process.pid}`;
const files = ['html', 'jsx'].map((ext) => path.join(root, `${name}.${ext}`));
const artifacts = fs.mkdtempSync(
  path.join(os.tmpdir(), 'enterprise-group-browser-'),
);
let server, browser;
const errors = [],
  writes = [],
  external = [];
let enterprise = {
  id: 11,
  name: '验收企业',
  owner_name: '验收账号',
  billing_group: 'default',
  status: 'active',
  unallocated: 100,
  net_credited: 100,
  created_at: 1,
};
let failSave = false;
let failGroups = false;
let preferredRatio = 0.8;
let groupReads = 0;
let blockNextGroups = false;
let releaseBlockedGroups;
let blockedGroupsStarted;
let groupWasBlocked;
const blockGroupRequest = () => {
  blockNextGroups = true;
  blockedGroupsStarted = new Promise((resolve) => {
    groupWasBlocked = resolve;
  });
};
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
    `<!doctype html><html><meta name="viewport" content="width=device-width, initial-scale=1"><div id="root"></div><script type="module" src="/${name}.jsx"></script></html>`,
  );
  fs.writeFileSync(
    files[1],
    `import React from 'react';import {createRoot} from 'react-dom/client';import {MemoryRouter} from 'react-router-dom';import EnterpriseAdmin from './src/pages/EnterpriseAdmin';import {StatusContext} from './src/context/Status';import {getTokensColumns} from './src/components/table/tokens/TokensColumnDefs';import {initialLocaleReady} from './src/i18n/i18n';import '@douyinfe/semi-ui/dist/css/semi.css';import './src/index.css';await initialLocaleReady;const cell=getTokensColumns({t:s=>s}).find(c=>c.key==='group_ratio').render;createRoot(document.getElementById('root')).render(<MemoryRouter><StatusContext.Provider value={[{},()=>{}]}><EnterpriseAdmin/><div data-qa-enterprise>{cell(null,{payer:'enterprise',group_ratio:0.8,group_ratio_status:'fixed',routing_config:'{"groups":["default"]}',auto_group_fallback:true})}</div><div data-qa-personal>{cell(null,{payer:'personal',group_ratio:0.8,group_ratio_status:'auto',routing_config:'{"groups":["default"]}'})}</div><div data-qa-unavailable>{cell(null,{payer:'enterprise',group_ratio:null,group_ratio_status:'unavailable',routing_config:'{"groups":["default"]}'})}</div></StatusContext.Provider></MemoryRouter>);`,
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
    localStorage.setItem('quota_per_unit', '500000');
    localStorage.setItem(
      'user',
      JSON.stringify({ id: 1, role: 100, username: 'fixture' }),
    );
    localStorage.setItem(
      'status',
      JSON.stringify({ enterprise_enabled: true }),
    );
  });
  await page.setRequestInterception(true);
  page.on('request', async (req) => {
    const url = new URL(req.url());
    if (url.origin !== base) {
      external.push(req.url());
      await req.abort();
      return;
    }
    if (!url.pathname.startsWith('/api/')) {
      await req.continue();
      return;
    }
    let response = { success: true, data: null };
    if (url.pathname === '/api/status')
      response.data = { enterprise_enabled: true };
    else if (url.pathname === '/api/group/') {
      groupReads++;
      response = failGroups
        ? { success: false, message: '分组读取失败' }
        : {
            success: true,
            data: [
              { name: 'default', ratio: 1 },
              { name: '优选', ratio: preferredRatio },
              { name: '免费', ratio: 0 },
            ],
          };
      if (blockNextGroups) {
        blockNextGroups = false;
        groupWasBlocked();
        await new Promise((resolve) => {
          releaseBlockedGroups = resolve;
        });
      }
    } else if (
      url.pathname === '/api/enterprise/admin/enterprises' &&
      req.method() === 'GET'
    )
      response.data = { items: [enterprise], total: 1 };
    else if (url.pathname === '/api/enterprise/admin/reservations')
      response.data = { items: [], total: 0 };
    else if (
      url.pathname === '/api/enterprise/admin/enterprises/11' &&
      req.method() === 'PUT'
    ) {
      const body = JSON.parse(req.postData());
      writes.push(body);
      if (failSave) response = { success: false, message: '保存失败，请重试' };
      else enterprise = { ...enterprise, ...body };
    } else {
      errors.push(`Unexpected synthetic API: ${req.method()} ${url.pathname}`);
      response = { success: false, message: 'unexpected' };
    }
    await req.respond({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(response),
    });
  });
  const clickText = async (text) => {
    const button = await page.evaluateHandle(
      (text) =>
        [...document.querySelectorAll('button')].find(
          (b) => b.textContent.trim() === text,
        ),
      text,
    );
    assert.ok(button.asElement(), `button ${text}`);
    await button.asElement().click();
    await button.dispose();
  };
  const open = async () => {
    await clickText('编辑');
    await page.waitForSelector('.ec-modal .semi-select');
  };
  const select = async (label) => {
    await page.waitForFunction(() => {
      const select = document.querySelector('.ec-modal .semi-select');
      return (
        select &&
        !select.classList.contains('semi-select-disabled') &&
        select.getAttribute('aria-disabled') !== 'true'
      );
    });
    await page.click('.ec-modal .semi-select');
    await page.waitForFunction(
      (label) =>
        [...document.querySelectorAll('.semi-select-option')].some(
          (e) => e.textContent.trim() === label,
        ),
      {},
      label,
    );
    const option = await page.evaluateHandle(
      (label) =>
        [...document.querySelectorAll('.semi-select-option')].find(
          (e) => e.textContent.trim() === label,
        ),
      label,
    );
    await option.asElement().click();
    await option.dispose();
    await page.waitForFunction(
      (label) =>
        document
          .querySelector('.ec-modal .semi-select-selection')
          ?.textContent.includes(label),
      {},
      label,
    );
  };
  await page.goto(`${base}/${name}.html`, { waitUntil: 'networkidle0' });
  await page.waitForFunction(() =>
    document.body.innerText.includes('验收企业'),
  );
  assert.equal(
    await page.$eval('[data-qa-enterprise]', (el) => el.textContent),
    '0.8x',
  );
  assert.equal(
    await page.$eval('[data-qa-personal]', (el) => el.textContent),
    '自动',
  );
  assert.equal(
    await page.$eval('[data-qa-unavailable]', (el) => el.textContent),
    '—',
  );
  await open();
  await select('优选 · 0.8×');

  await page.screenshot({
    path: path.join(artifacts, 'desktop.png'),
    fullPage: true,
  });
  await clickText('保存');
  await page.waitForFunction(() => !document.querySelector('.ec-modal'));
  assert.equal(writes.at(-1).billing_group, '优选');
  await page.waitForFunction(() =>
    document.querySelector('.ea')?.innerText.includes('优选 · 0.8×'),
  );
  await page.setViewport({ width: 390, height: 844 });
  await open();
  await select('免费 · 0×');
  await page.screenshot({
    path: path.join(artifacts, 'mobile.png'),
    fullPage: true,
  });
  await clickText('保存');
  await page.waitForFunction(() => !document.querySelector('.ec-modal'));
  assert.equal(writes.at(-1).billing_group, '免费');
  await open();
  await select('优选 · 0.8×');
  failSave = true;
  await clickText('保存');
  await page.waitForFunction(() =>
    document.body.innerText.includes('保存失败，请重试'),
  );
  assert.ok(await page.$('.ec-modal'));
  assert.ok(
    await page.$eval('.ec-modal', (el) => el.innerText.includes('优选 · 0.8×')),
  );
  assert.equal(enterprise.billing_group, '免费');
  await page.click('.ec-modal .semi-modal-close');
  await page.waitForFunction(() => !document.querySelector('.ec-modal'));
  failSave = false;
  preferredRatio = 0.7;
  const beforeOpen = groupReads;
  await open();
  await select('优选 · 0.7×');
  assert.ok(
    groupReads > beforeOpen,
    'opening editor must fetch current group prices',
  );
  await clickText('保存');
  await page.waitForFunction(() => !document.querySelector('.ec-modal'));
  await page.waitForFunction(() =>
    document.querySelector('.ea')?.innerText.includes('优选 · 0.7×'),
  );

  // Loading errors must not leave stale options saveable; retry preserves the draft.
  failGroups = true;
  await open();
  await page.waitForFunction(() =>
    [...document.querySelectorAll('.ec-modal button')].some(
      (b) => b.textContent.trim() === '重试',
    ),
  );
  assert.equal(
    await page.$$eval(
      '.ec-modal button',
      (buttons) =>
        buttons.find((b) => b.textContent.trim() === '保存')?.disabled,
    ),
    true,
  );
  // Let preceding synthetic success/error toasts clear before typing on mobile.
  await page.waitForFunction(() => !document.querySelector('.semi-toast-content'));
  const nameInput = await page.$('.ec-modal input');
  await nameInput.click({ clickCount: 3 });
  await nameInput.type('保留中的名称');
  failGroups = false;
  preferredRatio = 0.6;
  // Rapid synthetic operations can stack transient toasts above the retry button.
  await page.waitForFunction(
    () => !document.querySelector('.semi-toast-content'),
  );
  await clickText('重试');
  await select('优选 · 0.6×');
  assert.equal(
    await page.$eval('.ec-modal input', (el) => el.value),
    '保留中的名称',
  );

  // A late focus refresh may not replace a newer response or re-enable Save early.
  preferredRatio = 0.5;
  blockGroupRequest();
  await page.evaluate(() => window.dispatchEvent(new Event('focus')));
  await blockedGroupsStarted;
  assert.equal(
    await page.$$eval(
      '.ec-modal button',
      (buttons) =>
        buttons.find((b) => b.textContent.trim() === '保存')?.disabled,
    ),
    true,
  );
  preferredRatio = 0.4;
  await page.evaluate(() => window.dispatchEvent(new Event('focus')));
  await page.waitForFunction(() =>
    document
      .querySelector('.ec-modal .semi-select-selection')
      ?.textContent.includes('0.4×'),
  );
  releaseBlockedGroups();
  await page.waitForNetworkIdle({ idleTime: 200 });
  assert.ok(
    await page.$eval('.ec-modal .semi-select-selection', (el) =>
      el.textContent.includes('0.4×'),
    ),
  );
  await clickText('保存');
  await page.waitForFunction(() => !document.querySelector('.ec-modal'));
  assert.equal(writes.at(-1).name, '保留中的名称');
  assert.deepEqual(errors, []);
  assert.deepEqual(external, []);
  console.log(
    JSON.stringify({
      checks: [
        'select existing group',
        'save binding',
        'free group',
        'failure retains draft',
        'enterprise multi-route ratio',
        'personal auto ratio',
        'unknown price',
        'open and post-save price refresh',
        'failed refresh disables save and retries',
        'late responses cannot overwrite fresh prices',
      ],
      writes: writes.length,
      artifacts,
    }),
  );
} finally {
  releaseBlockedGroups?.();
  await browser?.close();
  await server?.close();
  stopEsbuild();
  for (const file of files) fs.rmSync(file, { force: true });
}
