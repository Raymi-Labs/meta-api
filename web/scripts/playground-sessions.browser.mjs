import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import puppeteer from 'puppeteer-core';

const dist = path.resolve(
  process.argv[2] || '/tmp/meta-api-conversations-build',
);
const calls = [],
  held = [],
  errors = [];
let hold = false;
let offeredTiers = ['default', 'priority', 'ultrafast'];
let failModel = null;
const sockets = new Set();
const server = createServer(async (req, res) => {
  if (req.url === '/pg/chat/completions') {
    let body = '';
    for await (const part of req) body += part;
    const payload = JSON.parse(body);
    calls.push({ payload, session: req.headers['x-session-id'] });
    if (payload.model === failModel) {
      res
        .writeHead(502, { 'Content-Type': 'application/json' })
        .end(JSON.stringify({ error: { message: '模拟上游失败' } }));
      return;
    }
    const text = 'answer-' + payload.model;
    if (payload.stream === false) {
      res.setHeader('Content-Type', 'application/json');
      const done = () =>
        res.end(
          JSON.stringify({
            choices: [
              {
                message: { role: 'assistant', content: text },
                finish_reason: 'stop',
              },
            ],
          }),
        );
      if (hold) held.push(done);
      else done();
      return;
    }
    res.setHeader('Content-Type', 'text/event-stream');
    res.flushHeaders();
    res.write(
      `data: ${JSON.stringify({ choices: [{ delta: { content: text } }] })}\n\n`,
    );
    const done = () => {
      if (!res.destroyed)
        res.end(
          `data: ${JSON.stringify({ choices: [{ delta: {}, finish_reason: 'stop' }] })}\n\ndata: [DONE]\n\n`,
        );
    };
    if (hold) held.push(done);
    else done();
    return;
  }
  const pathname = new URL(req.url, 'http://localhost').pathname;
  const file = pathname.startsWith('/assets/')
    ? path.join(dist, pathname)
    : path.join(dist, 'index.html');
  try {
    res.setHeader(
      'Content-Type',
      {
        '.js': 'application/javascript',
        '.css': 'text/css',
        '.html': 'text/html',
        '.svg': 'image/svg+xml',
      }[path.extname(file)] || 'application/octet-stream',
    );
    res.end(await readFile(file));
  } catch {
    res.writeHead(404).end();
  }
});
server.on('connection', (socket) => {
  sockets.add(socket);
  socket.on('close', () => sockets.delete(socket));
});
await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
const base = `http://127.0.0.1:${server.address().port}`;
let browser;
const active = '.pg-conversation-content>div:not([hidden])';
try {
  browser = await puppeteer.launch({
    headless: true,
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  });
  const setup = async () => {
    const page = await browser.newPage();
    page.setDefaultTimeout(15000);
    page.on('pageerror', (e) => errors.push(e.message));
    page.on('dialog', (d) =>
      d.accept(d.type() === 'prompt' ? 'Renamed conversation' : undefined),
    );
    await page.setViewport({ width: 1440, height: 1000 });
    await page.evaluateOnNewDocument(() => {
      if (!localStorage.getItem('fixture-init')) {
        localStorage.setItem(
          'user',
          JSON.stringify({
            id: 7,
            role: 1,
            username: 'fixture',
            group: 'default',
          }),
        );
        localStorage.setItem(
          'playground_config:user:7',
          JSON.stringify({
            inputs: {
              model: 'model-a',
              modelTabs: ['model-a'],
              group: 'default',
              stream: true,
            },
            parameterEnabled: {},
          }),
        );
        localStorage.setItem('fixture-init', 'yes');
      }
      localStorage.setItem('i18nextLng', 'zh-CN');
      localStorage.setItem('site-language', 'zh-CN');
      localStorage.setItem('newapi:user-onboarding:regular:v1:user:7', 'done');
    });
    await page.setRequestInterception(true);
    page.on('request', async (req) => {
      try {
        const u = new URL(req.url());
        if (u.origin !== base) return req.abort();
        if (!u.pathname.startsWith('/api/')) return req.continue();
        let data = [];
        let body;
        if (u.pathname === '/api/status')
          data = {
            system_name: 'Fixture',
            quota_per_unit: 500000,
            quota_display_type: 'CUSTOM',
            custom_currency_symbol: '⚡',
            custom_currency_exchange_rate: 1,
            price: 0.2,
            server_address: 'https://meta-api.vip',
          };
        if (u.pathname === '/api/user/self')
          data = await page.evaluate(() =>
            JSON.parse(localStorage.getItem('user')),
          );
        if (u.pathname === '/api/user/service-tiers')
          data = {
            model: u.searchParams.get('model'),
            group: u.searchParams.get('group'),
            managed: true,
            tiers: offeredTiers.map((id) => ({
              id,
              multiplier: { default: 1, priority: 2, ultrafast: 6 }[id],
            })),
          };
        if (u.pathname === '/api/user/models')
          data =
            u.searchParams.get('group') === 'limited'
              ? ['model-a']
              : ['model-a', 'model-b', 'model-c', 'model-d', 'model-e'];
        if (u.pathname === '/api/user/self/groups')
          data = {
            default: { ratio: 1, desc: '说明不能当组名' },
            limited: { ratio: 2, desc: '福利' },
            福利: { ratio: 0.6, desc: '福利' },
          };
        if (u.pathname === '/api/pricing')
          body = {
            success: true,
            data: ['model-a', 'model-b', 'model-c', 'model-d', 'model-e'].map(
              (model_name) => ({
                model_name,
                enable_groups:
                  model_name === 'model-a'
                    ? ['default', 'limited', '福利']
                    : ['default', '福利'],
                quota_type: 0,
                supported_endpoint_types: ['openai'],
                output_modalities: ['text'],
                model_ratio: 1,
                completion_ratio: 2,
              }),
            ),
            group_ratio: { default: 1, limited: 2, 福利: 0.6 },
          };
        await req.respond({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(body || { success: true, data }),
        });
      } catch {}
    });
    return page;
  };
  const page = await setup();
  const clickText = async (text, scope = '') =>
    page.evaluate(
      (text, scope) => {
        const b = [
          ...(scope
            ? document.querySelector(scope)
            : document
          ).querySelectorAll('button'),
        ].find(
          (b) => b.innerText === text || b.getAttribute('aria-label') === text,
        );
        if (!b) throw Error('Missing button ' + text);
        b.click();
      },
      text,
      scope,
    );
  const ready = async () => {
    await page.waitForSelector('.pg-conversation-toolbar');
    await page.waitForSelector(active + ' .pg-composer textarea');
  };
  const send = async (text) => {
    await page.type(active + ' .pg-composer textarea', text);
    await page.waitForFunction(
      (selector) => !document.querySelector(selector)?.disabled,
      {},
      active + ' button[aria-label="发送"]',
    );
    await page.click(active + ' button[aria-label="发送"]');
  };
  const saved = async () =>
    page.waitForFunction(() =>
      document
        .querySelector('.pg-conversation-toolbar')
        ?.textContent.includes('已保存在此浏览器'),
    );
  await page.goto(base + '/playground', { waitUntil: 'networkidle0' });
  await ready();
  assert.equal(calls.length, 0);
  // Focused layout utilities are opt-in and must not issue model requests.
  assert.equal(await page.$(active + ' .pg-workspace > .pg-settings'), null);
  assert.equal(await page.$(active + ' .pg-workspace > .pg-code'), null);
  const mainX = await page.$eval(
    active + ' .pg-main',
    (e) => e.getBoundingClientRect().x,
  );
  await page.click('.pg-tool-rail [aria-label="对话设置"]');
  await page.waitForSelector('.pg-docked-settings .pg-settings-scroll', {
    visible: true,
  });
  assert(
    await page.$eval(
      active + ' .pg-main',
      (e, x) => e.getBoundingClientRect().x > x + 200,
      mainX,
    ),
  );
  assert.equal(
    await page.$$eval(
      'input[type=checkbox]',
      (els) => els.filter((e) => e.closest('.pg-stream-toggle')).length,
    ),
    1,
  );
  assert(
    await page.$eval('.pg-docked-settings', (e) =>
      e.textContent.includes('生成参数'),
    ),
  );
  assert(
    await page.$eval('.pg-docked-settings', (e) =>
      e.textContent.includes('随机性'),
    ),
  );
  assert.equal(await page.$('.semi-sidesheet-mask'), null);
  assert.equal(await page.$('.pg-docked-settings .pg-group-section'), null);
  assert.equal(await page.$eval('.pg-parameters', (e) => e.open), false);
  assert.equal(
    await page.$eval('.pg-model-details-fold', (e) => e.open),
    false,
  );
  assert.equal(await page.$('.pg-main .pg-system-prompt'), null);
  await page.click('.pg-docked-settings .pg-system-prompt');
  await page.waitForSelector('textarea[aria-label="系统提示词"]', {
    visible: true,
  });
  await page.type('textarea[aria-label="系统提示词"]', '仅用于设置回归');
  await page.click('.semi-modal-close');
  await page.waitForSelector('.semi-modal', { hidden: true });
  await page.waitForSelector('.pg-prompt-dot');
  await clickText('恢复默认', '.pg-docked-settings');
  await page.waitForSelector('.semi-modal-confirm');
  await clickText('取消', '.semi-modal-confirm');
  await page.waitForFunction(
    () => !document.querySelector('.semi-modal-confirm'),
  );
  assert(await page.$('.pg-prompt-dot'));
  await clickText('恢复默认', '.pg-docked-settings');
  await page.waitForSelector('.semi-modal-confirm');
  await clickText('恢复默认', '.semi-modal-confirm');
  await page.waitForFunction(
    () => !document.querySelector('.semi-modal-confirm'),
  );
  await page.waitForSelector('.pg-prompt-dot', { hidden: true });
  assert(
    await page.$eval('.pg-top-group', (e) => e.textContent.includes('default')),
  );
  assert(
    await page.$eval('.pg-model-tab.is-active', (e) =>
      e.textContent.includes('model-a'),
    ),
  );
  await page.screenshot({ path: '/tmp/playground-refined-settings.png' });
  await page.setViewport({ width: 820, height: 1000 });
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
    'docked settings must fit a tablet viewport',
  );
  await page.screenshot({ path: '/tmp/playground-refined-tablet.png' });
  await page.setViewport({ width: 1440, height: 1000 });
  // Directly switch panels while settings is open; the rail must remain clickable.
  await page.waitForFunction(() => {
    const el = document.querySelector('.pg-tool-rail [aria-label="请求代码"]');
    const r = el.getBoundingClientRect();
    return el.contains(
      document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2),
    );
  });
  await page.click('.pg-tool-rail [aria-label="请求代码"]');
  await page.waitForSelector('.pg-docked-code .pg-code-panel', {
    visible: true,
  });
  assert.equal(await page.$('.pg-docked-settings'), null);
  assert(
    await page.$eval('.pg-docked-code', (e) =>
      e.textContent.includes('meta-api.com'),
    ),
  );
  assert(
    !(await page.$eval('.pg-docked-code', (e) =>
      e.textContent.includes('meta-api.vip'),
    )),
  );
  await page.keyboard.press('Escape');
  await page.waitForFunction(
    () =>
      document
        .querySelector('.pg-tool-rail [aria-label="对话设置"]')
        ?.getAttribute('aria-expanded') === 'false',
  );
  await page.waitForSelector('.pg-docked-settings', { hidden: true });
  await page.click('.pg-tool-rail [aria-label="请求代码"]');
  await page.waitForSelector('.pg-docked-code .pg-code-panel', {
    visible: true,
  });
  await page.keyboard.press('Escape');
  await page.waitForFunction(
    () =>
      document
        .querySelector('.pg-tool-rail [aria-label="请求代码"]')
        ?.getAttribute('aria-expanded') === 'false',
  );
  await page.waitForSelector('.pg-docked-code .pg-code-panel', {
    hidden: true,
  });
  await clickText('对话列表');
  await page.waitForFunction(
    () => document.activeElement?.getAttribute('aria-label') === '搜索对话标题',
  );
  assert.equal(await page.$('.pg-history-backdrop'), null);
  assert(
    await page.$eval(
      active + ' .pg-main',
      (e, x) => e.getBoundingClientRect().x > x + 200,
      mainX,
    ),
  );
  await page.keyboard.press('Escape');
  assert.equal(
    await page.$eval('.pg-conversation-list', (e) => e.hidden),
    true,
  );
  await page.click(active + ' .pg-model-tab.is-active button[role=tab]');
  await page.waitForSelector('.pg-model-options');
  await clickText('model-b', '.pg-model-options');
  await page.waitForSelector('.pg-model-options', { hidden: true });
  const selectGroup = async (value) => {
    await page.click(active + ' .pg-top-group .semi-select');
    await page.waitForSelector('.semi-select-option-list');
    const labels = await page.$$eval(
      '.semi-select-option-list [role="option"]',
      (els) => els.map((e) => e.textContent),
    );
    assert(!labels.some((x) => x.includes('说明不能当组名')));
    await page.evaluate((value) => {
      const e = [
        ...document.querySelectorAll(
          '.semi-select-option-list [role="option"]',
        ),
      ].find((e) => e.textContent.trim() === value);
      if (!e) throw Error('Missing group ' + value);
      e.click();
    }, value);
  };
  await selectGroup('limited');
  await page.waitForFunction(
    (scope) =>
      document
        .querySelector(scope + ' .pg-model-tab.is-active')
        ?.textContent.includes('model-a'),
    {},
    active,
  );
  assert.equal(
    await page.$eval(active + ' .pg-top-ratio', (e) => e.textContent),
    '2×',
  );
  await selectGroup('default');
  await page.waitForSelector('.pg-model-options', { hidden: true });
  await saved();
  assert.equal(calls.length, 0);
  assert(
    await page.$eval(
      '.pg-top-prices',
      (e) => e.textContent.includes('⚡2') && e.textContent.includes('≈¥0.4'),
    ),
  );
  assert(
    !(await page.$eval('.pg-top-prices', (e) => e.textContent.includes('$'))),
  );
  await page.screenshot({ path: '/tmp/playground-focused-desktop-empty.png' });
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  );
  await send('First topic');
  await page.waitForFunction(() =>
    document.body.textContent.includes('answer-model-a'),
  );
  await saved();
  const firstID = calls[0].session;
  assert(firstID);
  assert.equal(calls.length, 1);
  await clickText('新建对话');
  await ready();
  assert.equal(
    await page.$eval(active + ' .pg-messages', (e) =>
      e.textContent.includes('First topic'),
    ),
    false,
  );
  await send('Second topic');
  await saved();
  assert.equal(calls.length, 2);
  assert.notEqual(calls[1].session, firstID);
  assert(!JSON.stringify(calls[1].payload).includes('First topic'));
  await clickText('对话列表');
  await page.click(
    '.pg-conversation-items>div:nth-child(2) .pg-conversation-select',
  );
  await ready();
  assert(
    await page.$eval(active + ' .pg-messages', (e) =>
      e.textContent.includes('First topic'),
    ),
  );
  hold = true;
  await send('Long generation');
  await page.waitForFunction(() =>
    document.querySelector('[aria-label="停止生成"]'),
  );
  await clickText('新建对话');
  await ready();
  assert(
    await page.$$eval('.pg-local-warning', (els) =>
      els.some((e) => e.textContent.includes('另一个对话')),
    ),
  );
  await page.type(active + ' .pg-composer textarea', 'Blocked topic');
  assert(
    await page.$eval(active + ' button[aria-label="发送"]', (e) => e.disabled),
  );
  assert.equal(calls.length, 3);
  hold = false;
  held.splice(0).forEach((done) => done());
  await page.waitForFunction(
    () => !document.querySelector('.pg-local-warning'),
  );
  await send(' now');
  await saved();
  assert.equal(calls.length, 4);
  assert(!JSON.stringify(calls[3].payload).includes('Long generation'));
  // A second tab cannot overwrite or issue requests for this account.
  const second = await setup();
  await second.goto(base + '/playground', { waitUntil: 'networkidle0' });
  await second.waitForFunction(() =>
    document.body.textContent.includes('其他标签页'),
  );
  assert.equal(await second.$('.pg-composer'), null);
  await second.close();
  // Save a genuinely partial stream, reload and prove it is not resumed.
  hold = true;
  await send('Interrupted question');
  await page.waitForFunction(() =>
    document.querySelector('[aria-label="停止生成"]'),
  );
  await new Promise((r) => setTimeout(r, 700));
  const before = calls.length;
  await page.reload({ waitUntil: 'networkidle0' });
  await ready();
  await page.waitForFunction(() =>
    document.body.textContent.includes('上次生成已中断'),
  );
  assert.equal(calls.length, before);
  held.splice(0).forEach((done) => done());
  hold = false;
  // Rename/search, export format and import with fresh IDs.
  await clickText('对话列表');
  await page.click(
    '.pg-conversation-items>div.is-selected button[aria-label="重命名对话"]',
  );
  await saved();
  await page.type('input[aria-label="搜索对话标题"]', 'Renamed');
  assert.equal(
    await page.$$eval('.pg-conversation-items>div', (e) => e.length),
    1,
  );
  await page.$eval('input[aria-label="搜索对话标题"]', (e) => {
    const setter = Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      'value',
    ).set;
    setter.call(e, '');
    e.dispatchEvent(new Event('input', { bubbles: true }));
  });
  const snapshot = await page.evaluate(
    () =>
      new Promise((resolve, reject) => {
        const r = indexedDB.open('meta-api-playground-conversations');
        r.onsuccess = () => {
          const db = r.result;
          const tx = db.transaction('accounts');
          const q = tx.objectStore('accounts').get('7');
          q.onsuccess = () => resolve(q.result);
          tx.oncomplete = () => db.close();
        };
        r.onerror = () => reject(r.error);
      }),
  );
  assert.equal(snapshot.sessions.length, 3);
  assert(
    snapshot.sessions.some((s) => s.messages.some((m) => m.localInterrupted)),
  );
  const backup = JSON.stringify({
    format: 'meta-api-local-conversations',
    version: 1,
    sessions: [snapshot.sessions[0]],
  });
  await page.evaluate((text) => {
    const input = document.querySelector(
      '.pg-conversation-list input[type=file]',
    );
    const dt = new DataTransfer();
    dt.items.add(new File([text], 'backup.json', { type: 'application/json' }));
    input.files = dt.files;
    input.dispatchEvent(new Event('change', { bubbles: true }));
  }, backup);
  await page.waitForFunction(
    () => document.querySelectorAll('.pg-conversation-items>div').length === 4,
  );
  assert.equal(calls.length, before);
  await clickText('对话列表');
  await page.click(
    '.pg-conversation-items>div.is-selected button[aria-label="删除对话"]',
  );
  await page.waitForFunction(
    () => document.querySelectorAll('.pg-conversation-items>div').length === 3,
  );
  await page.setViewport({ width: 390, height: 1000 });
  await page.click('.pg-tool-rail [aria-label="对话设置"]');
  await page.waitForSelector('.pg-sheet .pg-settings-scroll', {
    visible: true,
  });
  await page.keyboard.press('Escape');
  await page.waitForSelector('.pg-sheet .pg-settings-scroll', { hidden: true });
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  );
  await page.screenshot({ path: '/tmp/playground-focused-mobile.png' });
  await clickText('对话列表');
  await page.waitForSelector('.pg-conversation-list.is-open');
  await page.screenshot({
    path: '/tmp/playground-local-conversations-mobile.png',
  });
  // Another account must not see or send this account's saved context.
  await page.evaluate(() =>
    localStorage.setItem(
      'user',
      JSON.stringify({ id: 8, role: 1, username: 'other', group: 'default' }),
    ),
  );
  await page.reload({ waitUntil: 'networkidle0' });
  await ready();
  assert.equal(
    await page.$$eval('.pg-conversation-items>div', (e) => e.length),
    1,
  );
  await page.waitForFunction(
    (scope) =>
      document
        .querySelector(scope + ' .pg-top-group')
        ?.textContent.includes('福利'),
    {},
    active,
  );
  assert(
    !(await page.$eval(active + ' .pg-messages', (e) =>
      e.textContent.includes('First topic'),
    )),
  );
  assert.equal(calls.length, before);
  await page.setViewport({ width: 1440, height: 1000 });
  await page.evaluate(() => document.body.setAttribute('theme-mode', 'dark'));
  await page.screenshot({ path: '/tmp/playground-focused-dark.png' });
  await page.evaluate(() => document.body.removeAttribute('theme-mode'));
  // Storage failure must be visible and recover without losing the draft.
  await page.evaluate(() => {
    window.originalPut = IDBObjectStore.prototype.put;
    IDBObjectStore.prototype.put = function () {
      throw new DOMException('fixture quota', 'QuotaExceededError');
    };
  });
  await page.type(active + ' .pg-composer textarea', 'unsaved draft');
  await page.waitForFunction(() =>
    document.body.textContent.includes('本地保存失败'),
  );
  assert.equal(
    await page.$eval(active + ' .pg-composer textarea', (e) => e.value),
    'unsaved draft',
  );
  await page.evaluate(() => {
    IDBObjectStore.prototype.put = window.originalPut;
    delete window.originalPut;
  });
  await clickText('重试保存');
  await saved();
  const importText = async (text) =>
    page.evaluate((text) => {
      const input = document.querySelector(
        '.pg-conversation-list input[type=file]',
      );
      const dt = new DataTransfer();
      dt.items.add(
        new File([text], 'backup.json', { type: 'application/json' }),
      );
      input.files = dt.files;
      input.dispatchEvent(new Event('change', { bubbles: true }));
    }, text);
  await importText('{broken');
  await page.waitForFunction(() =>
    document.body.textContent.includes('对话备份格式无效'),
  );
  assert.equal(calls.length, before);
  const parallel = {
    ...snapshot.sessions[0],
    messages: [],
    draft: '',
    parallelMode: true,
    parallelModels: ['model-a', 'model-b', 'model-c', 'model-d', 'model-e'],
    config: {
      ...snapshot.sessions[0].config,
      inputs: {
        ...snapshot.sessions[0].config.inputs,
        stream: true,
        systemPrompt: '',
      },
    },
  };
  await importText(
    JSON.stringify({
      format: 'meta-api-local-conversations',
      version: 1,
      sessions: [parallel],
    }),
  );
  await page.waitForSelector(active + ' .pg-parallel-composer textarea');
  await page.type(active + ' .pg-parallel-composer textarea', 'Five models');
  const beforeParallel = calls.length;
  hold = true;
  await page.waitForFunction(
    (scope) =>
      [...document.querySelector(scope).querySelectorAll('button')].some(
        (b) => b.innerText === '发送给 5 个模型' && !b.disabled,
      ),
    {},
    active,
  );
  await clickText('发送给 5 个模型', active);
  await new Promise(async (resolve) => {
    for (let i = 0; i < 60 && calls.length < beforeParallel + 5; i++)
      await new Promise((r) => setTimeout(r, 100));
    resolve();
  });
  assert.equal(calls.length, beforeParallel + 5);
  assert.equal(new Set(calls.slice(-5).map((x) => x.session)).size, 1);
  await clickText('新建对话');
  await ready();
  assert(
    await page.$$eval('.pg-local-warning', (els) =>
      els.some((e) => e.textContent.includes('另一个对话')),
    ),
  );
  hold = false;
  held.splice(0).forEach((done) => done());
  await page.waitForFunction(
    () => !document.querySelector('.pg-local-warning'),
  );
  await saved();
  await importText(
    JSON.stringify({
      format: 'meta-api-local-conversations',
      version: 1,
      sessions: [parallel],
    }),
  );
  await page.waitForSelector(active + ' .pg-parallel-composer textarea');
  await page.type(active + ' .pg-parallel-composer textarea', 'Compare layout');
  await clickText('发送给 5 个模型', active);
  await page.waitForFunction(
    () =>
      document.querySelectorAll('.pg-answer-comparison').length > 0 &&
      document.querySelectorAll('.pg-parallel-reply').length >= 5 &&
      [...document.querySelectorAll('.pg-reply-state')].filter((e) =>
        e.textContent.includes('已完成'),
      ).length >= 5,
  );
  assert.equal(
    await page.$$eval('.pg-answer-comparison', (els) => els.length),
    1,
  );
  assert(await page.$$eval('.pg-parallel-reply', (els) => els.length >= 5));
  await page.screenshot({ path: '/tmp/playground-answer-columns.png' });
  const layoutCalls = calls.length;
  await page.click('[aria-label="向右查看回答"]');
  await page.waitForFunction(
    () => document.querySelector('.pg-round-answers').scrollLeft > 100,
  );
  await page.setViewport({ width: 390, height: 1000 });
  await page.waitForSelector('.pg-answer-tabs');
  await page.click('.pg-answer-tabs button:nth-child(3)');
  assert.equal(
    await page.$eval(
      '.pg-answer-slot:not([hidden]) article',
      (e) => e.dataset.model,
    ),
    'model-c',
  );
  assert.equal(calls.length, layoutCalls);
  await page.screenshot({ path: '/tmp/playground-comparison-mobile.png' });
  await page.setViewport({ width: 1440, height: 1000 });
  await page.waitForSelector('.pg-answer-tabs', { hidden: true });
  const retryScope = '.pg-parallel-reply[data-model="model-a"]';
  failModel = 'model-a';
  await clickText('重新生成', retryScope);
  await page.waitForFunction(
    () =>
      document.querySelector(
        '.pg-parallel-reply[data-model="model-a"] .pg-reply-state',
      )?.textContent === '失败',
  );
  assert.equal(calls.length, layoutCalls + 1);
  assert.equal(
    await page.$$eval(
      '.pg-reply-state',
      (els) => els.filter((e) => e.textContent === '已完成').length,
    ),
    4,
  );
  failModel = null;
  hold = true;
  await clickText('重试', retryScope);
  await page.waitForFunction(
    () =>
      document.querySelector(
        '.pg-parallel-reply[data-model="model-a"] .pg-reply-state',
      )?.textContent === '回答中',
  );
  assert(
    await page.$$eval('.pg-answer-continue', (els) =>
      els.every((e) => e.disabled),
    ),
  );
  await clickText('停止生成', retryScope);
  await page.waitForFunction(
    () =>
      document.querySelector(
        '.pg-parallel-reply[data-model="model-a"] .pg-reply-state',
      )?.textContent === '已停止',
  );
  hold = false;
  held.splice(0).forEach((done) => done());
  await clickText('重试', retryScope);
  await page.waitForFunction(
    () =>
      document.querySelector(
        '.pg-parallel-reply[data-model="model-a"] .pg-reply-state',
      )?.textContent === '已完成',
  );
  assert.equal(calls.length, layoutCalls + 3);
  assert(calls.slice(layoutCalls).every((c) => c.payload.model === 'model-a'));
  await clickText('用这条继续', retryScope);
  await page.type(active + ' .pg-parallel-composer textarea', 'Second round');
  await clickText('发送给 5 个模型', active);
  await page.waitForFunction(
    () =>
      document.querySelectorAll('.pg-comparison-round').length === 2 &&
      [...document.querySelectorAll('.pg-reply-state')].every(
        (e) => e.textContent === '已完成',
      ),
  );
  assert(
    calls
      .slice(-5)
      .every(
        (c) =>
          c.payload.messages.filter((m) => m.role === 'assistant').length ===
            1 &&
          c.payload.messages.find((m) => m.role === 'assistant').content ===
            'answer-model-a',
      ),
  );
  await page.evaluate(() =>
    document.querySelector('.pg-parallel-messages').scrollTo(0, 0),
  );
  await page.click('.pg-answer-icon');
  assert.equal(
    await page.$$eval(
      '.pg-round-answers.is-expanded .pg-answer-slot:not([hidden])',
      (els) => els.length,
    ),
    1,
  );
  await page.keyboard.press('Escape');
  await page.click('.pg-answer-continue');
  await page.waitForFunction(() =>
    [...document.querySelectorAll('.pg-mode-switch button')].some(
      (b) =>
        b.getAttribute('aria-pressed') === 'true' &&
        b.textContent.includes('单模型'),
    ),
  );
  await send('Continue single');
  await page.waitForFunction(
    () => !document.querySelector('[aria-label="停止生成"]'),
  );
  assert.equal(calls.at(-1).payload.model, 'model-a');
  assert.equal(
    calls.at(-1).payload.messages.filter((m) => m.role === 'assistant').length,
    2,
  );
  assert.equal(
    await page.$$eval('.pg-messages', (els) =>
      els.reduce(
        (n, e) => n + (e.textContent.match(/answer-model-b/g) || []).length,
        0,
      ),
    ),
    2,
  );
  await page.screenshot({
    path: '/tmp/playground-local-conversations-desktop.png',
  });
  const beforeResetText = await page.$eval(
    active + ' .pg-messages',
    (e) => e.textContent,
  );
  await saved();
  await page.reload({ waitUntil: 'networkidle0' });
  await ready();
  assert.equal(
    await page.$eval(active + ' .pg-messages', (e) => e.textContent),
    beforeResetText,
  );
  await page.click('.pg-tool-rail [aria-label="对话设置"]');
  await page.waitForSelector('.pg-docked-settings');
  await clickText('恢复默认', '.pg-docked-settings');
  await page.waitForSelector('.semi-modal-confirm');
  await clickText('恢复默认', '.semi-modal-confirm');
  await page.waitForFunction(
    () => !document.querySelector('.semi-modal-confirm'),
  );
  assert.equal(
    await page.$eval(active + ' .pg-messages', (e) => e.textContent),
    beforeResetText,
  );
  await page.click('.pg-docked-settings [aria-label="关闭设置"]');
  await page.click(active + ' [aria-label="清空对话"]');
  await page.waitForSelector('.semi-modal-confirm');
  await clickText('取消', '.semi-modal-confirm');
  await page.waitForFunction(
    () => !document.querySelector('.semi-modal-confirm'),
  );
  assert.equal(
    await page.$eval(active + ' .pg-messages', (e) => e.textContent),
    beforeResetText,
  );
  await page.click(active + ' [aria-label="清空对话"]');
  await page.waitForSelector('.semi-modal-confirm');
  await clickText('清空对话', '.semi-modal-confirm');
  await page.waitForFunction(
    () => !document.querySelector('.semi-modal-confirm'),
  );
  assert(
    !(await page.$eval(active + ' .pg-messages', (e) =>
      e.textContent.includes('answer-model-a'),
    )),
  );
  // Non-streaming request remains cancellable and cannot leak to a new session.
  const nonstream = {
    ...parallel,
    parallelMode: false,
    config: {
      ...parallel.config,
      inputs: { ...parallel.config.inputs, stream: false },
    },
  };
  await importText(
    JSON.stringify({
      format: 'meta-api-local-conversations',
      version: 1,
      sessions: [nonstream],
    }),
  );
  await ready();
  hold = true;
  await send('Nonstream held');
  await page.waitForSelector(active + ' [aria-label="停止生成"]');
  await page.click(active + ' [aria-label="停止生成"]');
  await page.waitForFunction(
    () => !document.querySelector('[aria-label="停止生成"]'),
  );
  await clickText('新建对话');
  await ready();
  hold = false;
  held.splice(0).forEach((done) => done());
  assert(
    !(await page.$eval(active + ' .pg-messages', (e) =>
      e.textContent.includes('answer-model-a'),
    )),
  );
  // Paid service tier UI sends exact values, persists intent, and never silently downgrades.
  await page.setViewport({ width: 1440, height: 1000 });
  await page.click('.pg-tool-rail [aria-label="对话设置"]');
  await page.waitForSelector('#pg-service-tier:not(:disabled)');
  await page.select('#pg-service-tier', 'priority');
  await page.screenshot({ path: '/tmp/muze-service-tier-desktop.png' });
  await send('Fast fixture');
  await page.waitForFunction(
    () => !document.querySelector('[aria-label="停止生成"]'),
  );
  await saved();
  assert.equal(calls.at(-1).payload.service_tier, 'priority');
  await page.select('#pg-service-tier', 'ultrafast');
  await send('UltraFast fixture');
  await page.waitForFunction(
    () => !document.querySelector('[aria-label="停止生成"]'),
  );
  await saved();
  assert.equal(calls.at(-1).payload.service_tier, 'ultrafast');
  await page.setViewport({ width: 390, height: 900 });
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  );
  await page.waitForFunction(() => {
    const r = document
      .querySelector('#pg-service-tier')
      ?.getBoundingClientRect();
    return r && r.left >= 0 && r.right <= innerWidth;
  });
  await page.waitForSelector('.semi-sidesheet', {visible:true});
  await page.waitForFunction(()=>{const r=document.querySelector('#pg-service-tier')?.getBoundingClientRect();return r&&r.left>=0&&r.right<=innerWidth&&r.width>150;});
  await page.evaluate(()=>{document.documentElement.classList.add('dark');document.body.setAttribute('theme-mode','dark');});
  await page.screenshot({ path: '/tmp/muze-service-tier-mobile.png' });
  const callsBeforeLostCapability = calls.length;
  offeredTiers = ['default'];
  await page.setViewport({ width: 1440, height: 1000 });
  await page.reload({ waitUntil: 'networkidle0' });
  await ready();
  await page.click('.pg-tool-rail [aria-label="对话设置"]');
  await page.waitForSelector('#pg-service-tier[aria-invalid="true"]');
  assert.equal(
    await page.$eval('#pg-service-tier', (e) => e.value),
    'ultrafast',
  );
  await page.click('.pg-docked-settings [aria-label="关闭设置"]');
  await page.type(active + ' .pg-composer textarea', 'Must not downgrade');
  await page.click(active + ' button[aria-label="发送"]');
  assert.equal(calls.length, callsBeforeLostCapability);
  await page.click('.pg-tool-rail [aria-label="对话设置"]');
  await page.select('#pg-service-tier', 'default');
  await page.click('.pg-docked-settings [aria-label="关闭设置"]');
  await page.click(active + ' button[aria-label="发送"]');
  await page.waitForFunction(
    () => !document.querySelector('[aria-label="停止生成"]'),
  );
  await saved();
  assert.equal(calls.length, callsBeforeLostCapability + 1);
  assert(!('service_tier' in calls.at(-1).payload));
  assert.deepEqual(errors, []);
  console.log(
    'PASS local conversations: independent context/IDs, background stream, send lock, reload interruption, account/tab isolation, rename/search/import/delete, storage failure, 5 models, nonstream stop, mobile.',
  );
} catch (error) {
  const page = (await browser?.pages())?.at(-1);
  if (page) {
    console.error((await page.$eval('body', (e) => e.innerText)).slice(-1800));
    await page.screenshot({ path: '/tmp/playground-sessions-failure.png' });
  }
  throw error;
} finally {
  await browser?.close();
  for (const socket of sockets) socket.destroy();
  await new Promise((r) => server.close(r));
  console.log('Browser and local server closed; no retained test processes.');
}
