import assert from 'node:assert/strict';
import puppeteer from 'puppeteer-core';
const base = process.env.PLAYGROUND_TEST_URL || 'http://127.0.0.1:5197';
const browser = await puppeteer.launch({
  executablePath:
    process.env.CHROME_PATH ||
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  headless: true,
});
const requests = [],
  errors = [];
let failB = true,
  hold = false;
const held = [];
const user = { id: 7, role: 1, username: 'parallel-fixture', group: 'default' };
const catalog = [
  'model-a',
  'model-b',
  'model-c',
  'model-d',
  'model-e',
  'model-f',
].map((model_name) => ({
  model_name,
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 2,
  enable_groups: ['default'],
  supported_endpoint_types: ['openai'],
  output_modalities: ['text'],
}));
catalog.push({
  model_name: 'image-only',
  supported_endpoint_types: ['image-generation'],
  output_modalities: ['image'],
  enable_groups: ['default'],
});
const config = {
  inputs: {
    model: 'model-a',
    modelTabs: ['model-a'],
    systemPrompt: 'Same system',
    group: 'default',
    stream: true,
    toolsEnabled: false,
    imageEnabled: false,
  },
  parameterEnabled: {},
  showDebugPanel: false,
  customRequestMode: false,
  customRequestBody: '',
};
try {
  const page = await browser.newPage();
  page.setDefaultTimeout(15000);
  page.on('pageerror', (e) => errors.push(e.message));
  await page.setViewport({ width: 1440, height: 1000 });
  await page.evaluateOnNewDocument(
    (user, config) => {
      if (!localStorage.getItem('parallel-fixture-init')) {
        localStorage.setItem('user', JSON.stringify(user));
        localStorage.setItem(
          'playground_config:user:7',
          JSON.stringify(config),
        );
        localStorage.setItem('parallel-fixture-init', 'yes');
      }
      localStorage.setItem('i18nextLng', 'zh-CN');
      localStorage.setItem('site-language', 'zh-CN');
      localStorage.setItem('newapi:user-onboarding:regular:v1:user:7', 'done');
    },
    user,
    config,
  );
  await page.setRequestInterception(true);
  page.on('request', async (req) => {
    const url = new URL(req.url());
    if (url.origin !== base) return req.abort();
    if (url.pathname === '/pg/chat/completions') {
      const payload = JSON.parse(req.postData());
      requests.push(payload);
      const answer = () =>
        req.respond(
          payload.model === 'model-b' && failB
            ? {
                status: 503,
                contentType: 'application/json',
                body: JSON.stringify({
                  error: { message: 'fixture unavailable' },
                }),
              }
            : payload.stream === false
              ? {
                  status: 200,
                  contentType: 'application/json',
                  body: JSON.stringify({
                    choices: [
                      {
                        message: {
                          role: 'assistant',
                          content: payload.model + ' answer',
                        },
                        finish_reason: 'stop',
                      },
                    ],
                  }),
                }
              : {
                  status: 200,
                  contentType: 'text/event-stream',
                  body: `data: ${JSON.stringify({ choices: [{ delta: { content: payload.model + ' answer' } }] })}\n\ndata: ${JSON.stringify({ choices: [{ delta: {}, finish_reason: 'stop' }] })}\n\ndata: [DONE]\n\n`,
                },
        );
      if (hold) {
        held.push(Object.assign(answer, { model: payload.model }));
        return;
      }
      setTimeout(
        () => answer().catch(() => {}),
        payload.model === 'model-a' ? 300 : 70,
      );
      return;
    }
    if (!url.pathname.startsWith('/api/')) return req.continue();
    let body = { success: true, data: [] };
    if (url.pathname === '/api/status')
      body.data = {
        system_name: 'Fixture',
        quota_per_unit: 500000,
        quota_display_type: 'USD',
        HeaderNavModules: JSON.stringify({
          pricing: { enabled: true, requireAuth: false },
        }),
      };
    if (url.pathname === '/api/user/self') body.data = user;
    if (url.pathname === '/api/user/models')
      body.data =
        url.searchParams.get('group') === 'limited'
          ? ['model-a']
          : catalog.map((m) => m.model_name);
    if (url.pathname === '/api/user/self/groups')
      body.data = {
        default: { ratio: 1, desc: 'Default' },
        limited: { ratio: 1, desc: 'Limited' },
      };
    if (url.pathname === '/api/pricing')
      body = {
        success: true,
        data: catalog,
        usable_group: { default: 'Default' },
        group_ratio: { default: 1 },
      };
    await req.respond({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(body),
    });
  });
  const clickText = async (text, scope = '') =>
    page.evaluate(
      ({ text, scope }) => {
        const root = scope ? document.querySelector(scope) : document;
        const el = [...root.querySelectorAll('button')].find(
          (b) =>
            b.innerText.trim() === text ||
            (text === '同时发送' &&
              /^发送给 \d+ 个模型$/.test(b.innerText.trim())),
        );
        if (!el) throw Error('Missing ' + text);
        el.click();
      },
      { text, scope },
    );
  const selectModels = async (names) => {
    await page.click('.pg-parallel-selection .semi-select');
    await page.waitForSelector('.semi-select-option');
    const options = await page.$$eval('.semi-select-option', (els) =>
      els.map((e) => e.textContent),
    );
    assert.ok(!options.some((t) => t.includes('image-only')));
    for (const name of names) {
      await page.evaluate(
        (name) =>
          [...document.querySelectorAll('.semi-select-option')]
            .find((e) => e.textContent.trim() === name)
            ?.click(),
        name,
      );
      await page.waitForFunction(
        (name) =>
          document
            .querySelector('.pg-parallel-selection .semi-select')
            ?.textContent.includes(name),
        {},
        name,
      );
    }
    await page.keyboard.press('Escape');
  };
  await page.goto(base + '/playground');
  await page.waitForFunction(() =>
    [...document.querySelectorAll('button')].some(
      (b) => b.textContent === '多模型同时回答' && !b.disabled,
    ),
  );
  await clickText('多模型同时回答');
  await page.waitForSelector('.pg-parallel-selection .semi-select');
  await selectModels(['model-a', 'model-b']);
  assert.equal(await page.$('.pg-model-tabs'), null);
  assert.equal(
    await page.$eval(
      '.pg-mode-switch button[aria-pressed="true"]',
      (el) => el.innerText,
    ),
    '多模型同时回答',
  );
  assert.equal(
    await page
      .$$eval('.pg-inspector-models button', (els) =>
        els.map((el) => el.innerText),
      )
      .then((names) => names.join(',')),
    'model-a,model-b',
  );
  await clickText('model-b', '.pg-inspector-models');
  await page.waitForFunction(
    () =>
      document.querySelector('.pg-inspector-models button[aria-pressed="true"]')
        ?.innerText === 'model-b',
  );
  await page.click('.pg-inspector-details summary');
  await page.waitForSelector('.pg-inspector-details[open] .pg-model-card');
  assert.equal(
    await page.$eval(
      '.pg-inspector-details .pg-model-identity h2',
      (el) => el.innerText,
    ),
    'model-b',
  );
  assert.equal(requests.length, 0);
  await clickText('单模型回答', '.pg-mode-switch');
  await page.waitForSelector('.pg-model-tabs');
  await clickText('多模型同时回答', '.pg-mode-switch');
  await page.waitForSelector('.pg-parallel-selection');
  assert.equal(
    await page.$$eval('.pg-inspector-models button', (els) => els.length),
    2,
  );
  await page.type('.pg-parallel-composer textarea', 'First question');
  await clickText('同时发送');
  await clickText('同时发送').catch(() => {});
  await page.waitForFunction(
    () =>
      document
        .querySelector('.pg-parallel-messages')
        ?.textContent.includes('model-a answer') &&
      document
        .querySelector('.pg-parallel-messages')
        ?.textContent.includes('fixture unavailable'),
  );
  assert.equal(requests.length, 2);
  assert.deepEqual(requests[0].messages, requests[1].messages);
  assert.equal(
    await page.$eval('.pg-round-heading', (el) =>
      el.innerText.includes('已结束 2/2'),
    ),
    true,
  );
  assert.equal(
    await page.$eval('.pg-round-context', (el) =>
      el.innerText.includes('请选择'),
    ),
    true,
  );
  assert.equal(await page.$('.pg-parallel-selection [role="status"]'), null);
  await clickText('列表', '.pg-layout-switch');
  await page.waitForSelector('.pg-round-answers.is-list');
  await clickText('并排', '.pg-layout-switch');
  await page.waitForSelector('.pg-round-answers.is-grid');
  const pair = await page.$$eval(
    '.pg-round-answers .pg-parallel-reply',
    (els) =>
      els.map((el) => ({
        x: el.getBoundingClientRect().x,
        y: el.getBoundingClientRect().y,
      })),
  );
  assert.equal(pair[0].y, pair[1].y);
  assert.notEqual(pair[0].x, pair[1].x);
  assert.equal(requests.length, 2);
  failB = false;
  await clickText('重试', '.pg-parallel');
  await page.waitForFunction(() =>
    document
      .querySelector('.pg-parallel-messages')
      ?.textContent.includes('model-b answer'),
  );
  assert.equal(requests.length, 3);
  assert.equal(requests[2].model, 'model-b');
  assert.deepEqual(requests[2].messages, requests[0].messages);
  await page.evaluate(() => {
    const card = [...document.querySelectorAll('.pg-parallel-reply')].find(
      (x) => x.querySelector('strong')?.textContent === 'model-a',
    );
    [...card.querySelectorAll('button')]
      .find((b) => b.innerText === '用这条继续')
      .click();
  });
  await page.waitForFunction(() =>
    document
      .querySelector('.pg-round-context')
      ?.innerText.includes('下一轮使用 model-a 的回答'),
  );
  await page.screenshot({
    path: '/tmp/playground-comparison-selected-desktop.png',
  });
  await page.type('.pg-parallel-composer textarea', 'Next question');
  await clickText('同时发送');
  await page.waitForFunction(
    () =>
      document.querySelectorAll('.pg-parallel-reply').length === 6 &&
      [...document.querySelectorAll('.pg-parallel-reply')].filter((e) =>
        e.innerText.includes('用这条继续'),
      ).length === 2,
  );
  assert.equal(requests.length, 5);
  assert.deepEqual(requests[3].messages, requests[4].messages);
  assert.deepEqual(
    requests[3].messages.map((m) => m.content),
    ['Same system', 'First question', 'model-a answer', 'Next question'],
  );
  assert.equal(
    await page.$$eval('.pg-round-context', (els) =>
      els[0].innerText.includes('本轮已采用 model-a 的回答'),
    ),
    true,
  );
  await clickText('清空对话', '.pg-parallel');
  await selectModels(['model-c', 'model-d', 'model-e']);
  await page.click('.pg-parallel-selection .semi-select');
  assert.equal(
    await page.$$eval('.semi-select-option', (options) =>
      options
        .find((el) => el.textContent.trim() === 'model-f')
        ?.getAttribute('aria-disabled'),
    ),
    'true',
  );
  await page.keyboard.press('Escape');
  hold = true;
  await page.type('.pg-parallel-composer textarea', 'Stop test');
  await clickText('同时发送');
  await page.waitForFunction(
    () => document.querySelectorAll('.pg-parallel-reply').length === 6,
  );
  await new Promise((r) => setTimeout(r, 150));
  assert.equal(requests.length, 10);
  assert.equal(new Set(requests.slice(-5).map((r) => r.model)).size, 5);
  for (const request of requests.slice(-5))
    assert.deepEqual(request.messages, requests.at(-1).messages);
  await clickText('停止生成', '.pg-parallel-reply[data-model="model-a"]');
  const bIndex = held.findIndex((fn) => fn.model === 'model-b');
  await held.splice(bIndex, 1)[0]();
  await page.waitForFunction(() =>
    document
      .querySelector('.pg-parallel-reply[data-model="model-b"]')
      ?.textContent.includes('用这条继续'),
  );
  await clickText('停止生成', '.pg-parallel-composer');
  await Promise.all(held.splice(0).map((fn) => fn().catch(() => {})));
  hold = false;
  await page.waitForFunction(() =>
    document
      .querySelector('.pg-parallel-reply[data-model="model-c"]')
      ?.innerText.includes('已停止'),
  );
  assert.ok(
    !(
      await page.$eval(
        '.pg-parallel-reply[data-model="model-a"]',
        (el) => el.textContent,
      )
    ).includes('model-a answer'),
  );
  assert.equal(
    await page.$eval('.pg-parallel-reply[data-model="model-b"]', (el) =>
      el.textContent.includes('已停止'),
    ),
    false,
  );
  await page.setViewport({ width: 390, height: 900 });
  const mobilePair = await page.$$eval(
    '.pg-round-answers .pg-parallel-reply',
    (els) =>
      els.slice(0, 2).map((el) => ({
        x: el.getBoundingClientRect().x,
        y: el.getBoundingClientRect().y,
      })),
  );
  assert.equal(mobilePair[0].x, mobilePair[1].x);
  assert.notEqual(mobilePair[0].y, mobilePair[1].y);
  assert.equal(
    await page.evaluate(
      () => document.documentElement.scrollWidth > innerWidth + 1,
    ),
    false,
  );
  await page.screenshot({ path: '/tmp/playground-parallel-mobile.png' });
  assert.equal(
    await page.$eval('.pg-parallel-user-text', (el) => el.innerText),
    'Stop test',
  );
  await page.setViewport({ width: 1440, height: 1000 });
  await page.screenshot({ path: '/tmp/playground-parallel-desktop.png' });
  await page.setViewport({ width: 390, height: 900 });
  const before = requests.length;
  await page.reload();
  await page.waitForSelector('.pg-parallel');
  assert.equal(requests.length, before);
  assert.ok(
    (await page.$eval('.pg-parallel-messages', (e) => e.textContent)).includes(
      '已停止',
    ),
  );
  // Non-stream requests use the same isolation and original-history contract.
  await clickText('清空对话', '.pg-parallel');
  await page.evaluate(() => {
    const key = 'playground_config:user:7';
    const config = JSON.parse(localStorage.getItem(key));
    config.inputs.stream = false;
    localStorage.setItem(key, JSON.stringify(config));
  });
  await page.reload();
  await page.waitForSelector('.pg-parallel-toggle');
  await clickText('多模型同时回答');
  await page.waitForSelector('.pg-parallel-selection .semi-select');
  await selectModels(['model-a', 'model-b']);
  await page.type('.pg-parallel-composer textarea', 'JSON test');
  await clickText('同时发送');
  await page.waitForFunction(
    () =>
      document
        .querySelector('.pg-parallel-messages')
        ?.textContent.includes('model-a answer') &&
      document
        .querySelector('.pg-parallel-messages')
        ?.textContent.includes('model-b answer'),
  );
  assert.equal(requests.length, before + 2);
  assert.equal(requests.at(-1).stream, false);
  assert.deepEqual(requests.at(-1).messages, requests.at(-2).messages);
  await clickText('清空对话', '.pg-parallel');
  hold = true;
  await page.type('.pg-parallel-composer textarea', 'Account switch');
  await clickText('同时发送');
  await page.waitForFunction(
    () => document.querySelectorAll('.pg-parallel-reply').length === 3,
  );
  await new Promise((r) => setTimeout(r, 150));
  await page.evaluate(() =>
    localStorage.setItem(
      'user',
      JSON.stringify({
        id: 9,
        role: 1,
        username: 'other-fixture',
        group: 'default',
      }),
    ),
  );
  await Promise.all(held.splice(0).map((fn) => fn().catch(() => {})));
  hold = false;
  await new Promise((r) => setTimeout(r, 300));
  assert.equal(
    await page.$eval('.pg-parallel-messages', (e) =>
      e.textContent.includes('model-a answer'),
    ),
    false,
  );
  assert.equal(
    await page.evaluate(() =>
      localStorage.getItem('playground_messages:user:9'),
    ),
    null,
  );
  // Reload interrupted calls: placeholders are stopped, no automatic retries.
  await page.evaluate(() =>
    localStorage.setItem(
      'user',
      JSON.stringify({
        id: 7,
        role: 1,
        username: 'parallel-fixture',
        group: 'default',
      }),
    ),
  );
  const stoppedCount = requests.length;
  await page.reload();
  await page.waitForSelector('.pg-parallel');
  assert.equal(requests.length, stoppedCount);
  assert.equal(
    await page.$$eval(
      '.pg-parallel-reply',
      (els) => els.filter((e) => e.textContent.includes('已停止')).length,
    ),
    2,
  );
  await clickText('撤销本轮', '.pg-parallel');
  await page.waitForFunction(
    () => document.querySelectorAll('.pg-parallel-reply').length === 0,
  );
  // Group permissions prune both the sending list and the independent inspector.
  await page.setViewport({ width: 1440, height: 1000 });
  await selectModels(['model-a', 'model-b']);
  const noNewRequests = requests.length;
  await clickText('model-b', '.pg-inspector-models');
  await page.click('.pg-group-section .semi-select');
  await page.waitForFunction(() =>
    [...document.querySelectorAll('.semi-typography')].some(
      (el) => el.textContent === 'Limited',
    ),
  );
  await page.evaluate(() =>
    [...document.querySelectorAll('.semi-typography')]
      .find((el) => el.textContent === 'Limited')
      ?.click(),
  );
  await page.waitForFunction(
    () => document.querySelectorAll('.pg-inspector-models button').length === 1,
  );
  assert.equal(
    await page.$eval('.pg-inspector-models button', (el) => el.innerText),
    'model-a',
  );
  await page.type('.pg-parallel-composer textarea', 'Must not send');
  assert.equal(
    await page.$$eval(
      '.pg-parallel-composer button',
      (els) => els.find((el) => el.innerText.startsWith('发送给')).disabled,
    ),
    true,
  );
  assert.equal(requests.length, noNewRequests);
  await page.evaluate(() => localStorage.setItem('theme-mode', 'dark'));
  await page.reload();
  await page.waitForSelector('.pg-parallel-toggle');
  await clickText('多模型同时回答', '.pg-mode-switch');
  await page.waitForSelector('.pg-parallel');
  await page.screenshot({ path: '/tmp/playground-comparison-dark.png' });
  await page.evaluate(() => {
    const key = 'playground_config:user:7';
    const conf = JSON.parse(localStorage.getItem(key));
    conf.customRequestMode = true;
    conf.customRequestBody = JSON.stringify({
      model: 'model-a',
      messages: [{ role: 'user', content: 'custom' }],
      stream: false,
    });
    localStorage.setItem(key, JSON.stringify(conf));
    localStorage.setItem(
      'playground_messages:user:7',
      JSON.stringify({
        messages: [
          { id: 'q', role: 'user', content: 'q' },
          {
            id: 'a',
            role: 'assistant',
            parallelRound: 'q',
            parallelModel: 'model-a',
            parallelChosen: true,
            status: 'complete',
            content: 'done',
          },
        ],
      }),
    );
  });
  const beforeCustom = requests.length;
  await page.reload();
  await page.waitForFunction(
    () =>
      document.querySelector('.pg-mode-switch button[aria-pressed="true"]')
        ?.innerText === '单模型回答',
  );
  assert.equal(await page.$('.pg-parallel'), null);
  assert.equal(requests.length, beforeCustom);
  assert.deepEqual(errors, []);
  console.log(
    'PASS: concurrent isolated replies, equal snapshots, failure-only retry, chosen continuation, 5 models with sixth disabled, stop/late response, mobile, reload without resending.',
  );
} catch (error) {
  const pages = await browser.pages();
  const page = pages.at(-1);
  console.error(
    'Fixture errors',
    errors,
    'Requests',
    requests.length,
    'URL',
    page.url(),
  );
  console.error((await page.$eval('body', (el) => el.innerText)).slice(-3500));
  await page.screenshot({ path: '/tmp/playground-parallel-error.png' });
  throw error;
} finally {
  await browser.close();
}
