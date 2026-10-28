import assert from 'node:assert/strict';
import puppeteer from 'puppeteer-core';
import fs from 'node:fs/promises';

const baseURL = process.env.PRICING_TEST_URL || 'http://127.0.0.1:5198';
const screenshotDir = process.env.PROTOCOL_SCREENSHOT_DIR;
const user = { id: 7, role: 1, username: 'protocol-fixture', group: 'default' };
const status = {
  system_name: 'New API',
  quota_per_unit: 500000,
  quota_display_type: 'USD',
  server_address: baseURL,
  HeaderNavModules: JSON.stringify({
    pricing: { enabled: true, requireAuth: false },
  }),
};
const entries = [
  ['gpt-chat-fixture', ['openai']],
  ['responses-fixture', ['openai', 'openai-response']],
  ['image-fixture', ['image-generation', 'openai']],
  ['video-fixture', ['openai-video', 'openai']],
  ['unknown-fixture', []],
];
const pricing = {
  success: true,
  vendors: [{ id: 1, name: 'OpenAI' }],
  usable_group: { Public: 'Allowed' },
  group_ratio: { Public: 1 },
  supported_endpoint: {
    openai: { path: '/v1/chat/completions', method: 'POST' },
    'openai-response': { path: '/v1/responses', method: 'POST' },
  },
  data: entries.map(([model_name, supported_endpoint_types]) => ({
    model_name,
    supported_endpoint_types,
    vendor_id: 1,
    enable_groups: ['Public'],
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 2,
  })),
};
const browser = await puppeteer.launch({
  executablePath:
    process.env.CHROME_PATH ||
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  headless: true,
});
try {
  if (screenshotDir) await fs.mkdir(screenshotDir, { recursive: true });
  for (const mobile of [false, true]) {
    const page = await browser.newPage();
    const errors = [],
      mutations = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await page.setViewport({ width: mobile ? 390 : 1440, height: 1000 });
    await page.evaluateOnNewDocument(
      (user, status, mobile) => {
        localStorage.setItem('user', JSON.stringify(user));
        localStorage.setItem('status', JSON.stringify(status));
        localStorage.setItem('i18nextLng', 'zh-CN');
        localStorage.setItem('theme-mode', mobile ? 'dark' : 'light');
        Object.defineProperty(navigator, 'clipboard', {
          value: {
            writeText: async (text) => {
              window.__protocolCopy = text;
            },
          },
        });
      },
      user,
      status,
      mobile,
    );
    await page.setRequestInterception(true);
    page.on('request', async (req) => {
      const url = new URL(req.url());
      if (url.origin !== baseURL) return req.abort();
      if (!url.pathname.startsWith('/api/')) return req.continue();
      if (req.method() !== 'GET') mutations.push(url.pathname);
      let body = { success: true, data: [] };
      if (url.pathname === '/api/status')
        body = { success: true, data: status };
      if (url.pathname === '/api/pricing') body = pricing;
      if (url.pathname === '/api/user/self')
        body = { success: true, data: user };
      if (url.pathname === '/api/health/groups')
        body = { success: true, data: { enabled: true, groups: [] } };
      if (url.pathname === '/api/health/models')
        body = { success: true, data: { enabled: true, models: [] } };
      await req.respond({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(body),
      });
    });
    await page.goto(baseURL + '/pricing');
    await page.waitForSelector(
      'article[aria-label="gpt-chat-fixture"] .model-protocol-template',
    );
    const card = await page.$eval(
      'article[aria-label="gpt-chat-fixture"]',
      (el) => el.innerText,
    );
    assert.match(card, /已声明协议.*Chat Completions/s);
    assert.match(card, /Responses · 示例 · 未声明/);
    for (const name of [
      'responses-fixture',
      'image-fixture',
      'video-fixture',
      'unknown-fixture',
    ]) {
      assert.equal(
        await page.$(`article[aria-label="${name}"] .model-protocol-template`),
        null,
        name,
      );
    }
    if (screenshotDir)
      await page.screenshot({
        path: `${screenshotDir}/catalog-${mobile ? 'mobile' : 'desktop'}.png`,
      });
    await page.click('button[aria-label="表格视图"]');
    await page.waitForSelector('table .model-protocol-template');
    assert.match(
      await page.$eval('table', (el) => el.innerText),
      /Responses · 示例 · 未声明/,
    );
    await page.click('button[aria-label="卡片视图"]');
    await page.waitForSelector('article[aria-label="gpt-chat-fixture"]');
    await page.click('button[aria-label="更多筛选"]');
    await page.waitForFunction(
      () =>
        document.querySelector(
          '[aria-label="已声明协议"] .pricing-filter-chip-count',
        ) != null,
    );
    const responseCount = await page.$eval(
      '[aria-label="已声明协议"]',
      (el) => {
        const b = [...el.querySelectorAll('button')].find(
          (b) =>
            b.querySelector('.pricing-filter-chip-label')?.textContent ===
            'Responses',
        );
        b.click();
        return b.querySelector('.pricing-filter-chip-count').textContent;
      },
    );
    assert.equal(
      responseCount,
      '1',
      'undeclared example must not count as Responses support',
    );
    await page.keyboard.press('Escape');
    await page.waitForSelector('.pricing-filter-drawer .semi-sidesheet-inner', {
      hidden: true,
    });
    await page.waitForFunction(
      () => document.querySelectorAll('.pricing-model-card').length === 1,
    );
    assert(await page.$('article[aria-label="responses-fixture"]'));
    assert.match(
      await page.$eval('.pricing-active-filters', (el) => el.innerText),
      /已声明协议: Responses/,
    );
    await page.click('.pricing-reset');
    await page.waitForSelector(
      'article[aria-label="gpt-chat-fixture"] .model-protocol-template',
    );
    await page.click(
      'article[aria-label="gpt-chat-fixture"] .model-protocol-template',
    );
    await page.waitForSelector(
      '#endpoints [data-protocol="openai-response"][data-declared="false"]',
    );
    assert.match(page.url(), /\/model\/gpt-chat-fixture#endpoints$/);
    assert.match(
      await page.$eval('#endpoints', (el) => el.innerText),
      /未声明不等于不支持/,
    );
    assert.equal(
      await page.$eval(
        '#endpoints [data-protocol="openai"]',
        (el) => el.dataset.declared,
      ),
      'true',
    );
    const path = await page.$eval(
      '#endpoints [data-protocol="openai-response"] code',
      (el) => el.textContent,
    );
    assert.equal(path, '/v1/responses');
    if (screenshotDir)
      await page.screenshot({
        path: `${screenshotDir}/endpoints-${mobile ? 'mobile' : 'desktop'}.png`,
      });
    await page.select('select[aria-label="接口协议"]', 'openai-response');
    await page.waitForFunction(() =>
      document
        .querySelector('.model-quickstart pre')
        ?.textContent.includes('/v1/responses'),
    );
    await page.evaluate(() => {
      document.querySelector('.model-quickstart-code-heading button').click();
    });
    await page.waitForFunction(() =>
      window.__protocolCopy?.includes('/v1/responses'),
    );
    assert.match(await page.evaluate(() => window.__protocolCopy), /input/);
    await page.select('select[aria-label="代码语言"]', 'TypeScript');
    await page.waitForFunction(() =>
      document
        .querySelector('.model-quickstart pre')
        ?.textContent.includes('fetch('),
    );
    assert.equal(
      await page.$eval('select[aria-label="接口协议"]', (el) => el.value),
      'openai-response',
    );
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth + 2,
      ),
      true,
    );
    assert.deepEqual(errors, []);
    assert.deepEqual(mutations, []);
    await page.close();
    console.log(
      `${mobile ? 'Mobile/dark' : 'Desktop'} protocol labels, metadata boundaries, detail navigation and copy passed`,
    );
  }
} finally {
  await browser.close();
}
