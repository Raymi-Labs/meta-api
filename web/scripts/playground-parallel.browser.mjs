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
      body.data = catalog.map((m) => m.model_name);
    if (url.pathname === '/api/user/self/groups')
      body.data = { default: { ratio: 1, desc: 'Default' } };
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
          (b) => b.innerText.trim() === text,
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
  await clickText('停止生成', '.pg-parallel-reply:nth-child(2)');
  const bIndex = held.findIndex((fn) => fn.model === 'model-b');
  await held.splice(bIndex, 1)[0]();
  await page.waitForFunction(() =>
    document
      .querySelector('.pg-parallel-reply:nth-child(3)')
      ?.textContent.includes('用这条继续'),
  );
  await clickText('停止生成', '.pg-parallel-composer');
  await Promise.all(held.splice(0).map((fn) => fn().catch(() => {})));
  hold = false;
  await page.waitForFunction(() =>
    document
      .querySelector('.pg-parallel-reply:nth-child(4)')
      ?.innerText.includes('已停止'),
  );
  assert.ok(
    !(
      await page.$eval(
        '.pg-parallel-reply:nth-child(2)',
        (el) => el.textContent,
      )
    ).includes('model-a answer'),
  );
  assert.equal(
    await page.$eval('.pg-parallel-reply:nth-child(3)', (el) =>
      el.textContent.includes('已停止'),
    ),
    false,
  );
  await page.setViewport({ width: 390, height: 900 });
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
