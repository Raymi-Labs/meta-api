import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';
import { createServer, loadConfigFromFile } from 'vite';
import { stop as stopEsbuild } from 'esbuild';

const root = fileURLToPath(new URL('..', import.meta.url));
const { config } = await loadConfigFromFile(
  { command: 'serve', mode: 'development' },
  path.join(root, 'vite.config.js'),
);
const server = await createServer({
  ...config,
  configFile: false,
  root,
  server: { host: '127.0.0.1', port: 0, open: false, proxy: {} },
  plugins: config.plugins
    .flat(Infinity)
    .filter((plugin) => plugin?.name !== '@code-inspector/vite'),
});
await server.listen();
const baseURL = `http://127.0.0.1:${server.httpServer.address().port}`;
const status = {
  system_name: 'New API',
  quota_per_unit: 500000,
  quota_display_type: 'USD',
  server_address: baseURL,
  HeaderNavModules: JSON.stringify({
    pricing: { enabled: true, requireAuth: false },
  }),
};
const modelDefaults = {
  vendor_id: 1,
  enable_groups: ['Public'],
  supported_endpoint_types: ['openai'],
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 2,
};
const pricing = {
  success: true,
  vendors: [{ id: 1, name: 'OpenAI' }],
  usable_group: { Public: 'Allowed' },
  group_ratio: { Public: 1 },
  supported_endpoint: {
    openai: { path: '/v1/chat/completions', method: 'POST' },
  },
  data: [
    {
      ...modelDefaults,
      model_name: 'same-model',
      display_name: 'same-model',
      description: '',
      billing_mode: 'tiered_expr',
      billing_expr: 'v1:tier("base", p*10+c*20)',
    },
    { ...modelDefaults, model_name: 'no-display-name' },
    {
      ...modelDefaults,
      model_name: 'friendly-id',
      display_name: 'Friendly model',
      description: 'Fixture description',
    },
  ],
};
const logRecord = {
  id: 1,
  type: 2,
  created_at: 1793145600,
  model_name: 'same-model',
  token_name: 'fixture-key',
  group: 'Public',
  quota: 100,
  prompt_tokens: 10,
  completion_tokens: 5,
  use_time: 1,
  is_stream: true,
  channel: 3,
  username: 'fixture',
  content: '',
  other: JSON.stringify({ group_ratio: 1, model_ratio: 1 }),
};

let browser;
const launchBrowser = async () => {
  browser = await puppeteer.launch({
    headless: true,
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  });
};

const makePage = async (role = 1, dark = false) => {
  const page = await browser.newPage();
  const errors = [];
  const apiRequests = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.setViewport({ width: dark ? 390 : 1440, height: 1000 });
  const user = {
    id: role === 1 ? 7 : 8,
    role,
    username: 'view-fixture',
    group: 'default',
  };
  await page.evaluateOnNewDocument(
    (user, status, dark) => {
      localStorage.setItem('user', JSON.stringify(user));
      localStorage.setItem('status', JSON.stringify(status));
      localStorage.setItem('i18nextLng', 'zh-CN');
      localStorage.setItem('theme-mode', dark ? 'dark' : 'light');
      localStorage.setItem(
        `newapi:user-onboarding:regular:v1:user:${user.id}`,
        'done',
      );
    },
    user,
    status,
    dark,
  );
  await page.setRequestInterception(true);
  page.on('request', async (request) => {
    const url = new URL(request.url());
    if (url.origin !== baseURL) return request.abort();
    if (!url.pathname.startsWith('/api/')) return request.continue();
    apiRequests.push(`${request.method()} ${url.pathname}${url.search}`);
    let body = { success: true, data: [] };
    if (url.pathname === '/api/status') body = { success: true, data: status };
    if (url.pathname === '/api/pricing') body = pricing;
    if (url.pathname === '/api/user/self') body = { success: true, data: user };
    if (url.pathname === '/api/health/groups')
      body = { success: true, data: { enabled: true, groups: [] } };
    if (url.pathname === '/api/health/models')
      body = { success: true, data: { enabled: true, models: [] } };
    if (url.pathname === '/api/log/self/' || url.pathname === '/api/log/') {
      body = {
        success: true,
        data: { items: [logRecord], page: 1, page_size: 10, total: 1 },
      };
    }
    if (
      url.pathname === '/api/log/self/stat' ||
      url.pathname === '/api/log/stat'
    ) {
      body = {
        success: true,
        data: {
          quota: 100,
          rpm: 0,
          tpm: 0,
          cache_hit_rate: 0,
          cache_global_rate: 0,
        },
      };
    }
    await request.respond({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(body),
    });
  });
  return { page, errors, apiRequests };
};

const clickButton = async (page, text) => {
  await page.waitForFunction(
    (text) =>
      [...document.querySelectorAll('button')].some(
        (button) => button.textContent.trim() === text && !button.disabled,
      ),
    {},
    text,
  );
  await page.evaluate(
    (text) =>
      [...document.querySelectorAll('button')]
        .find(
          (button) => button.textContent.trim() === text && !button.disabled,
        )
        .click(),
    text,
  );
};

