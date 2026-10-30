import assert from 'node:assert/strict';
import path from 'node:path';
import fs from 'node:fs';
import os from 'node:os';
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
  group_ratio: { Public: 0.5 },
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
      display_name: 'GPT 6 Astra',
      icon: 'OpenAI',
      description:
        '界面验收数据：管理员可在模型管理中编辑此介绍，完整内容展示在顶部卡片。多档官方参考价与本站实际分组价格独立显示。',
      official_pricing: {
        currency: 'USD',
        source_url: 'https://example.com/pricing',
        tiers: [
          {
            label: 'Input ≤ 272K',
            unit: 'million_tokens',
            input: 10,
            output: 50,
            cache_read: 0,
          },
          {
            label: 'Input > 272K',
            unit: 'million_tokens',
            input: 20,
            output: 75,
            cache_read: 2,
            cache_write: 25,
          },
        ],
      },
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
const artifacts = fs.mkdtempSync(
  path.join(os.tmpdir(), 'model-official-pricing-'),
);
const fixtureName = `.official-pricing-qa-${process.pid}`;
const writes = [];
const fixtureModel = pricing.data[2];
fixtureModel.id = 1;
fixtureModel.name_rule = 0;
fixtureModel.status = 1;
const files = ['html', 'jsx'].map((ext) =>
  path.join(root, `${fixtureName}.${ext}`),
);
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
    if (url.pathname === '/api/models/1')
      body = { success: true, data: fixtureModel };
    if (url.pathname === '/api/models/' && request.method() === 'PUT') {
      const saved = JSON.parse(request.postData());
      writes.push(saved);
      Object.assign(fixtureModel, saved);
      body = { success: true, data: fixtureModel };
    }
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
  fs.writeFileSync(
    files[0],
    `<html><meta name="viewport" content="width=device-width, initial-scale=1"><div id="root"></div><script type="module" src="/${fixtureName}.jsx"></script></html>`,
  );
  fs.writeFileSync(
    files[1],
    `import React from 'react';import {createRoot} from 'react-dom/client';import {BrowserRouter} from 'react-router-dom';import EditModelModal from './src/components/table/models/modals/EditModelModal';import './src/i18n/i18n';import './src/index.css';function App(){const [open,setOpen]=React.useState(true);return <EditModelModal visiable={open} editingModel={{id:1}} handleClose={()=>setOpen(false)} refresh={()=>{}}/>}createRoot(document.getElementById('root')).render(<BrowserRouter><App/></BrowserRouter>);`,
  );
  await launchBrowser();
  for (const dark of [false, true]) {
    const { page, errors } = await makePage(1, dark);
    await page.goto(`${baseURL}/model/friendly-id`, {
      waitUntil: 'networkidle0',
    });
    await page.waitForSelector('#official-pricing table');
    const official = await page.$eval(
      '#official-pricing',
      (el) => el.textContent,
    );
    assert.ok(
      official.includes('$10') &&
        official.includes('$0') &&
        official.includes('$75'),
    );
    assert.equal(
      await page.$eval('#official-pricing a', (el) => el.href),
      'https://example.com/pricing',
    );
    await page.click('.model-detail-price-controls .semi-button');
    assert.equal(
      await page.$eval('#official-pricing', (el) => el.textContent),
      official,
      'site unit conversion cannot affect official prices',
    );
    await page.screenshot({
      path: path.join(artifacts, dark ? 'mobile-dark.png' : 'desktop.png'),
      fullPage: false,
    });
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 2,
      ),
      true,
    );
    await page.setViewport({ width: 320, height: 900 });
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 2,
      ),
      true,
    );
    await page.goto(`${baseURL}/model/no-display-name`, {
      waitUntil: 'networkidle0',
    });
    assert.ok(
      (await page.$eval('#official-pricing', (el) => el.textContent)).includes(
        '暂无官方参考价',
      ),
    );
    assert.deepEqual(errors, []);
    await page.close();
  }
  const { page, errors } = await makePage(10);
  await page.goto(`${baseURL}/${fixtureName}.html`, {
    waitUntil: 'networkidle0',
  });
  await page.waitForSelector('input[aria-label="输入 1"]');
  assert.equal(
    await page.$eval('input[aria-label="输入 1"]', (el) => el.value),
    '10',
  );
  const fill = async (selector, text) => {
    await page.click(selector, { clickCount: 3 });
    await page.keyboard.press('Backspace');
    await page.type(selector, text);
  };
  await fill('input[aria-label="输入 1"]', '12');
  await fill('textarea#description', '后台保存后的完整介绍');
  await clickButton(page, '添加价格档位');
  await fill('input[aria-label="档位 3"]', '额外服务');
  await fill('input[aria-label="价格 3"]', '0.01');
  await clickButton(page, '提交');
  await page.waitForFunction(
    () => !document.querySelector('.semi-sidesheet-body'),
  );
  assert.equal(writes.length, 1);
  assert.equal(writes[0].official_pricing.tiers[0].input, 12);
  assert.equal(writes[0].official_pricing.tiers[0].cache_read, 0);
  assert.equal(writes[0].official_pricing.tiers.length, 3);
  assert.equal(writes[0].description, '后台保存后的完整介绍');
  await page.goto(`${baseURL}/model/friendly-id`, {
    waitUntil: 'networkidle0',
  });
  assert.equal(
    await page.$eval('.model-detail-identity p', (el) => el.textContent),
    '后台保存后的完整介绍',
  );
  assert.ok(
    (await page.$eval('#official-pricing', (el) => el.textContent)).includes(
      '$12',
    ),
  );
  await page.goto(`${baseURL}/${fixtureName}.html`, {
    waitUntil: 'networkidle0',
  });
  await page.waitForSelector('input[aria-label="输入 1"]');
  for (let i = 0; i < 3; i++) await clickButton(page, '删除');
  await clickButton(page, '提交');
  await page.waitForFunction(
    () => !document.querySelector('.semi-sidesheet-body'),
  );
  assert.equal(writes.at(-1).official_pricing.tiers.length, 0);
  await page.goto(`${baseURL}/model/friendly-id`, {
    waitUntil: 'networkidle0',
  });
  assert.ok(
    (await page.$eval('#official-pricing', (el) => el.textContent)).includes(
      '暂无官方参考价',
    ),
  );
  assert.deepEqual(errors, []);
  await page.close();
  console.log(
    'Official pricing browser checks passed; screenshots: ' + artifacts,
  );
} finally {
  await browser?.close();
  await server.close();
  stopEsbuild();
  files.forEach((file) => fs.rmSync(file, { force: true }));
}
