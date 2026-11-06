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
const sockets = new Set();
const server = createServer(async (req, res) => {
  if (req.url === '/pg/chat/completions') {
    let body = '';
    for await (const part of req) body += part;
    const payload = JSON.parse(body);
    calls.push({ payload, session: req.headers['x-session-id'] });
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
            quota_display_type: 'USD',
          };
        if (u.pathname === '/api/user/self')
          data = await page.evaluate(() =>
            JSON.parse(localStorage.getItem('user')),
          );
        if (u.pathname === '/api/user/models')
          data =
            u.searchParams.get('group') === 'limited'
              ? ['model-a']
              : ['model-a', 'model-b', 'model-c', 'model-d', 'model-e'];
        if (u.pathname === '/api/user/self/groups')
          data = {
            default: { ratio: 1, desc: 'Default' },
            limited: { ratio: 2, desc: 'Limited' },
          };
        if (u.pathname === '/api/pricing')
          body = {
            success: true,
            data: ['model-a', 'model-b', 'model-c', 'model-d', 'model-e'].map(
              (model_name) => ({
                model_name,
                enable_groups:
                  model_name === 'model-a'
                    ? ['default', 'limited']
                    : ['default'],
                quota_type: 0,
                supported_endpoint_types: ['openai'],
                output_modalities: ['text'],
                model_ratio: 1,
                completion_ratio: 2,
              }),
            ),
            group_ratio: { default: 1, limited: 2 },
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
  await page.click('.pg-tool-rail [aria-label="参数"]');
  await page.waitForSelector('.pg-sheet .pg-settings-scroll', {
    visible: true,
  });
  await page.keyboard.press('Escape');
  await page.waitForFunction(
    () =>
      document
        .querySelector('.pg-tool-rail [aria-label="参数"]')
        ?.getAttribute('aria-expanded') === 'false',
  );
  await page.waitForSelector('.pg-sheet .pg-settings-scroll', { hidden: true });
  await page.click('.pg-tool-rail [aria-label="请求代码"]');
  await page.waitForSelector('.pg-sheet .pg-code-panel', { visible: true });
  await page.keyboard.press('Escape');
  await page.waitForFunction(
    () =>
      document
        .querySelector('.pg-tool-rail [aria-label="请求代码"]')
        ?.getAttribute('aria-expanded') === 'false',
  );
  await page.waitForSelector('.pg-sheet .pg-code-panel', { hidden: true });
  await clickText('对话列表');
  await page.waitForFunction(
    () => document.activeElement?.getAttribute('aria-label') === '搜索对话标题',
  );
  await page.keyboard.press('Escape');
  assert.equal(
    await page.$eval('.pg-conversation-list', (e) => e.hidden),
    true,
  );
  await page.click(active + ' .pg-model-tab.is-active button[role=tab]');
  await page.waitForSelector('.pg-model-options');
  await clickText('model-b', '.pg-model-options');
  await page.select(active + ' select[aria-label="当前分组"]', 'limited');
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
  await page.select(active + ' select[aria-label="当前分组"]', 'default');
  await page.waitForSelector('.pg-model-options', { hidden: true });
  await saved();
  assert.equal(calls.length, 0);
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
  await page.screenshot({
    path: '/tmp/playground-local-conversations-desktop.png',
  });
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
