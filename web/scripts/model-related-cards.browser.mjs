import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import puppeteer from 'puppeteer-core';

const base = process.env.PRICING_TEST_URL || 'http://127.0.0.1:5206';
const output = process.env.RELATED_SCREENSHOT_DIR;
const user = { id: 7, role: 1, username: 'related-fixture', group: 'default' };
const status = {
  system_name: 'Meta API',
  server_address: base,
  quota_per_unit: 500000,
  quota_display_type: 'USD',
  HeaderNavModules: JSON.stringify({
    pricing: { enabled: true, requireAuth: false },
  }),
};
const models = [
  'kimi-current',
  'kimi-k2.5/preview',
  'kimi-k2.6',
  'kimi-k2.7-code',
  'kimi-k2.7-code-highspeed',
  'kimi-k3',
  'kimi-k3.1-pro-highspeed',
  'z-kimi-extra',
].map((model_name, index) => ({
  model_name,
  vendor_id: 1,
  icon: 'Moonshot',
  display_name: index === 1 ? 'Kimi K2.5' : model_name,
  description: '面向编程与知识工作的模型，支持长上下文理解和多轮对话。',
  context_length: index === 2 ? null : 1000000,
  quota_type: 0,
  model_ratio: index === 2 ? 0 : 0.15,
  completion_ratio: 6,
  enable_groups: ['Public'],
  supported_endpoint_types: ['openai'],
}));
models.push({ ...models[0], model_name: 'other-vendor', vendor_id: 2 });
const pricing = {
  success: true,
  vendors: [
    { id: 1, name: 'Moonshot AI', icon: 'Moonshot' },
    { id: 2, name: 'Other' },
  ],
  usable_group: { Public: 'Public' },
  group_ratio: { Public: 1 },
  supported_endpoint: {
    openai: { path: '/v1/chat/completions', method: 'POST' },
  },
  data: models,
};
const browser = await puppeteer.launch({
  executablePath:
    process.env.CHROME_PATH ||
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  headless: true,
});
try {
  if (output) await fs.mkdir(output, { recursive: true });
  for (const anonymous of [false, true]) {
    const context = await browser.createBrowserContext();
    const page = await context.newPage();
    const errors = [],
      requests = [],
      writes = [];
    let failHealth = !anonymous;
    let onlyCurrent = false;
    page.on('pageerror', (error) => errors.push(error.message));
    await page.setViewport({ width: 1440, height: 1000 });
    await page.evaluateOnNewDocument(
      ({ user, status, anonymous }) => {
        if (!anonymous) localStorage.setItem('user', JSON.stringify(user));
        localStorage.setItem('status', JSON.stringify(status));
        localStorage.setItem('site-language', 'zh-CN');
        localStorage.setItem('theme-mode', 'light');
        Object.defineProperty(navigator, 'clipboard', {
          value: {
            writeText: async (text) => {
              window.__copied = text;
            },
          },
        });
      },
      { user, status, anonymous },
    );
    await page.setRequestInterception(true);
    page.on('request', async (request) => {
      const url = new URL(request.url());
      if (url.origin !== base) return request.abort();
      if (!url.pathname.startsWith('/api/')) return request.continue();
      if (request.method() !== 'GET') writes.push(url.pathname);
      let body = { success: true, data: [] };
      if (url.pathname === '/api/status') body.data = status;
      if (url.pathname === '/api/user/self') body.data = user;
      if (url.pathname === '/api/pricing')
        body = { ...pricing, data: onlyCurrent ? [models[0]] : models };
      if (url.pathname === '/api/health/groups')
        body.data = { enabled: true, groups: [] };
      if (url.pathname === '/api/health/models') {
        const names = url.searchParams.getAll('model');
        requests.push(names);
        body = failHealth
          ? { success: false }
          : {
              success: true,
              data: {
                enabled: true,
                models: names.map((model, index) => ({
                  model,
                  avg_ttft_ms: index === 1 ? null : 910,
                  ttft_count: 50,
                  avg_output_tps: index === 1 ? null : 73,
                  throughput_sample_count: 50,
                })),
              },
            };
      }
      await request.respond({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(body),
      });
    });
    await page.goto(base + '/model/kimi-current');
    await page.waitForSelector('#related article');
    assert.equal(await page.$$eval('#related article', (els) => els.length), 6);
    assert.equal(await page.$('#related .pricing-catalog-pagination'), null);
    assert.equal(await page.$('#related input[type="checkbox"]'), null);
    assert.equal(
      await page.$('#related article[aria-label="kimi-current"]'),
      null,
    );
    assert.equal(
      await page.$('#related article[aria-label="other-vendor"]'),
      null,
    );
    assert.equal(
      await page.$eval('#related h3 a', (el) => el.getAttribute('href')),
      '/model/kimi-k2.5%2Fpreview',
    );
    if (anonymous) {
      assert.match(
        await page.$eval('#related', (el) => el.innerText),
        /登录后查看/,
      );
      assert.deepEqual(requests, []);
    } else {
      await page.waitForSelector('#related .pricing-card-health-status button');
      failHealth = false;
      await page.click('#related .pricing-card-health-status button');
      await page.waitForFunction(() =>
        document.querySelector('#related').innerText.includes('73.0 tok/s'),
      );
      assert.match(
        await page.$eval('#related', (el) => el.innerText),
        /910 ms/,
      );
      assert.match(
        await page.$eval(
          '#related article[aria-label="kimi-k2.6"]',
          (el) => el.innerText,
        ),
        /未配置[\s\S]*暂无数据/,
      );
      assert.deepEqual(
        requests[0],
        models.slice(1, 7).map((model) => model.model_name),
      );
      assert.deepEqual(
        await page.$$eval(
          '#related article:first-child .pricing-card-prices strong',
          (els) => els.map((el) => el.textContent),
        ),
        ['¥0.30', '¥1.80'],
      );
    }
    const beforeCopy = page.url();
    await page.click('#related article:first-child button');
    assert.equal(page.url(), beforeCopy);
    assert.equal(
      await page.evaluate(() => window.__copied),
      'kimi-k2.5/preview',
    );
    for (const width of [1440, 768, 390, 320]) {
      await page.setViewport({ width, height: 1000 });
      await page.evaluate((dark) => {
        document.documentElement.classList.toggle('dark', dark);
        document.body.setAttribute('theme-mode', dark ? 'dark' : 'light');
      }, width <= 390);
      const layout = await page.$eval(
        '#related .pricing-catalog-grid',
        (el) => ({
          columns: getComputedStyle(el).gridTemplateColumns.split(' ').length,
          overflow: el.scrollWidth > el.clientWidth + 1,
        }),
      );
      assert.equal(layout.columns, width <= 600 ? 1 : 2);
      assert.equal(layout.overflow, false);
      assert.equal(
        await page.evaluate(
          () => document.documentElement.scrollWidth > innerWidth + 1,
        ),
        false,
      );
      if (output && !anonymous && [1440, 390].includes(width))
        await (
          await page.$('#related')
        ).screenshot({ path: `${output}/related-${width}.png` });
    }
    await page.click('#pricing button[aria-label="按K显示单位"]');
    await page.waitForFunction(() =>
      document
        .querySelector('#related .pricing-card-prices')
        .innerText.includes('/K'),
    );
    await page.click('#related h3 a');
    await page.waitForFunction(
      () => location.pathname === '/model/kimi-k2.5%2Fpreview',
    );
    await page.waitForSelector('#related article');
    assert.match(
      await page.$eval('#related .pricing-card-prices', (el) => el.innerText),
      /\/K/,
    );
    assert.equal(
      await page.$('#related article[aria-label="Kimi K2.5"]'),
      null,
    );
    onlyCurrent = true;
    await page.goto(base + '/model/kimi-current');
    await page.waitForSelector('.model-detail-hero');
    assert.equal(await page.$('#related'), null);
    assert.deepEqual(errors, []);
    assert.deepEqual(writes, []);
    await context.close();
  }
  console.log(
    'Related cards: layout, data, bounded queries, retry, anonymous state, navigation, copy and empty state passed.',
  );
} finally {
  await browser.close();
}
