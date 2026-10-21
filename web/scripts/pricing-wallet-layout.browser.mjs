import assert from 'node:assert/strict';
import puppeteer from 'puppeteer-core';

const baseURL = process.env.PRICING_TEST_URL || 'http://127.0.0.1:5179';
const browser = await puppeteer.launch({
  executablePath:
    process.env.CHROME_PATH ||
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  headless: true,
});
const user = {
  id: 7,
  username: 'layout-fixture',
  role: 1,
  group: 'default',
  quota: 50000000,
  transferable_quota: 10000000,
  used_quota: 100000,
  request_count: 4,
};
const status = {
  system_name: 'New API',
  chats: '[]',
  price: 0.2,
  usd_exchange_rate: 7,
  quota_per_unit: 500000,
  quota_display_type: 'CUSTOM',
  custom_currency_symbol: '⚡',
  custom_currency_exchange_rate: 1,
};
const pricing = {
  success: true,
  vendors: [
    { id: 1, name: 'OpenAI' },
    { id: 2, name: 'Claude' },
    { id: 3, name: 'Inception' },
  ],
  usable_group: {
    Premium: '独享资源，适合稳定调用',
    Standard: '适合日常使用',
    Shared: '跨厂商共享',
    Free: '免费额度',
  },
  group_ratio: { Premium: 1, Standard: 0.5, Shared: 0.5, Free: 0 },
  supported_endpoint: {},
  auto_groups: [],
  data: [
    { model_name: 'gpt-5.9', vendor_id: 1, quota_type: 0 },
    { model_name: 'gpt-5.10', vendor_id: 1, quota_type: 0 },
    { model_name: 'gpt-6.1', vendor_id: 1, quota_type: 0 },
    {
      model_name: 'ordinary-fixture',
      vendor_id: 1,
      quota_type: 0,
      cache_ratio: 0.1,
    },
    {
      model_name: 'tiered-fixture',
      vendor_id: 1,
      quota_type: 0,
      billing_mode: 'tiered_expr',
      billing_expr: 'v1:tier("base", p*10+c*20+cr*1+cc*12.5)',
    },
    { model_name: 'image-fixture', vendor_id: 1, quota_type: 1 },
    { model_name: 'hourly-fixture', vendor_id: 1, quota_type: 2 },
    {
      model_name: 'other-fixture',
      vendor_id: 2,
      quota_type: 0,
      enable_groups: ['Shared'],
    },
    {
      model_name: 'mercury-fixture',
      vendor_id: 3,
      quota_type: 0,
      enable_groups: ['Shared'],
    },
  ].map((m) => ({
    model_ratio: 2,
    completion_ratio: 3,
    model_price: 2,
    enable_groups: ['Premium', 'Standard', 'Shared', 'Free'],
    supported_endpoint_types: ['openai'],
    ...m,
  })),
};

