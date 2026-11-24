import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { preview } from 'vite';
import puppeteer from 'puppeteer-core';

// Exercise the production bundle against local, account-scoped fixtures.
const server = await preview({
  configFile: false,
  root: fileURLToPath(new URL('..', import.meta.url)),
  preview: { host: '127.0.0.1', port: 0 },
});
const origin = server.resolvedUrls.local[0];
let browser;
try {
  browser = await puppeteer.launch({
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  });
  const page = await browser.newPage();
  page.setDefaultTimeout(12000);
  const status = {
    system_name: 'New API',
    quota_per_unit: 500000,
    quota_display_type: 'USD',
    enable_data_export: true,
    server_address: 'https://gateway.example/v1',
    uptime_kuma_enabled: true,
  };
  let user = {
    id: 701,
    role: 1,
    group: 'default',
    username: 'fixture',
    quota: 0,
    used_quota: 0,
    request_count: 0,
  };
  let keys = 0,
    keyError = false,
    models = ['image-only', 'glm-5', 'gpt-6-sol'];
  let counts = [
    { model_name: 'private-chat', request_count: 1000 },
    { model_name: 'image-only', request_count: 900 },
    { model_name: 'glm-5', request_count: 8 },
    { model_name: 'gpt-6-sol', request_count: 12 },
  ];
  let failUsage = false;
  const requests = [];
  const errors = [];
  page.on('pageerror', (err) => errors.push(err.message));
  await page.setRequestInterception(true);
  page.on('request', async (req) => {
    const url = new URL(req.url());
    if (url.origin !== new URL(origin).origin) return req.abort();
    if (!url.pathname.startsWith('/api/')) return req.continue();
    requests.push(
      url.pathname +
        (url.pathname === '/api/perf-metrics/summary' ? url.search : ''),
    );
    const fixtures = {
      '/api/status': status,
      '/api/home_page_content': '',
      '/api/notice': '',
      '/api/user/self': user,
      '/api/token/': { total: keys, items: [] },
      '/api/user/models': models,
      '/api/perf-metrics/summary': { models: counts },
      '/api/pricing': [
        { model_name: 'private-chat', supported_endpoint_types: ['openai'] },
        {
          model_name: 'image-only',
          supported_endpoint_types: ['image-generation'],
        },
        { model_name: 'glm-5', supported_endpoint_types: ['openai'] },
        { model_name: 'gpt-6-sol', supported_endpoint_types: ['openai'] },
      ],
      '/api/uptime/status': [
        {
          categoryName: 'fixture',
          monitors: [
            { name: 'edge', status: 1 },
            { name: 'backup', status: 0 },
          ],
        },
      ],
    };
    await req.respond({
      contentType: 'application/json',
      body: JSON.stringify({
        success:
          !(keyError && url.pathname === '/api/token/') &&
          !(failUsage && url.pathname === '/api/perf-metrics/summary'),
        data: fixtures[url.pathname] ?? [],
      }),
    });
  });
  await page.goto(origin);
  const enter = async () => {
    await page.evaluate(
      (u, s) => {
        localStorage.setItem('user', JSON.stringify(u));
        localStorage.setItem('status', JSON.stringify(s));
        localStorage.setItem('site-language', 'zh-CN');
        localStorage.setItem('enable_data_export', 'true');
        localStorage.setItem('quota_per_unit', '500000');
      },
      user,
      status,
    );
    await page.goto(origin + 'console', { waitUntil: 'networkidle0' });
  };
  const toggle = '.dashboard-setup-toggle',
    guide = '.dashboard-setup';
  const text = () => page.$eval(guide, (el) => el.textContent);
  await page.setViewport({ width: 390, height: 844 });
  await enter();
  assert.equal(
    await page.$eval(toggle, (e) => e.getAttribute('aria-expanded')),
    'true',
  );
  assert.match(await text(), /0\s*\/\s*3/);
  assert.match(await text(), /正常 1\/2/);
  const code = await page.$eval(guide + ' code', (el) => el.textContent);
  assert.match(code, /https:\/\/gateway.example\/v1\/chat\/completions/);
  assert.match(code, /"model": "gpt-6-sol"/);
  assert.ok(requests.includes('/api/perf-metrics/summary?hours=24'));
  const checkAfterRefresh = async (model) => {
    const refresh =
      '.dashboard-query-actions button[aria-label="刷新"]:not(.semi-button-loading)';
    await page.waitForSelector(refresh, { visible: true });
    await Promise.all([
      page.waitForResponse(
        (res) =>
          new URL(res.url()).pathname === '/api/perf-metrics/summary' &&
          res.ok(),
      ),
      page.click(refresh),
    ]);
    await page.waitForFunction(
      (selector, expected) => {
        const panel = document.querySelector(selector);
        return (
          panel
            ?.querySelector('code')
            ?.textContent.includes('"model": "' + expected + '"') &&
          [...panel.querySelectorAll('dd')].some(
            (el) => el.textContent === expected,
          )
        );
      },
      {},
      guide,
      model,
    );
  };
  counts[2].request_count = 15;
  await checkAfterRefresh('glm-5');
  counts[3].request_count = 15;
  await checkAfterRefresh('gpt-6-sol');
  counts = [];
  await checkAfterRefresh('gpt-6-sol');
  failUsage = true;
  await checkAfterRefresh('gpt-6-sol');
  models = ['glm-5', 'image-only'];
  await checkAfterRefresh('glm-5');
  failUsage = false;
  assert.match(code, /Bearer \$META_API_KEY/);
  assert.equal(await page.$('.newapi-onboarding-layer'), null);
  assert.equal(await page.$(guide + ' a[href="/console/channel"]'), null);
  assert.ok(
    await page.$eval(guide, (el) => el.scrollWidth <= el.clientWidth + 1),
  );
  for (const path of ['/console/token', '/console/topup', '/playground']) {
    await page.click(guide + ' li a[href="' + path + '"]');
    await page.waitForFunction((path) => location.pathname === path, {}, path);
    await enter();
  }
  await page.click(toggle);
  await enter();
  assert.equal(await page.$(guide + ' code'), null);
  user = {
    ...user,
    id: 702,
    role: 10,
    quota: 500000,
    used_quota: 700,
    request_count: 4,
  };
  keys = 15;
  requests.length = 0;
  await enter();
  assert.equal(
    await page.$eval(toggle, (e) => e.getAttribute('aria-expanded')),
    'false',
  );
  assert.ok(!requests.includes('/api/user/models'));
  await page.focus(toggle);
  await page.keyboard.press('Enter');
  await page.waitForSelector(guide + ' code');
  assert.ok(await page.$(guide + ' a[href="/console/channel"]'));
  user = { ...user, id: 703 };
  keyError = true;
  await enter();
  assert.match(await text(), /加载失败，请重试/);
  keyError = false;
  await page.click('.dashboard-setup-grid > div button');
  await page.waitForFunction(
    (sel) =>
      document.querySelector(sel)?.getAttribute('aria-expanded') === 'false',
    {},
    toggle,
  );
  models = [];
  await page.click(toggle);
  await page.waitForFunction(
    (sel) => document.querySelector(sel)?.textContent.includes('暂无可用模型'),
    {},
    guide,
  );
  assert.equal(await page.$(guide + ' code'), null);
  status.SidebarModulesAdmin = JSON.stringify({
    console: { token: false },
    personal: { topup: false },
    admin: { channel: false },
  });
  status.HeaderNavModules = JSON.stringify({ console: true, pricing: false });
  await enter();
  for (const path of [
    '/console/token',
    '/console/topup',
    '/console/channel',
    '/pricing',
  ]) {
    assert.equal(await page.$(guide + ' a[href="' + path + '"]'), null);
  }
  assert.ok(
    !requests.some((p) => p.endsWith('/key') || p.includes('completions')),
  );
  assert.deepEqual(errors, []);
  console.log(
    'PASS: setup links, first request, live progress, permissions, account preference, retry, empty models and mobile layout',
  );
} finally {
  await browser?.close();
  await new Promise((resolve) => server.httpServer.close(resolve));
}
