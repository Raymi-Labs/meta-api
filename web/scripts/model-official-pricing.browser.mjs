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
  usd_exchange_rate: 1,
  price: 2,
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
      official_pricing: {
        currency: 'USD',
        tiers: [
          { label: '标准', unit: 'million_tokens', input: 20, output: 40 },
        ],
      },
    },
    { ...modelDefaults, model_name: 'no-display-name' },
    {
      ...modelDefaults,
      model_name: 'friendly-id',
      display_name: 'GPT 6 Astra',
      faq: [
        {
          question: '如何选择分组？',
          answer: '先比较分组价格与用量。\n再查看快速开始中的接入示例。',
        },
        {
          question: '答案可以包含什么内容？',
          answer:
            '<img src=x onerror=alert(1)> 仅作为文字显示。' +
            '长内容'.repeat(100),
        },
      ],
      context_length: 1050000,
      max_output_tokens: 128000,
      input_modalities: ['text', 'image'],
      output_modalities: ['text'],
      capabilities: {
        reasoning: true,
        streaming: true,
        function_calling: true,
        web_search: true,
        code_interpreter: true,
        computer_use: true,
        file_search: true,
        structured_output: true,
        memory: false,
      },
      attribute_sources: {
        context_length: {
          kind: 'official',
          url: 'https://developers.openai.com/api/docs/models/gpt-6-astra',
        },
        max_output_tokens: {
          kind: 'community',
          url: 'https://basellm.github.io/llm-metadata',
        },
        'capabilities.reasoning': {
          kind: 'official',
          url: 'https://developers.openai.com/api/docs/models/gpt-6-astra',
        },
      },
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
let rejectNextSave = false;
let comparisonFixtures = false;
const artifacts = fs.mkdtempSync(
  path.join(os.tmpdir(), 'model-official-pricing-'),
);
const fixtureName = `.official-pricing-qa-${process.pid}`;
const writes = [];
const fixtureModel = pricing.data[2];
fixtureModel.id = 1;
fixtureModel.supports_cache_read = true;
fixtureModel.cache_ratio = 0.1;
fixtureModel.enable_groups = [
  'Public',
  'Equal',
  'Higher',
  'Free',
  'Extra1',
  'Extra2',
  'Extra3',
];
Object.assign(
  pricing.usable_group,
  Object.fromEntries(fixtureModel.enable_groups.map((name) => [name, name])),
);
Object.assign(pricing.group_ratio, {
  Equal: 5,
  Higher: 10,
  Free: 0,
  Extra1: 1,
  Extra2: 1,
  Extra3: 1,
});
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
      Object.defineProperty(navigator, 'clipboard', {
        configurable: true,
        value: {
          writeText: async (text) => {
            window.copiedText = text;
          },
        },
      });
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
    if (url.pathname.startsWith('/pg/')) {
      apiRequests.push(request.method() + ' ' + url.pathname);
      return request.respond({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          choices: [
            {
              message: { role: 'assistant', content: '工具回合完成' },
              finish_reason: 'stop',
            },
          ],
        }),
      });
    }
    if (!url.pathname.startsWith('/api/')) return request.continue();
    apiRequests.push(`${request.method()} ${url.pathname}${url.search}`);
    let body = { success: true, data: [] };
    if (url.pathname === '/api/models/1')
      body = { success: true, data: fixtureModel };
    if (url.pathname === '/api/models/' && request.method() === 'PUT') {
      const saved = JSON.parse(request.postData());
      if (rejectNextSave) {
        rejectNextSave = false;
        body = { success: false, message: 'FAQ 保存失败（验收）' };
      } else {
        writes.push(saved);
        const { capability_overrides, ...savedFields } = saved;
        Object.assign(fixtureModel, savedFields);
        for (const [key, value] of Object.entries(
          saved.capability_overrides || {},
        )) {
          if (value === null) delete fixtureModel.capabilities[key];
          else fixtureModel.capabilities[key] = value;
        }
        body = { success: true, data: fixtureModel };
      }
    }
    if (url.pathname === '/api/status') body = { success: true, data: status };
    if (url.pathname === '/api/pricing') body = pricing;
    if (url.pathname === '/api/pricing/exchange-rate')
      body = {
        success: true,
        data: {
          usd_cny: 7,
          date: new Date().toISOString().slice(0, 10),
          source: 'ECB',
          stale: false,
        },
      };
    if (url.pathname === '/api/user/models')
      body.data = pricing.data
        .filter((model) =>
          model.enable_groups.includes(
            url.searchParams.get('group') || 'Public',
          ),
        )
        .map((model) => model.model_name);
    if (url.pathname === '/api/user/self/groups')
      body.data = Object.fromEntries(
        Object.keys(pricing.usable_group).map((group) => [
          group,
          { ratio: pricing.group_ratio[group], desc: group },
        ]),
      );
    if (url.pathname === '/api/user/self') body = { success: true, data: user };
    if (url.pathname === '/api/health/groups')
      body = { success: true, data: { enabled: true, groups: [] } };
    if (url.pathname === '/api/health/groups' && comparisonFixtures) {
      const stats = {
        group: url.searchParams.get('group') || 'Public',
        observed_count: 100,
        request_count: 100,
        success_rate: 98,
        total_tokens: 123456,
        avg_ttft_ms:
          url.searchParams.get('model') === 'same-model' ? 2500 : 1234,
        avg_output_tps: 30,
      };
      body = {
        success: true,
        data: {
          enabled: true,
          groups: [
            {
              ...stats,
              series: [
                { ...stats, ts: 1793318400 },
                { ...stats, ts: 1793322000 },
              ],
            },
          ],
        },
      };
    }
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