try {
  await launchBrowser();
  for (const dark of [false, true]) {
    const { page, errors } = await makePage(1, dark);
    await page.goto(`${baseURL}/model/same-model`, {
      waitUntil: 'networkidle0',
    });
    await page.waitForSelector('.model-detail-hero h1');
    assert.equal(await page.$('.model-detail-model-id'), null);
    assert.equal(
      await page.evaluate(() =>
        document.body.textContent.includes('暂无模型描述'),
      ),
      false,
    );
    assert.equal(
      await page.evaluate(() => {
        const quickStart = document.querySelector('#quick-start');
        const billing = document.querySelector('#billing-details');
        return Boolean(
          quickStart &&
            billing &&
            quickStart.compareDocumentPosition(billing) &
              Node.DOCUMENT_POSITION_FOLLOWING,
        );
      }),
      true,
    );
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth + 2,
      ),
      true,
    );
    for (const name of ['same-model', 'no-display-name']) {
      await page.goto(`${baseURL}/model/${name}`, {
        waitUntil: 'networkidle0',
      });
      await page.waitForSelector('button[aria-label="复制模型名称"]');
      assert.equal(await page.$('.model-detail-model-id'), null);
      await page.evaluate(() => {
        window.fixtureCopied = '';
        Object.defineProperty(navigator, 'clipboard', {
          configurable: true,
          value: {
            writeText: async (value) => {
              window.fixtureCopied = value;
            },
          },
        });
      });
      await page.click('button[aria-label="复制模型名称"]');
      assert.equal(await page.evaluate(() => window.fixtureCopied), name);
    }
    await page.goto(`${baseURL}/model/friendly-id`, {
      waitUntil: 'networkidle0',
    });
    await page.waitForSelector('.model-detail-model-key');
    assert.equal(
      await page.$eval(
        '.model-detail-model-id',
        (element) => element.textContent,
      ),
      'friendly-id',
    );
    assert.equal(
      (await page.$('button[aria-label="复制模型名称"]')) !== null,
      true,
    );
    assert.deepEqual(errors, []);
    await page.close();
    console.log(`${dark ? 'Mobile/dark' : 'Desktop'} model detail passed`);
  }

  const { page, errors, apiRequests } = await makePage(1);
  await page.goto(`${baseURL}/console/log`, { waitUntil: 'networkidle0' });
  await page.waitForSelector('.usage-logs-table .semi-table-row');
  const summaryHeader = await page.$eval(
    '.usage-logs-table .semi-table-thead',
    (element) => element.textContent,
  );
  assert.match(summaryHeader, /时间/);
  assert.match(summaryHeader, /模型/);
  assert.doesNotMatch(summaryHeader, /令牌/);
  assert.deepEqual(
    await page.evaluate(() =>
      JSON.parse(localStorage.getItem('logs-table-columns-user')),
    ),
    {
      time: true,
      channel: false,
      username: false,
      token: true,
      group: true,
      type: true,
      model: true,
      use_time: true,
      prompt: true,
      completion: true,
      cost: true,
      retry: false,
      ip: true,
      details: true,
    },
  );

  await clickButton(page, '完整视图');
  await page.waitForFunction(() =>
    document
      .querySelector('.usage-logs-table .semi-table-thead')
      ?.textContent.includes('令牌'),
  );
  const requestCount = apiRequests.length;
  await clickButton(page, '摘要视图');
  await clickButton(page, '完整视图');
  assert.equal(
    apiRequests.length,
    requestCount,
    'view switches must not query',
  );
  assert.equal(
    await page.evaluate(() =>
      localStorage.getItem('logs-table-column-mode-user'),
    ),
    'full',
  );

  await clickButton(page, '摘要视图');
  await clickButton(page, '列设置');
  await page.waitForSelector('.semi-modal');
  assert.equal(
    await page.evaluate(() =>
      localStorage.getItem('logs-table-column-mode-user'),
    ),
    'full',
  );
  await clickButton(page, '确定');
  await page.reload({ waitUntil: 'networkidle0' });
  await page.waitForFunction(() =>
    document
      .querySelector('.usage-logs-table .semi-table-thead')
      ?.textContent.includes('令牌'),
  );
  await page.click('.usage-logs-table .semi-table-expand-icon');
  await page.waitForSelector('.usage-logs-detail');
  assert.deepEqual(errors, []);
  await page.close();
  console.log(
    'Ordinary user summary/full persistence and request stability passed',
  );

  const admin = await makePage(10);
  await admin.page.goto(`${baseURL}/console/log`, {
    waitUntil: 'networkidle0',
  });
  await admin.page.waitForSelector('.usage-logs-table .semi-table-row');
  const adminHeader = await admin.page.$eval(
    '.usage-logs-table .semi-table-thead',
    (element) => element.textContent,
  );
  assert.match(adminHeader, /渠道/);
  assert.match(adminHeader, /用户/);
  assert.match(adminHeader, /令牌/);
  assert.equal(
    await admin.page.evaluate(() =>
      [...document.querySelectorAll('button')].some(
        (button) => button.textContent.trim() === '摘要视图',
      ),
    ),
    false,
  );
  assert.deepEqual(admin.errors, []);
  await admin.page.close();
  console.log('Administrator full columns and controls passed');
} finally {
  await browser?.close();
  await server.close();
  stopEsbuild();
}
