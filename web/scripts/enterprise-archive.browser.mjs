import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';
import { createServer, loadConfigFromFile } from 'vite';
import { stop as stopEsbuild } from 'esbuild';
const root = fileURLToPath(new URL('..', import.meta.url));
const name = `.enterprise-archive-qa-${process.pid}`;
const files = ['html', 'jsx'].map((ext) => path.join(root, `${name}.${ext}`));
const artifacts = fs.mkdtempSync(
  path.join(os.tmpdir(), 'enterprise-archive-browser-'),
);
let server, browser;
const errors = [],
  writes = [],
  external = [],
  apiReads = [];
let enterprise = {
  id: 11,
  name: '归档验收企业',
  status: 'frozen',
  owner_name: '本地验收',
  owner_user_id: 1,
  billing_group: 'default',
  timezone: 'Asia/Shanghai',
  unallocated: -299,
  net_credited: 0,
  created_at: 1,
};
let failPreview = false,
  failList = false,
  failArchive = false,
  pendingTasks = 0,
  previewReads = 0;
let delayedList = false,
  releaseList,
  listStarted;
let holdSubmit = false,
  submitSeen,
  releaseSubmit;
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
    `import React from 'react';import {createRoot} from 'react-dom/client';import {MemoryRouter} from 'react-router-dom';import EnterpriseAdmin from './src/pages/EnterpriseAdmin';import {StatusContext} from './src/context/Status';import {initialLocaleReady} from './src/i18n/i18n';import '@douyinfe/semi-ui/dist/css/semi.css';import './src/index.css';await initialLocaleReady;createRoot(document.getElementById('root')).render(<MemoryRouter><StatusContext.Provider value={[{},()=>{}]}><EnterpriseAdmin/></StatusContext.Provider></MemoryRouter>);`,
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
  page.setDefaultTimeout(12000);
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
    apiReads.push(`${req.method()} ${url.pathname}${url.search}`);
    let response = { success: true, data: null };
    if (url.pathname === '/api/status')
      response.data = { enterprise_enabled: true };
    else if (url.pathname === '/api/group/')
      response.data = [{ name: 'default', ratio: 1 }];
    else if (url.pathname === '/api/enterprise/admin/enterprises') {
      const archived = url.searchParams.get('status') === 'archived';
      const keyword = url.searchParams.get('keyword') || '';
      const match =
        (enterprise.status === 'archived') === archived &&
        enterprise.name.includes(keyword);
      response = failList
        ? { success: false, message: '列表读取失败' }
        : {
            success: true,
            data: {
              items: match ? [{ ...enterprise }] : [],
              total: match ? 1 : 0,
            },
          };
      if (delayedList) {
        delayedList = false;
        listStarted();
        await new Promise((resolve) => {
          releaseList = resolve;
        });
      }
    } else if (url.pathname.endsWith('/archive-preview')) {
      previewReads++;
      const blockers = [
        ...(enterprise.unallocated ? ['remaining_balance'] : []),
        ...(pendingTasks ? ['pending_tasks'] : []),
      ];
      response = failPreview
        ? { success: false, message: '预检读取失败' }
        : {
            success: true,
            data: {
              enterprise: { ...enterprise },
              remaining: enterprise.unallocated,
              members: 1,
              tokens: 2,
              pending_invitations: 0,
              pending_requests: 0,
              open_reservations: 0,
              pending_tasks: pendingTasks,
              pending_midjourney: 0,
              can_archive: !blockers.length,
              blockers,
            },
          };
    } else if (req.method() === 'POST' && url.pathname.endsWith('/adjust')) {
      const body = JSON.parse(req.postData());
      writes.push({ action: 'adjust', body });
      enterprise.unallocated += body.delta;
      enterprise.net_credited += body.delta;
    } else if (req.method() === 'POST' && url.pathname.endsWith('/archive')) {
      const body = JSON.parse(req.postData());
      writes.push({ action: 'archive', body });
      if (holdSubmit) {
        submitSeen();
        await new Promise((resolve) => {
          releaseSubmit = resolve;
        });
      }
      if (failArchive)
        response = {
          success: false,
          message: '企业尚未满足归档条件，请刷新预检',
        };
      else {
        assert.equal(body.name, enterprise.name);
        assert.ok(body.reason);
        enterprise = {
          ...enterprise,
          status: 'archived',
          archived_at: 1794844800,
          archive_reason: body.reason,
        };
        response.data = enterprise;
      }
    } else if (url.pathname.endsWith('/usage'))
      response.data = {
        items: [
          {
            id: 1,
            request_id: 'synthetic-request',
            member_name: '本地验收',
            model_name: 'synthetic-model',
            quota: 299,
            status: 'settled',
            created_at: 1,
          },
        ],
        total: 1,
      };
    else if (url.pathname.endsWith('/ledger'))
      response.data = {
        items: [
          {
            id: 1,
            kind: 'adjust',
            member_name: '本地验收',
            unallocated_delta: 299,
            member_delta: 0,
            reason: '测试消费核销',
            created_at: 1,
          },
        ],
        total: 1,
      };
    else if (url.pathname.endsWith('/audit'))
      response.data = {
        items: [
          {
            id: 1,
            action: 'enterprise.archive',
            actor_name: '平台',
            reason: enterprise.archive_reason,
            created_at: 1,
          },
        ],
        total: 1,
      };
    else if (url.pathname.endsWith('/members'))
      response.data = {
        items: [
          {
            id: 1,
            user_id: 1,
            display_name: '本地验收',
            role: 'owner',
            consumed: 299,
            joined_at: 1,
          },
        ],
        total: 1,
      };
    else if (url.pathname.endsWith('/reservations'))
      response.data = { items: [], total: 0 };
    else
      throw new Error(`Unexpected local API ${req.method()} ${url.pathname}`);
    await req.respond({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(response),
    });
  });
  const clickText = async (text, selector = 'button') => {
    await page.waitForFunction(
      (text, selector) =>
        [...document.querySelectorAll(selector)].some(
          (e) => e.getClientRects().length && e.textContent.trim() === text,
        ),
      {},
      text,
      selector,
    );
    const el = await page.evaluateHandle(
      (text, selector) =>
        [...document.querySelectorAll(selector)].find(
          (e) => e.getClientRects().length && e.textContent.trim() === text,
        ),
      text,
      selector,
    );
    await el.asElement().click();
    await el.dispose();
  };
  const openArchive = async () => {
    await clickText('更多');
    await clickText('归档企业', '.semi-dropdown-item');
    await page.waitForSelector('.ea-archive-modal');
  };
  const waitModalExit = async () => {
    await page.waitForFunction(() => !document.querySelector('.ec-modal'));
    await page.waitForFunction(() => document.body.style.overflow !== 'hidden');
    await page.waitForFunction(() => {
      const rect = document
        .querySelector('#semiTabarchived')
        ?.getBoundingClientRect();
      if (!rect) return false;
      const hit = document.elementFromPoint(
        rect.x + rect.width / 2,
        rect.y + rect.height / 2,
      );
      return (
        hit?.closest('.semi-modal-wrap') == null &&
        hit?.closest('.semi-modal-mask') == null
      );
    });
  };
  const clearToasts = async () => {
    // Rapid synthetic actions stack toasts over the small viewport's tabs.
    // Wait for their normal dismissal instead of clicking an animating node.
    await page.waitForFunction(() => !document.querySelector('.semi-toast'));
  };
  const closeModal = async () => {
    await page.click('.ec-modal .semi-modal-close');
    await waitModalExit();
  };
  const canSubmit = () =>
    page.$$eval(
      '.ea-archive-modal button',
      (buttons) =>
        !buttons.find((b) => b.textContent.trim() === '归档企业')?.disabled,
    );
  await page.goto(`${base}/${name}.html`, { waitUntil: 'networkidle0' });
  await openArchive();
  await page.waitForFunction(() =>
    document.querySelector('.ea-archive-modal')?.innerText.includes('-299'),
  );
  assert.equal(await canSubmit(), false);
  assert.ok(
    await page.$eval('.ea-archive-modal', (e) =>
      e.innerText.includes('净余额须结清'),
    ),
  );
  await page.screenshot({ path: path.join(artifacts, 'blocked-desktop.png') });
  await closeModal();
  await clickText('更多');
  await clickText('调账', '.semi-dropdown-item');
  await clickText('原始额度', '.ec-modal .semi-radio');
  await page.type('.ec-modal input[aria-label="原始额度"]', '299');
  const fields = await page.$$('.ec-modal input');
  await fields.at(-1).type('测试消费核销');
  await clickText('调账', '.ec-modal button');
  await page.waitForFunction(() => !document.querySelector('.ec-modal'));
  assert.deepEqual(writes.at(-1), {
    action: 'adjust',
    body: { delta: 299, reason: '测试消费核销' },
  });
  failPreview = true;
  await openArchive();
  await page.waitForFunction(() =>
    document
      .querySelector('.ea-archive-modal')
      ?.innerText.includes('预检读取失败'),
  );
  assert.equal(await canSubmit(), false);
  failPreview = false;
  pendingTasks = 1;
  await clickText('重试', '.ec-modal button');
  await page.waitForFunction(() =>
    document
      .querySelector('.ea-archive-modal')
      ?.innerText.includes('视频或音乐任务'),
  );
  assert.equal(await canSubmit(), false);
  pendingTasks = 0;
  await clickText('重新检查', '.ec-modal button');
  await page.waitForSelector('#archive-name');
  await page.type('#archive-name', '错误企业');
  await page.type('#archive-reason', '本地验收结束');
  assert.equal(await canSubmit(), false);
  await page.click('#archive-name', { clickCount: 3 });
  await page.type('#archive-name', enterprise.name);
  assert.equal(await canSubmit(), true);
  failArchive = true;
  const before = previewReads;
  await clickText('归档企业', '.ec-modal button');
  await page.waitForFunction(
    () => document.querySelector('#archive-reason')?.value === '本地验收结束',
  );
  for (let i = 0; previewReads <= before && i < 100; i++)
    await new Promise((r) => setTimeout(r, 25));
  assert.ok(previewReads > before);
  await clearToasts();
  await page.waitForSelector('#archive-name');
  failArchive = false;
  await page.setViewport({ width: 390, height: 540 });
  const bounds = await page.$eval('.ea-archive-modal .semi-modal', (e) => {
    const r = e.getBoundingClientRect();
    return { left: r.left, right: r.right, top: r.top, bottom: r.bottom };
  });
  assert.ok(
    bounds.left >= 0 &&
      bounds.right <= 391 &&
      bounds.top >= 0 &&
      bounds.bottom <= 541,
    JSON.stringify(bounds),
  );
  await page.screenshot({
    path: path.join(artifacts, 'confirm-mobile-short.png'),
  });
  const submitted = new Promise((resolve) => {
    submitSeen = resolve;
  });
  holdSubmit = true;
  await clickText('归档企业', '.ec-modal button');
  await submitted;
  const n = writes.length;
  await page.keyboard.press('Escape');
  assert.ok(await page.$('.ea-archive-modal'));
  assert.equal(writes.length, n);
  holdSubmit = false;
  releaseSubmit();
  await waitModalExit();
  await clearToasts();
  await clickText('已归档', '.semi-tabs-tab');
  await page.waitForFunction(() =>
    document
      .querySelector('.semi-tabs-tab-active')
      ?.innerText.includes('已归档'),
  );
  await page.waitForFunction(() =>
    document.querySelector('.ea')?.innerText.includes('归档验收企业'),
  );
  await clickText('历史记录');
  await page.waitForFunction(() =>
    document
      .querySelector('.ea-history-modal')
      ?.innerText.includes('synthetic-model'),
  );
  assert.ok(
    await page.$eval('.ea-history-modal', (e) =>
      e.innerText.includes('本地验收结束'),
    ),
  );
  await clickText('资金流水', '.ea-history-modal [role="tab"]');
  await page.waitForFunction(() =>
    document
      .querySelector('.ea-history-modal')
      ?.innerText.includes('测试消费核销'),
  );
  await clickText('操作记录', '.ea-history-modal [role="tab"]');
  await page.waitForFunction(() =>
    document.querySelector('.ea-history-modal')?.innerText.includes('归档企业'),
  );
  await page.screenshot({ path: path.join(artifacts, 'history-mobile.png') });
  await closeModal();
  failList = true;
  await clickText('企业', '.semi-tabs-tab');
  await page.waitForFunction(() =>
    document.querySelector('.ea')?.innerText.includes('列表读取失败'),
  );
  failList = false;
  await clickText('重试', '.ea button');
  await page.waitForFunction(() => !document.querySelector('.ea-load-error'));
  const delayed = new Promise((resolve) => {
    listStarted = resolve;
  });
  delayedList = true;
  await clickText('已归档', '.semi-tabs-tab');
  await delayed;
  await clickText('企业', '.semi-tabs-tab');
  releaseList();
  await page.waitForFunction(() =>
    document.querySelector('.ea')?.innerText.includes('还没有企业'),
  );
  assert.equal(
    await page.$$eval('.ea .semi-table-tbody tr', (rows) =>
      rows.some((r) => r.innerText.includes('归档验收企业')),
    ),
    false,
  );
  await page.setViewport({ width: 1440, height: 960 });
  await clickText('已归档', '.semi-tabs-tab');
  await page.waitForFunction(() =>
    document.querySelector('.ea')?.innerText.includes('归档验收企业'),
  );
  await page.screenshot({ path: path.join(artifacts, 'archived-desktop.png') });
  assert.deepEqual(errors, []);
  assert.deepEqual(external, []);
  assert.equal(writes.filter((w) => w.action === 'archive').length, 2);
  console.log(
    JSON.stringify({
      passed: true,
      artifacts,
      checks: [
        'negative microbalance',
        'exact adjustment',
        'preview retry',
        'async blocker',
        'exact name',
        'commit recheck',
        'short viewport',
        'submit guard',
        'history',
        'list retry',
        'stale responses',
      ],
      externalRequests: external.length,
    }),
  );
} catch (error) {
  if (browser) {
    const pages = await browser.pages();
    const last = pages.at(-1);
    await last.screenshot({ path: path.join(artifacts, 'failure.png') });
    console.error(
      JSON.stringify({
        artifacts,
        enterprise,
        writes,
        apiReads,
        screen: await last.evaluate(() => document.body.innerText),
      }),
    );
  }
  throw error;
} finally {
  releaseList?.();
  releaseSubmit?.();
  if (browser) await browser.close();
  if (server) await server.close();
  stopEsbuild();
  for (const file of files) if (fs.existsSync(file)) fs.unlinkSync(file);
}