const expectActiveSection = async (page, id) => {
  await page.waitForFunction(
    (id) => {
      const active = document.querySelectorAll(
        '.model-detail-sections button[aria-current="location"]',
      );
      return (
        active.length === 1 && active[0].getAttribute('aria-controls') === id
      );
    },
    {},
    id,
  );
  const weights = await page.$$eval(
    '.model-detail-sections button',
    (buttons) =>
      buttons.map((button) => ({
        id: button.getAttribute('aria-controls'),
        active: button.getAttribute('aria-current'),
        weight: getComputedStyle(button).fontWeight,
      })),
  );
  assert.ok(
    weights.every(
      ({ active, weight }) =>
        weight === (active === 'location' ? '700' : '400'),
    ),
    JSON.stringify({ expected: id, url: page.url(), weights }),
  );
};

const selectOption = async (page, label, text) => {
  const selector = {
    官方对比档位: '.model-group-comparison-controls .semi-select',
    价格模式: '.model-detail-settings .semi-select',
    分组: '.model-detail-price-controls .semi-select',
    对比分组: '[role="combobox"][aria-labelledby="compare-group-label"]',
  }[label];
  await page.click(selector);
  await page.waitForFunction(
    (text) =>
      [...document.querySelectorAll('.semi-select-option')].some(
        (el) => el.textContent.trim() === text,
      ),
    {},
    text,
  );
  await page.evaluate(
    (text) =>
      [...document.querySelectorAll('.semi-select-option')]
        .find((el) => el.textContent.trim() === text)
        .click(),
    text,
  );
  await page.waitForFunction(
    (selector, text) =>
      document.querySelector(selector)?.textContent.includes(text),
    {},
    selector,
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
    `import React from 'react';import {createRoot} from 'react-dom/client';import {BrowserRouter} from 'react-router-dom';import EditModelModal from './src/components/table/models/modals/EditModelModal';import {ThemeProvider} from './src/context/Theme';import './src/i18n/i18n';import './src/index.css';function App(){const [open,setOpen]=React.useState(true);return <EditModelModal visiable={open} editingModel={{id:1}} handleClose={()=>setOpen(false)} refresh={()=>{}}/>}createRoot(document.getElementById('root')).render(<BrowserRouter><ThemeProvider><App/></ThemeProvider></BrowserRouter>);`,
  );
  await launchBrowser();
  for (const dark of [false, true]) {
    const { page, errors } = await makePage(1, dark);
    await page.goto(`${baseURL}/model/friendly-id`, {
      waitUntil: 'networkidle0',
    });
    await page.waitForSelector('#official-pricing table');
    await page.waitForSelector('#faq details');
    await expectActiveSection(page, 'official-pricing');
    assert.equal(
      await page.$eval('.model-sticky-bar', (el) => el.hidden),
      true,
    );
    const historyBeforeScroll = await page.evaluate(() => history.length);
    const urlBeforeScroll = page.url();
    await clickButton(page, '模型属性');
    await expectActiveSection(page, 'capabilities');
    await page.focus('.model-detail-sections button[aria-controls="pricing"]');
    await page.keyboard.press('Enter');
    await expectActiveSection(page, 'pricing');
    await page.waitForFunction(
      () => !document.querySelector('.model-sticky-bar').hidden,
    );
    assert.ok(
      await page.$eval('.model-sticky-identity', (el) =>
        el.textContent.includes('GPT 6 Astra'),
      ),
    );
    assert.equal(
      await page.$eval('.model-sticky-actions a:first-child', (el) =>
        new URL(el.href).searchParams.get('model'),
      ),
      'friendly-id',
    );
    assert.equal(
      await page.$eval('.model-sticky-actions a:last-child', (el) =>
        new URL(el.href).searchParams.get('model'),
      ),
      'friendly-id',
    );
    assert.equal(
      await page.evaluate(
        () =>
          document.querySelector('#pricing').getBoundingClientRect().top >=
          document.querySelector('.model-sticky-bar').getBoundingClientRect()
            .bottom -
            1,
      ),
      true,
    );
    await page.mouse.move(dark ? 200 : 1000, 400);
    await page.mouse.wheel({ deltaY: -300 });
    await expectActiveSection(page, 'capabilities');
    await page.mouse.wheel({ deltaY: 300 });
    await expectActiveSection(page, 'pricing');
    assert.equal(
      await page.evaluate(
        (dark) =>
          dark
            ? window.scrollY > 0
            : document.querySelector('.app-workspace').scrollTop > 0,
        dark,
      ),
      true,
      'exercise window scrolling on mobile and workspace scrolling on desktop',
    );
    await page.screenshot({
      path: path.join(
        artifacts,
        dark ? 'active-section-mobile.png' : 'active-section-desktop.png',
      ),
    });
    assert.equal(
      await page.$eval(
        '#faq',
        (el) => el.parentElement.lastElementChild === el,
      ),
      true,
    );
    assert.equal(await page.$eval('#faq details', (el) => el.open), false);
    await clickButton(page, '常见问题');
    await expectActiveSection(page, 'faq');
    assert.equal(page.url(), urlBeforeScroll);
    assert.equal(
      await page.evaluate(() => history.length),
      historyBeforeScroll,
    );
    await page.focus('#faq summary');
    await page.keyboard.press('Enter');
    assert.equal(await page.$eval('#faq details', (el) => el.open), true);
    assert.ok(
      (await page.$eval('#faq details p', (el) => el.innerText)).includes('\n'),
    );
    await page.click('#faq details:nth-child(2) summary');
    await page.$eval('#faq', (el) => el.scrollIntoView({ block: 'start' }));
    await expectActiveSection(page, 'faq');
    assert.equal(
      await page.$('#faq img'),
      null,
      'FAQ answers are rendered as text',
    );
    await page.screenshot({
      path: path.join(
        artifacts,
        dark ? 'faq-mobile-dark.png' : 'faq-desktop.png',
      ),
    });
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 2,
      ),
      true,
    );
    const attributes = await page.$eval(
      '#capabilities',
      (el) => el.textContent,
    );
    assert.ok(
      attributes.includes('1.05M tokens') && attributes.includes('128K tokens'),
    );
    assert.ok(
      !attributes.includes('厂商官方') &&
        !attributes.includes('模型库补全') &&
        !attributes.includes('人工配置'),
    );
    assert.equal(
      await page.$eval('[data-capability="memory"]', (el) => el.dataset.status),
      '不支持',
    );
    assert.equal(
      await page.$eval(
        '[data-capability="background"]',
        (el) => el.dataset.status,
      ),
      '未知',
    );
    assert.equal(
      await page.$eval(
        '[data-capability="reasoning"]',
        (el) => el.dataset.status,
      ),
      '支持',
    );
    assert.equal(
      await page.evaluate(() => {
        const a = document.querySelector('#capabilities'),
          p = document.querySelector('#pricing'),
          o = document.querySelector('#official-pricing');
        return (
          !p.contains(a) &&
          a.getBoundingClientRect().top >= o.getBoundingClientRect().bottom &&
          p.getBoundingClientRect().top >= a.getBoundingClientRect().bottom
        );
      }),
      true,
    );
    if (!dark)
      assert.equal(
        await page.$$eval(
          '.model-attribute-column',
          (els) =>
            new Set(els.map((el) => Math.round(el.getBoundingClientRect().top)))
              .size,
        ),
        1,
      );
    await page.$eval('#capabilities', (el) =>
      el.scrollIntoView({ block: 'start' }),
    );
    await expectActiveSection(page, 'capabilities');
    await page.screenshot({
      path: path.join(
        artifacts,
        dark ? 'attributes-mobile-dark.png' : 'attributes-desktop.png',
      ),
    });

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
    await page.waitForSelector('.model-group-discount-lower');
    assert.equal(
      await page.$eval(
        '.model-group-row [data-price-key="input"] del',
        (el) => el.textContent,
      ),
      '¥70.00',
    );
    assert.equal(
      await page.$eval(
        '.model-group-row [data-price-key="input"] .model-group-discount',
        (el) => el.textContent,
      ),
      '较官方低 97.1%',
    );
    assert.equal(
      await page.$('.model-group-row [data-price-key="cache"] del'),
      null,
    );
    assert.equal(
      await page.$eval(
        '.model-group-row [data-price-key="cache"] .model-group-discount',
        (el) => el.textContent,
      ),
      '高于官方',
    );
    assert.equal(
      await page.evaluate(
        () =>
          document
            .querySelector('#pricing')
            .compareDocumentPosition(document.querySelector('#quick-start')) &
          Node.DOCUMENT_POSITION_FOLLOWING,
      ),
      4,
    );
    await page.click('.model-detail-price-controls .semi-button');
    assert.equal(
      await page.$eval('#official-pricing', (el) => el.textContent),
      official,
      'site unit conversion cannot affect official prices',
    );
    assert.equal(
      await page.$eval(
        '.model-group-row [data-price-key="input"] .model-group-discount',
        (el) => el.textContent,
      ),
      '较官方低 97.1%',
    );
    await selectOption(page, '官方对比档位', 'Input > 272K');
    await page.waitForFunction(
      () =>
        document.querySelector(
          '.model-group-row [data-price-key="input"] .model-group-discount',
        )?.textContent === '较官方低 98.6%',
    );
    await selectOption(page, '官方对比档位', 'Input ≤ 272K');
    await clickButton(page, '显示设置');
    await selectOption(page, '价格模式', 'USD ($)');
    await page.waitForFunction(
      () =>
        document.querySelector('.model-group-row [data-price-key="input"] del')
          ?.textContent === '$0.01',
    );
    assert.equal(
      await page.$eval(
        '.model-group-row [data-price-key="input"] .model-group-discount',
        (el) => el.textContent,
      ),
      '较官方低 90%',
    );
    assert.ok(
      (await page.$eval('.model-group-list', (el) => el.textContent)).includes(
        '与官方持平',
      ),
    );
    assert.ok(
      (await page.$eval('.model-group-list', (el) => el.textContent)).includes(
        '较官方高 100%',
      ),
    );
    assert.ok(
      (await page.$eval('.model-group-list', (el) => el.textContent)).includes(
        '较官方低 100%',
      ),
    );
    await clickButton(page, '分组价格');
    await expectActiveSection(page, 'pricing');
    await page.type('input[aria-label="搜索分组"]', 'Extra3');
    await expectActiveSection(page, 'pricing');
    assert.equal((await page.$$('.model-group-row')).length, 1);
    assert.ok(
      (
        await page.$eval('.model-group-row header', (el) => el.textContent)
      ).includes('Extra3'),
    );
    await page.click('input[aria-label="搜索分组"]', { clickCount: 3 });
    await page.keyboard.press('Backspace');
    assert.equal((await page.$$('.model-group-row')).length, 6);
    await selectOption(page, '分组', 'Higher');
    assert.equal((await page.$$('.model-group-row')).length, 1);
    assert.equal(
      await page.$('.model-group-row [data-price-key="input"] del'),
      null,
    );
    await selectOption(page, '分组', '全部分组');
    await clickButton(page, '显示设置');
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
    await page.click('.model-detail-related a[href="/model/same-model"]');
    await page.waitForSelector(
      '[data-price-key="tiered-p"] .model-group-discount',
    );
    await expectActiveSection(page, 'official-pricing');
    assert.equal(
      await page.$eval(
        '[data-price-key="tiered-p"] .model-group-discount',
        (el) => el.textContent,
      ),
      '较官方低 92.9%',
    );
    assert.ok(
      !(await page.$eval('.model-group-row', (el) => el.textContent)).includes(
        '本站首档：base',
      ),
    );
    await page.click('.model-detail-related a[href="/model/no-display-name"]');
    await page.waitForFunction(
      () =>
        location.pathname === '/model/no-display-name' &&
        document.querySelector('.model-detail-hero h1')?.textContent ===
          'no-display-name' &&
        document
          .querySelector('#official-pricing')
          ?.textContent.includes('暂无官方参考价'),
    );
    await expectActiveSection(page, 'official-pricing');
    assert.ok(
      (await page.$eval('#official-pricing', (el) => el.textContent)).includes(
        '暂无官方参考价',
      ),
    );
    assert.equal(await page.$('#faq'), null);
    assert.equal(
      await page.$$eval('.model-detail-sections button', (els) =>
        els.some((el) => el.textContent === '常见问题'),
      ),
      false,
    );
    assert.deepEqual(errors, []);
    await page.close();
  }
  comparisonFixtures = true;
  const encodedName = 'vendor/a+b%2F';
  pricing.data.push({ ...modelDefaults, model_name: encodedName });
  for (const dark of [false, true]) {
    const { page, errors, apiRequests } = await makePage(1, dark);
    await page.goto(`${baseURL}/model/friendly-id`, {
      waitUntil: 'networkidle0',
    });
    await clickButton(page, '分组价格');
    await page.waitForFunction(
      () => !document.querySelector('.model-sticky-bar')?.hidden,
    );
    // Exercise keyboard activation of the sticky controls after navigation;
    // avoid pointer coordinates changing across the scroll boundary.
    await page.focus('.model-sticky-copy');
    await page.keyboard.press('Enter');
    await page.waitForFunction(() => window.copiedText === 'friendly-id');
    assert.equal(await page.evaluate(() => window.copiedText), 'friendly-id');
    await page.evaluate(() => {
      window.scrollTo(0, 0);
      document.querySelector('.app-workspace').scrollTo(0, 0);
    });
    await page.waitForFunction(
      () => document.querySelector('.model-sticky-bar').hidden,
    );
    await clickButton(page, '分组价格');
    await page.waitForFunction(
      () => !document.querySelector('.model-sticky-bar').hidden,
    );
    await page.focus('.model-sticky-actions a:first-child');
    await page.keyboard.press('Enter');
    await page.waitForSelector('.model-compare-card');
    assert.equal(new URL(page.url()).searchParams.get('model'), 'friendly-id');
    await page.click('.model-compare-chips .semi-select');
    await page.waitForSelector('.semi-select-option-list');
    await page.evaluate(() =>
      [...document.querySelectorAll('.semi-select-option')]
        .find((el) => el.textContent.trim() === 'same-model')
        ?.click(),
    );
    await page.waitForFunction(
      () => document.querySelectorAll('.model-compare-card').length === 2,
    );
    assert.equal(new URL(page.url()).searchParams.getAll('model').length, 2);
    await page.waitForFunction(() =>
      document
        .querySelector('.model-compare-table')
        .textContent.includes('1.23 s'),
    );
    assert.ok(
      (
        await page.$eval('.model-compare-table', (el) => el.textContent)
      ).includes('2.50 s'),
    );
    const health72 = page.waitForResponse((response) => {
      const url = new URL(response.url());
      return (
        url.pathname === '/api/health/groups' &&
        url.searchParams.get('hours') === '72' &&
        url.searchParams.get('model') === 'same-model'
      );
    });
    await page.click(
      '[role="combobox"][aria-labelledby="compare-hours-label"]',
    );
    await page.waitForSelector('.semi-select-option-list');
    await page.evaluate(() =>
      [...document.querySelectorAll('.semi-select-option')]
        .find((el) => el.textContent.includes('72'))
        ?.click(),
    );
    await page.waitForFunction(() =>
      document
        .querySelector('.model-compare-table')
        .textContent.includes('2.50 s'),
    );
    await health72;
    assert.ok(
      apiRequests.some(
        (url) =>
          url.includes('/api/health/groups?') &&
          url.includes('hours=72') &&
          url.includes('model=same-model'),
      ),
    );
    await selectOption(page, '对比分组', 'Equal');
    await page.waitForFunction(() =>
      document
        .querySelectorAll('.model-compare-card')[1]
        .textContent.includes('当前分组不可用'),
    );
    assert.ok(
      !(
        await page.$$eval('.model-compare-card', (els) => els[1].textContent)
      ).includes('$5'),
    );
    await selectOption(page, '对比分组', 'Public');
    await page.waitForFunction(
      () =>
        !document
          .querySelectorAll('.model-compare-card')[1]
          .textContent.includes('当前分组不可用'),
    );

    assert.ok(
      await page.$eval('.model-compare-table', (el) =>
        el.textContent.includes('1.05M tokens'),
      ),
    );
    assert.ok(
      await page.$eval('.model-compare-table', (el) =>
        el.textContent.includes('不支持'),
      ),
    );
    assert.ok(
      await page.$eval('.model-cost-results', (el) =>
        el.textContent.includes('暂无可用估算'),
      ),
    );
    await page.$eval('input[aria-label="每日请求数"]', (el) =>
      el.scrollIntoView(),
    );
    await page.click('input[aria-label="每日请求数"]', { clickCount: 3 });
    await page.keyboard.type('0');
    assert.ok(
      await page.$eval('.model-cost-results article', (el) =>
        el.textContent.includes('$0.00'),
      ),
    );
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 2,
      ),
      true,
    );
    await page.evaluate(() => {
      window.scrollTo(0, 0);
      document.querySelector('.app-workspace').scrollTo(0, 0);
    });
    await page.screenshot({
      path: path.join(
        artifacts,
        dark ? 'comparison-mobile-dark.png' : 'comparison-desktop.png',
      ),
    });
    const sharedURL = page.url();
    await page.reload({ waitUntil: 'networkidle0' });
    await page.waitForFunction(
      () => document.querySelectorAll('.model-compare-card').length === 2,
    );
    assert.equal(page.url(), sharedURL);
    await page.click(
      '.model-compare-chip button[aria-label="移除 same-model"]',
    );
    await page.waitForFunction(
      () => document.querySelectorAll('.model-compare-card').length === 1,
    );
    await page.click('.model-compare-card-actions a:last-child');
    await page.waitForSelector('.pg-model-tab [aria-selected="true"]');
    await page.waitForFunction(
      () =>
        document
          .querySelector('.pg-model-tab [aria-selected="true"]')
          ?.textContent.trim() === 'friendly-id',
    );
    assert.equal(new URL(page.url()).pathname, '/playground');
    assert.equal(
      apiRequests.some((request) => request.includes('/pg/')),
      false,
    );
    await page.goto(`${baseURL}/playground?model=not-allowed&group=Public`, {
      waitUntil: 'networkidle0',
    });
    await page.waitForFunction(() =>
      document.body.innerText.includes('目标模型在当前分组不可用'),
    );
    assert.equal(
      apiRequests.some((request) => request.includes('/pg/')),
      false,
    );
    await page.goto(
      `${baseURL}/playground?model=friendly-id&group=not-allowed`,
      { waitUntil: 'networkidle0' },
    );
    await page.waitForFunction(() =>
      document.body.innerText.includes('目标分组不可用'),
    );
    assert.equal(
      apiRequests.some((request) => request.includes('/pg/')),
      false,
    );
    const compareURL = new URL('/compare', baseURL);
    for (const name of [
      'friendly-id',
      'same-model',
      'no-display-name',
      encodedName,
    ])
      compareURL.searchParams.append('model', name);
    await page.goto(compareURL.href, { waitUntil: 'networkidle0' });
    assert.equal(
      await page.$$eval('.model-compare-card', (els) => els.length),
      4,
    );
    assert.equal(
      await page.$eval('[aria-labelledby="compare-add-label"]', (el) =>
        el.getAttribute('aria-disabled'),
      ),
      'true',
    );
    assert.ok(
      (
        await page.$eval('.model-compare-cards', (el) => el.textContent)
      ).includes(encodedName),
    );
    await page.goto(`${baseURL}/compare?model=not-visible`, {
      waitUntil: 'networkidle0',
    });
    assert.equal(await page.$('.model-compare-card'), null);
    assert.ok(
      (
        await page.$eval('.model-compare-warning', (el) => el.textContent)
      ).includes('该模型不存在或当前不可见'),
    );
    if (!dark) {
      await page.evaluate(() => {
        localStorage.setItem(
          'playground_config:user:7',
          JSON.stringify({
            inputs: {
              model: 'same-model',
              group: 'Public',
              stream: false,
              modelTabs: ['same-model'],
            },
          }),
        );
        localStorage.setItem(
          'playground_messages:user:7',
          JSON.stringify({
            messages: [
              {
                id: 'q',
                role: 'user',
                content: '保留的原对话',
                status: 'complete',
              },
              {
                id: 'a',
                role: 'assistant',
                content: '',
                status: 'complete',
                tool_calls: [
                  {
                    id: 'call_one',
                    type: 'function',
                    function: { name: 'test_tool', arguments: '{}' },
                  },
                ],
              },
            ],
          }),
        );
      });
      await page.goto(`${baseURL}/playground?model=friendly-id&group=Public`, {
        waitUntil: 'networkidle0',
      });
      await page.waitForFunction(() =>
        document.body.innerText.includes('完成当前请求或工具回合后'),
      );
      assert.equal(
        await page.$eval('.pg-model-tab [aria-selected="true"]', (el) =>
          el.textContent.trim(),
        ),
        'same-model',
      );
      assert.ok(
        (await page.$eval('body', (el) => el.innerText)).includes(
          '保留的原对话',
        ),
      );
      await page.type('.pg-tool-results textarea', '测试结果');
      assert.equal(
        await page.$eval('.pg-tool-results button', (el) => el.disabled),
        false,
      );
      await page.click('.pg-tool-results button');
      await page.waitForFunction(
        () =>
          document
            .querySelector('.pg-model-tab [aria-selected="true"]')
            ?.textContent.trim() === 'friendly-id',
      );
      assert.ok(
        (await page.$eval('body', (el) => el.innerText)).includes(
          '保留的原对话',
        ),
      );
      assert.equal(
        apiRequests.filter((request) => request.includes('/pg/')).length,
        1,
      );
    }
    assert.deepEqual(errors, []);
    await page.close();
  }
  comparisonFixtures = false;
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
    await page.$eval(selector, (el) => {
      el.focus();
      el.select();
    });
    await page.keyboard.press('Backspace');
    await page.type(selector, text);
  };
  await fill('input#model-faq-question-0', '更新后的问题');
  await fill('textarea#model-faq-answer-0', '更新后的答案\n第二行');
  await clickButton(page, '添加常见问题');
  await clickButton(page, '提交');
  await page.waitForFunction(() =>
    document.body.textContent.includes('FAQ 的问题和答案不能为空'),
  );
  assert.equal(writes.length, 0, 'empty FAQ cannot be saved');
  await fill('input#model-faq-question-2', '新增的问题');
  await fill('textarea#model-faq-answer-2', '新增的答案');
  await page.click('button[aria-label="上移 FAQ 3"]');
  await page.click('button[aria-label="下移 FAQ 1"]');
  assert.equal(
    await page.$eval('input#model-faq-question-0', (el) => el.value),
    '新增的问题',
  );
  await page.click('button[aria-label="删除 FAQ 3"]');
  await fill('input[aria-label="输入 1"]', '12');
  await fill('textarea#description', '后台保存后的完整介绍');
  await fill('input#max_output_tokens', '64000');
  await page.click('[aria-labelledby="capability-label-streaming"]');
  await page.waitForSelector('.semi-select-option');
  await page.evaluate(() =>
    [...document.querySelectorAll('.semi-select-option')]
      .find((el) => el.textContent.trim() === '不支持')
      .click(),
  );

  await page.waitForFunction(() =>
    document
      .querySelector('[aria-labelledby="capability-label-streaming"]')
      .textContent.includes('不支持'),
  );
  await clickButton(page, '添加价格档位');
  await fill('input[aria-label="档位 3"]', '额外服务');
  await fill('input[aria-label="价格 3"]', '0.01');
  rejectNextSave = true;
  await clickButton(page, '提交');
  await page.waitForFunction(() =>
    document.body.textContent.includes('FAQ 保存失败（验收）'),
  );
  assert.equal(writes.length, 0);
  assert.equal(
    await page.$eval('input#model-faq-question-0', (el) => el.value),
    '新增的问题',
  );
  await page.screenshot({ path: path.join(artifacts, 'faq-editor.png') });
  await clickButton(page, '提交');
  await page.waitForFunction(
    () => !document.querySelector('.semi-sidesheet-body'),
  );
  assert.equal(writes.length, 1);
  assert.equal(writes[0].official_pricing.tiers[0].input, 12);
  assert.equal(writes[0].official_pricing.tiers[0].cache_read, 0);
  assert.equal(writes[0].official_pricing.tiers.length, 3);
  assert.deepEqual(writes[0].faq, [
    { question: '新增的问题', answer: '新增的答案' },
    { question: '更新后的问题', answer: '更新后的答案\n第二行' },
  ]);
  assert.equal(writes[0].description, '后台保存后的完整介绍');
  assert.equal(writes[0].max_output_tokens, 64000);
  assert.equal(
    'context_length' in writes[0],
    false,
    'unchanged automatic limits must not become manual overrides',
  );
  assert.equal(
    'vendor_id' in writes[0],
    false,
    'unchanged vendor keeps automatic ownership',
  );
  assert.equal(writes[0].capability_overrides.streaming, false);
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
  assert.deepEqual(
    await page.$$eval('#faq summary span', (els) =>
      els.map((el) => el.textContent),
    ),
    ['新增的问题', '更新后的问题'],
  );
  const { page: mobileEditor, errors: mobileErrors } = await makePage(10, true);
  await mobileEditor.goto(`${baseURL}/${fixtureName}.html`, {
    waitUntil: 'networkidle0',
  });
  await mobileEditor.waitForSelector('input#model-faq-question-0');
  await mobileEditor.waitForFunction(
    () => document.body.getAttribute('theme-mode') === 'dark',
  );
  await mobileEditor.$eval('.model-faq-editor', (el) =>
    el.scrollIntoView({ block: 'start' }),
  );
  assert.equal(
    await mobileEditor.$eval('input#model-faq-question-0', (el) => el.value),
    '新增的问题',
  );
  await mobileEditor.screenshot({
    path: path.join(artifacts, 'faq-editor-mobile-dark.png'),
  });
  await mobileEditor.setViewport({ width: 320, height: 900 });
  assert.equal(
    await mobileEditor.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 2,
    ),
    true,
  );
  assert.deepEqual(mobileErrors, []);
  await mobileEditor.close();
  await page.goto(`${baseURL}/${fixtureName}.html`, {
    waitUntil: 'networkidle0',
  });
  await page.waitForSelector('input[aria-label="输入 1"]');
  assert.equal(
    await page.$eval('input#model-faq-question-0', (el) => el.value),
    '新增的问题',
  );
  await page.click('button[aria-label="删除 FAQ 2"]');
  await page.click('button[aria-label="删除 FAQ 1"]');
  await fill('input#max_output_tokens', '0');
  await page.click('[aria-labelledby="capability-label-streaming"]');
  await page.waitForSelector('.semi-select-option');
  await page.evaluate(() =>
    [...document.querySelectorAll('.semi-select-option')]
      .find((el) => el.textContent.trim() === '未知')
      .click(),
  );
  await page.waitForFunction(() =>
    document
      .querySelector('[aria-labelledby="capability-label-streaming"]')
      .textContent.includes('未知'),
  );
  for (let i = 0; i < 3; i++) await clickButton(page, '删除');
  await clickButton(page, '提交');
  await page.waitForFunction(
    () => !document.querySelector('.semi-sidesheet-body'),
  );
  assert.equal(writes.at(-1).official_pricing.tiers.length, 0);
  assert.deepEqual(writes.at(-1).faq, []);
  assert.equal(writes.at(-1).max_output_tokens, 0);
  assert.equal(writes.at(-1).capability_overrides.streaming, null);
  await page.goto(`${baseURL}/model/friendly-id`, {
    waitUntil: 'networkidle0',
  });
  assert.ok(
    (await page.$eval('#official-pricing', (el) => el.textContent)).includes(
      '暂无官方参考价',
    ),
  );
  assert.equal(await page.$('#faq'), null);
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