try {
  for (const mobile of [false, true]) {
    const page = await browser.newPage();
    const errors = [];
    const mutations = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await page.setViewport({ width: mobile ? 390 : 1440, height: 1000 });
    await page.evaluateOnNewDocument(
      (user, status) => {
        localStorage.setItem('user', JSON.stringify(user));
        localStorage.setItem('status', JSON.stringify(status));
        localStorage.setItem('i18nextLng', 'zh-CN');
        localStorage.setItem('quota_per_unit', '500000');
        localStorage.setItem('quota_display_type', 'CUSTOM');
        localStorage.setItem(
          'newapi:user-onboarding:regular:v1:user:7',
          'done',
        );
      },
      user,
      status,
    );
    await page.setRequestInterception(true);
    page.on('request', async (req) => {
      const url = new URL(req.url());
      if (url.origin !== baseURL) return req.abort();
      if (!url.pathname.startsWith('/api/')) return req.continue();
      if (req.method() === 'POST' && !url.pathname.endsWith('/amount'))
        mutations.push(url.pathname);
      let body = { success: true, data: [] };
      switch (url.pathname) {
        case '/api/status':
          body.data = status;
          break;
        case '/api/pricing':
          body = pricing;
          break;
        case '/api/user/self':
          body.data = user;
          break;
        case '/api/user/topup/info':
          body.data = {
            enable_online_topup: true,
            min_topup: 10,
            amount_options: [10, 30, 100, 500, 1000, 2000, 5000],
            discount: {},
            pay_methods: [
              { type: 'alipay', name: '支付宝' },
              { type: 'wxpay', name: '微信支付' },
            ],
          };
          break;
        case '/api/user/amount':
          body = { message: 'success', data: '2.00', currency: 'CNY' };
          break;
        case '/api/subscription/self':
          body.data = { subscriptions: [], active_subscriptions: [] };
          break;
        case '/api/user/aff':
          body.data = 'fixture';
          break;
      }
      await req.respond({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(body),
      });
    });
    const click = async (text, root = 'body') => {
      await page.waitForFunction(
        (text, root) =>
          Array.from(document.querySelectorAll(root + ' button')).some(
            (b) => b.textContent.trim() === text && !b.disabled,
          ),
        {},
        text,
        root,
      );
      await page.evaluate(
        (text, root) =>
          Array.from(document.querySelectorAll(root + ' button'))
            .find((b) => b.textContent.trim() === text && !b.disabled)
            .click(),
        text,
        root,
      );
    };
    await page.goto(baseURL + '/pricing', { waitUntil: 'networkidle0' });
    if (!mobile) {
      // Both independent columns must keep their search fields on one baseline.
      for (const width of [1440, 1024, 768]) {
        await page.setViewport({ width, height: 1000 });
        for (const dark of [false, true]) {
          await page.evaluate((dark) => {
            if (dark) document.body.setAttribute('theme-mode', 'dark');
            else document.body.removeAttribute('theme-mode');
            document.documentElement.classList.toggle('dark', dark);
          }, dark);
          await page.waitForFunction(
            (dark) =>
              getComputedStyle(
                document.querySelector('.pricing-model-table th'),
              ).backgroundColor ===
              (dark ? 'rgb(29, 35, 48)' : 'rgb(248, 250, 252)'),
            {},
            dark,
          );
          const layout = await page.evaluate(() => {
            const left = document.querySelector('.pricing-sidebar input');
            const right = document.querySelector(
              '.pricing-search-header input',
            );
            return {
              delta: Math.abs(
                left.getBoundingClientRect().top -
                  right.getBoundingClientRect().top,
              ),
              sidebar: getComputedStyle(
                document.querySelector('.pricing-sidebar'),
              ).backgroundColor,
              content: getComputedStyle(
                document.querySelector('.pricing-content'),
              ).backgroundColor,
              overflow: document.documentElement.scrollWidth > innerWidth,
              tableHeader: getComputedStyle(
                document.querySelector('.pricing-model-table th'),
              ).backgroundColor,
              selectedMarker: getComputedStyle(
                document.querySelector(
                  '.pricing-group-list button[aria-pressed="true"]',
                ),
              ).boxShadow,
            };
          });
          assert(
            layout.delta <= 1,
            `Search alignment ${width}/${dark}: ${layout.delta}px`,
          );
          assert.notEqual(layout.sidebar, layout.content);
          assert.notEqual(layout.selectedMarker, 'none');
          assert.equal(
            layout.tableHeader,
            dark ? 'rgb(29, 35, 48)' : 'rgb(248, 250, 252)',
          );
          assert.equal(layout.overflow, false);
          await page.screenshot({
            path: `/tmp/pricing-alignment-${width}-${dark ? 'dark' : 'light'}.png`,
            fullPage: true,
          });
        }
      }
      await page.evaluate(() => {
        document.body.removeAttribute('theme-mode');
        document.documentElement.classList.remove('dark');
      });
      await page.setViewport({ width: 1440, height: 1000 });
    }
    assert.deepEqual(
      await page.$$eval('.pricing-vendors button', (els) =>
        els.map((e) => e.textContent),
      ),
      ['全部供应商', 'Claude', 'OpenAI', 'Inception'],
    );
    await click('OpenAI', '.pricing-vendors');
    if (mobile) {
      await page.waitForFunction(() =>
        document
          .querySelector('.pricing-view-container')
          ?.textContent.includes('缓存'),
      );
      await page.screenshot({
        path: '/tmp/pricing-wallet-mobile-card.png',
        fullPage: true,
      });
      await click('更多筛选');
      await page.evaluate(() =>
        Array.from(document.querySelectorAll('.semi-modal-body button'))
          .find((b) => b.textContent.trim() === '表格视图')
          .querySelector('input')
          .click(),
      );
      await page.evaluate(() =>
        Array.from(document.querySelectorAll('.semi-modal-body button'))
          .find((b) => b.textContent.startsWith('Standard'))
          .click(),
      );
      await click('确定', '.semi-modal');
      await page.waitForSelector('.semi-modal', { hidden: true });
    } else {
      const groups = await page.$$eval('.pricing-group-label', (els) =>
        els.map((e) => e.textContent),
      );
      assert.deepEqual(groups, [
        '全部分组',
        'Premium1×',
        'Standard0.5×',
        'Shared0.5×',
        'Free0×',
      ]);
      await page.evaluate(() =>
        Array.from(document.querySelectorAll('.pricing-group-list button'))
          .find((b) => b.textContent.startsWith('Standard'))
          .click(),
      );
    }
    await page.waitForFunction(() =>
      document
        .querySelector('.pricing-price-context')
        ?.textContent.includes('Standard'),
    );
    const rows = await page.$$eval('.semi-table-row', (els) =>
      els.map((e) => e.textContent),
    );
    const versionRows = rows.filter((row) => row.includes('gpt-'));
    assert(
      versionRows[0].includes('gpt-6.1') &&
        versionRows[1].includes('gpt-5.10') &&
        versionRows[2].includes('gpt-5.9'),
    );
    assert(
      !(await page.evaluate(() =>
        document.body.textContent.includes('当前查看的分组为'),
      )),
    );
    const ordinary = rows.find((r) => r.includes('ordinary-fixture'));
    assert(
      ordinary.includes('¥0.4000') &&
        ordinary.includes('¥1.2000') &&
        ordinary.includes('¥0.0400'),
      ordinary,
    );
    const tiered = rows.find((r) => r.includes('tiered-fixture'));
    assert(
      tiered.includes('¥1') && tiered.includes('¥2') && tiered.includes('¥0.1'),
      tiered,
    );
    assert(rows.find((r) => r.includes('image-fixture')).includes('/ 次'));
    assert(rows.find((r) => r.includes('hourly-fixture')).includes('/ 小时'));
    await page.screenshot({
      path: '/tmp/pricing-wallet-' + (mobile ? 'mobile' : 'desktop') + '.png',
      fullPage: true,
    });
    await page.click('button[aria-label="复制模型名称: ordinary-fixture"]');
    assert.equal(
      await page.$('.semi-sidesheet'),
      null,
      'Copy must not open details',
    );
    await page.evaluate(() =>
      Array.from(
        Array.from(document.querySelectorAll('.semi-table-row'))
          .find((r) => r.textContent.includes('tiered-fixture'))
          .querySelectorAll('button'),
      )
        .find((b) => b.textContent === '价格详情')
        .click(),
    );
    await page.waitForSelector('.semi-sidesheet');
    assert(
      await page.$eval('.semi-sidesheet', (el) =>
        el.textContent.includes('缓存'),
      ),
    );
    await page.waitForFunction(() => {
      const el = document.querySelector('.semi-sidesheet-close');
      const r = el?.getBoundingClientRect();
      return r && r.width > 0 && r.x + r.width <= innerWidth;
    });
    await page.$eval('.semi-sidesheet-close', (el) => el.click());
    await page.waitForSelector('.semi-sidesheet', { hidden: true });
    // An exclusive group is cleared atomically when changing vendor.
    await click('Claude', '.pricing-vendors');
    await page.waitForFunction(() =>
      document
        .querySelector('.pricing-price-context')
        .textContent.includes('全部分组'),
    );
    assert(
      await page.$eval(
        '.pricing-view-container',
        (el) =>
          el.textContent.includes('other-fixture') &&
          el.textContent.includes('¥0.4000'),
      ),
    );
    await click('更多筛选');
    await page.evaluate(() =>
      Array.from(document.querySelectorAll('.semi-modal-body button'))
        .find((b) => b.textContent.startsWith('Shared'))
        .click(),
    );
    await click('确定', '.semi-modal');
    await page.waitForSelector('.semi-modal', { hidden: true });
    await click('OpenAI', '.pricing-vendors');
    assert(
      await page.$eval('.pricing-price-context', (el) =>
        el.textContent.includes('Shared'),
      ),
    );
    // Empty search results must never reset a usable group.
    await page.$eval('input[placeholder*="搜索模型"]', (input) => {
      Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        'value',
      ).set.call(input, 'no-matching-model');
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await page.waitForFunction(
      () =>
        !document
          .querySelector('.pricing-view-container')
          .textContent.includes('ordinary-fixture'),
    );
    await click('OpenAI', '.pricing-vendors');
    assert(
      await page.$eval('.pricing-price-context', (el) =>
        el.textContent.includes('Shared'),
      ),
    );
    await click('更多筛选');
    await click('重置', '.semi-modal');
    await click('确定', '.semi-modal');
    await page.waitForSelector('.semi-modal', { hidden: true });
    assert(
      await page.$eval('.pricing-price-context', (el) =>
        el.textContent.includes('全部分组'),
      ),
    );
    assert(
      !(await page.evaluate(() =>
        document.body.textContent.includes('当前查看的分组为'),
      )),
    );
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth > innerWidth,
      ),
      false,
    );

    await page.goto(baseURL + '/console/topup', { waitUntil: 'networkidle0' });
    await page.waitForFunction(() =>
      document.body.textContent.includes('到账额度：⚡10.00'),
    );
    assert(
      await page.evaluate(() => document.body.textContent.includes('¥2.00')),
    );
    assert(
      await page.evaluate(() =>
        document.body.textContent.includes(
          '将可转赠额度生成兑换码，赠送给其他用户',
        ),
      ),
    );
    const geometry = await page.evaluate(() => {
      const button = Array.from(document.querySelectorAll('button')).find(
        (b) => b.textContent === '微信支付',
      );
      const note = Array.from(document.querySelectorAll('span')).find(
        (e) => e.textContent === '预计支付金额会按当前支付方式实时计算',
      );
      return {
        buttonBottom: button.getBoundingClientRect().bottom,
        noteTop: note.getBoundingClientRect().top,
      };
    });
    assert(
      geometry.noteTop >= geometry.buttonBottom,
      'Payment note must be below methods',
    );
    await page.screenshot({
      path: '/tmp/wallet-' + (mobile ? 'mobile' : 'desktop') + '.png',
      fullPage: true,
    });
    await click('创建兑换码');
    await page.waitForSelector('.semi-modal');
    assert(
      await page.$eval('.semi-modal', (el) =>
        el.textContent.includes('可创建兑换码额度'),
      ),
    );
    assert.deepEqual(
      mutations,
      [],
      'Preview must not submit payment/redemption',
    );
    assert.deepEqual(errors, []);
    await page.close();
    console.log(
      (mobile ? 'Mobile' : 'Desktop') +
        ' layout, prices, copy/detail, wallet workflow passed',
    );
  }
} catch (error) {
  const pages = await browser.pages();
  await pages
    .at(-1)
    ?.screenshot({ path: '/tmp/pricing-wallet-error.png', fullPage: true });
  throw error;
} finally {
  await browser.close();
}
