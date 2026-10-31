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
    supports_cache_read: true,
    cache_ratio: 0.1,
    supports_cache_creation: true,
    cache_creation_ratio: 1.25,
    supports_cache_creation_ttl: true,
    cache_creation_ratio_1h: 2,
    ...(model_name === 'responses-fixture'
      ? {
          billing_mode: 'tiered_expr',
          billing_expr: 'tier("base", p*4+c*20+cr*.4+cc*5+cc1h*8)',
        }
      : {}),
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
      mutations = [],
      groupHealthRequests = [];
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
      if (url.pathname === '/api/health/groups') {
        groupHealthRequests.push(url.searchParams.get('model'));
        body = {
          success: true,
          data: {
            enabled: true,
            groups: [
              {
                group: 'Public',
                request_count: 100,
                success_count: 99,
                success_rate: 99,
                uptime_rate: 99,
                series: [
                  {
                    ts: 1793451600,
                    request_count: 100,
                    success_count: 99,
                    success_rate: 99,
                    uptime_rate: 99,
                  },
                ],
              },
            ],
          },
        };
      }
      if (url.pathname === '/api/health/models')
        body = { success: true, data: { enabled: true, models: [] } };
      await req.respond({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(body),
      });
    });
    await page.goto(baseURL + '/pricing');
    await page.waitForSelector('article[aria-label="gpt-chat-fixture"]');
    assert.equal(await page.$('.pricing-content-body [role="tab"]'), null);
    assert.equal(await page.$('.group-health-overview'), null);
    assert.deepEqual(
      groupHealthRequests,
      [],
      'catalog must not load group overview',
    );
    assert.equal(
      await page.$eval(
        'body',
        (el) => el.getAttribute('theme-mode') === 'dark',
      ),
      mobile,
    );
    for (const [name] of entries) {
      const card = await page.$eval(
        `article[aria-label="${name}"]`,
        (el) => el.innerText,
      );
      assert.doesNotMatch(
        card,
        /已声明协议|Responses · 示例|缓存.*价格|阶梯计费|OpenAI|动态计费|按量计费/,
        name,
      );
      assert.match(card, /输入/);
      assert.match(card, /输出/);
    }
    assert.deepEqual(
      await page.$$eval(
        'article[aria-label="gpt-chat-fixture"] .pricing-card-prices strong',
        (els) => els.map((el) => el.textContent),
      ),
      ['¥2.0000', '¥4.0000'],
    );
    assert.deepEqual(
      await page.$$eval(
        'article[aria-label="responses-fixture"] .pricing-card-prices strong',
        (els) => els.map((el) => el.textContent),
      ),
      ['¥4.0000', '¥20.0000'],
    );
    await page.click('button[aria-label="复制模型名称: gpt-chat-fixture"]');
    await page.waitForFunction(
      () => window.__protocolCopy === 'gpt-chat-fixture',
    );
    await page.click('input[aria-label="选择模型: gpt-chat-fixture"]');
    assert.match(page.url(), /\/pricing$/);
    await page.click('input[aria-label="选择模型: gpt-chat-fixture"]');
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth + 2,
      ),
      true,
    );
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
    await page.waitForSelector('article[aria-label="gpt-chat-fixture"]');
    await page.click('article[aria-label="gpt-chat-fixture"]');
    await page.waitForSelector('#quick-start .model-quickstart pre');
    assert.match(page.url(), /\/model\/gpt-chat-fixture$/);
    await page.waitForSelector('.model-group-uptime:not([disabled])');
    await page.click('.model-group-uptime');
    await page.waitForSelector('.model-group-row .group-health-panel');
    assert.equal(
      await page.$eval('.model-group-uptime', (el) =>
        el.getAttribute('aria-expanded'),
      ),
      'true',
    );
    assert(groupHealthRequests.every((model) => model === 'gpt-chat-fixture'));
    await page.click('.model-group-uptime');
    await page.waitForSelector('.model-group-row .group-health-panel', {
      hidden: true,
    });
    for (const key of ['cache', 'create-cache', 'create-cache-1h']) {
      assert(await page.$(`.model-group-row [data-price-key="${key}"]`), key);
    }
    await page.select('select[aria-label="接口协议"]', 'openai-response');
    await page.$eval('#quick-start', (el) => el.scrollIntoView());
    assert.equal(await page.$('#endpoints'), null);
    assert.doesNotMatch(
      await page.$eval('.model-detail-sections', (el) => el.innerText),
      /API端点/,
    );
    assert.match(
      await page.$eval('#quick-start', (el) => el.innerText),
      /该模型尚未声明支持此协议/,
    );
    assert.equal(
      await page.$eval('select[aria-label="接口协议"]', (el) => el.value),
      'openai-response',
    );
    if (screenshotDir)
      await page.screenshot({
        path: `${screenshotDir}/quick-start-${mobile ? 'mobile' : 'desktop'}.png`,
      });
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
    await page.select('select[aria-label="接口协议"]', 'openai');
    await page.waitForFunction(() =>
      document
        .querySelector('.model-quickstart pre')
        ?.textContent.includes('/v1/chat/completions'),
    );
    assert.equal(await page.$('.model-quickstart-note[role="status"]'), null);
    await page.goto(baseURL + '/model/gpt-chat-fixture#endpoints');
    await page.waitForSelector('#quick-start .model-quickstart pre');
    await page.waitForFunction(() => {
      const heading = document.querySelector('#quick-start h2');
      const top = heading.getBoundingClientRect().top;
      return top >= 0 && top < window.innerHeight / 2;
    });
    assert.equal(await page.$('#endpoints'), null);
    assert.equal(
      await page.$eval('select[aria-label="接口协议"]', (el) => el.value),
      'openai',
      'legacy endpoint links retain the declared protocol default',
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
