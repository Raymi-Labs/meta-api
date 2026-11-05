import assert from 'node:assert/strict';
import { mkdirSync } from 'node:fs';
import puppeteer from 'puppeteer-core';
const baseURL = process.env.WALLET_TEST_URL || 'http://127.0.0.1:5188';
const output = process.env.WALLET_SCREENSHOTS || '/tmp/meta-api-wallet-costs';
mkdirSync(output, { recursive: true });
const user = {
  id: 7,
  username: 'wallet-fixture',
  display_name: '示例账户',
  role: 1,
  status: 1,
  group: 'default',
  quota: 640000000,
  used_quota: 1960250000,
  request_count: 12480,
  transferable_quota: 90000000,
  aff_quota: 40000000,
  aff_history_quota: 130000000,
  aff_count: 12,
};
const status = {
  system_name: 'Meta API',
  server_address: baseURL,
  quota_per_unit: 500000,
  quota_display_type: 'CUSTOM',
  custom_currency_symbol: '⚡',
  custom_currency_exchange_rate: 1,
  price: 0.2,
  stripe_unit_price: 0.03,
  min_topup: 10,
  usd_exchange_rate: 1,
  chats: '[]',
};
const now = Math.floor(Date.now() / 1000);
const costReference = {
  method: 'stripe',
  baseline_method: 'alipay',
  sample_count: 3,
  sample_amount: 100,
  observed_at: now - 60,
  expires_at: now + 86400,
  baseline_amount: 20,
  reference_paid: 20.81,
  reference_usd: 3,
  difference_percent: 4.05,
};
const browser = await puppeteer.launch({
  executablePath:
    process.env.CHROME_PATH ||
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  headless: true,
});
try {
  for (const mobile of [false, true]) {
    const page = await browser.newPage();
    const errors = [],
      writes = [],
      reads = [];
    let failQuotes = false,
      failInfo = false,
      holdQuotes = false,
      withPlans = false,
      releaseOld = [];
    page.on('pageerror', (error) => errors.push(error.message));
    await page.setViewport({
      width: mobile ? 390 : 1440,
      height: mobile ? 2600 : 1450,
      deviceScaleFactor: 1,
    });
    await page.evaluateOnNewDocument(
      (user, status) => {
        localStorage.setItem('user', JSON.stringify(user));
        localStorage.setItem('status', JSON.stringify(status));
        localStorage.setItem('site-language', 'zh-CN');
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
    page.on('request', async (request) => {
      const url = new URL(request.url());
      if (url.origin !== baseURL) return request.abort();
      if (!url.pathname.startsWith('/api/')) return request.continue();
      reads.push(url.pathname);
      if (request.method() !== 'GET' && !url.pathname.endsWith('/amount')) {
        writes.push(url.pathname);
        return request.respond({
          status: 403,
          contentType: 'application/json',
          body: JSON.stringify({
            success: false,
            message: 'Fixture forbids writes',
          }),
        });
      }
      let body = { success: true, data: [] },
        httpStatus = 200;
      if (url.pathname === '/api/status') body.data = status;
      else if (url.pathname === '/api/user/self') body.data = user;
      else if (url.pathname === '/api/user/topup/info') {
        if (failInfo) {
          httpStatus = 503;
          body = { success: false };
        } else
          body.data = {
            enable_online_topup: true,
            enable_stripe_topup: true,
            min_topup: 10,
            stripe_min_topup: 1,
            amount_options: [10, 30, 100, 500, 1000, 2000, 5000],
            discount: {},
            creem_products: '[]',
            pay_methods: [
              { type: 'alipay', name: '支付宝' },
              {
                type: 'stripe',
                name: '微信支付',
                display_icon: 'wxpay',
                min_topup: '1',
              },
            ],
            cost_reference: costReference,
          };
      } else if (url.pathname.endsWith('/amount')) {
        const { amount } = JSON.parse(request.postData());
        const stripe = url.pathname.includes('/stripe/');
        body = failQuotes
          ? { message: 'error', data: 'Fixture unavailable' }
          : {
              message: 'success',
              data: (amount * (stripe ? 0.03 : 0.2)).toFixed(2),
              currency: stripe ? 'USD' : 'CNY',
              estimated: true,
            };
        if (holdQuotes && amount === 100) {
          await new Promise((resolve) => releaseOld.push(resolve));
        }
      } else if (url.pathname === '/api/subscription/plans')
        body.data = withPlans
          ? [
              {
                plan: {
                  id: 1,
                  title: '验收套餐',
                  price_amount: 10,
                  total_amount: 50000000,
                  duration_unit: 'month',
                  duration_value: 1,
                  enabled: true,
                },
              },
            ]
          : [];
      else if (url.pathname === '/api/subscription/self')
        body.data = { subscriptions: [], active_subscriptions: [] };
      else if (url.pathname === '/api/user/aff') body.data = 'DEMO';
      else if (url.pathname === '/api/user/redemptions/self/summary')
        body.data = {
          minimum_quota: 5000000,
          daily_created_count: 0,
          daily_created_quota: 0,
          active_count: 0,
          reset_at: now + 3600,
        };
      else if (url.pathname === '/api/user/redemptions/self')
        body.data = { items: [] };
      else if (
        url.pathname.includes('payment-records') ||
        url.pathname.includes('invoices')
      )
        body.data = { items: [], total: 0, page: 1, page_size: 10 };
      try {
        await request.respond({
          status: httpStatus,
          contentType: 'application/json',
          body: JSON.stringify(body),
        });
      } catch {}
    });
    const click = async (text, root = 'body') => {
      await page.waitForFunction(
        (text, root) =>
          [...document.querySelectorAll(root + ' button')].some(
            (e) => e.textContent.trim() === text && !e.disabled,
          ),
        {},
        text,
        root,
      );
      await page.evaluate(
        (text, root) =>
          [...document.querySelectorAll(root + ' button')]
            .find((e) => e.textContent.trim() === text && !e.disabled)
            .click(),
        text,
        root,
      );
    };
    const preset = async (index) =>
      page.evaluate(
        (index) => document.querySelectorAll('.wallet-preset')[index].click(),
        index,
      );
    const method = async (index) =>
      page.evaluate(
        (index) =>
          document.querySelectorAll('.wallet-payment-method')[index].click(),
        index,
      );
    const total = async (text) =>
      page.waitForFunction(
        (text) =>
          document
            .querySelector('.wallet-payment-total')
            ?.textContent.includes(text),
        {},
        text,
      );
    await page.goto(baseURL + '/console/topup', { waitUntil: 'networkidle0' });
    await page.waitForSelector('.wallet-payment-method');
    await preset(4);
    await method(1);
    await total('30.00');
    await page.waitForFunction(() =>
      document
        .querySelector('.wallet-payment-methods')
        ?.textContent.includes('4.05%'),
    );
    const columns = await page.$eval(
      '.wallet-content-grid',
      (e) => getComputedStyle(e).gridTemplateColumns.split(' ').length,
    );
    assert.equal(columns, mobile ? 1 : 2);
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth > innerWidth,
      ),
      false,
    );
    await page.screenshot({
      path: output + '/wallet-' + (mobile ? 'mobile' : 'desktop') + '.png',
      fullPage: true,
    });
    if (!mobile) {
      await page.evaluate(() => {
        document.body.setAttribute('theme-mode', 'dark');
        document.documentElement.classList.add('dark');
      });
      await page.screenshot({
        path: output + '/wallet-dark.png',
        fullPage: true,
      });
      await page.evaluate(() => {
        document.body.removeAttribute('theme-mode');
        document.documentElement.classList.remove('dark');
      });
    }
    await page.click('.wallet-pay-button');
    await page.waitForSelector('.semi-modal');
    assert(
      await page.$eval(
        '.semi-modal',
        (e) =>
          e.textContent.includes('30.00') && e.textContent.includes('微信支付'),
      ),
    );
    await page.click('.semi-modal-close');
    await page.waitForSelector('.semi-modal', { hidden: true });
    await click('支付记录', '.wallet-page-heading');
    await page.waitForSelector('.semi-modal');
    assert(reads.includes('/api/user/payment-records/self'));
    assert(!reads.includes('/api/user/payment-records'));
    await page.click('.semi-modal-close');
    await page.waitForSelector('.semi-modal', { hidden: true });
    await click('发票管理', '.wallet-page-heading');
    await page.waitForSelector('.semi-modal');
    assert(
      await page.$eval('.semi-modal', (e) => e.textContent.includes('发票')),
    );
    await page.click('.semi-modal-close');
    await page.waitForSelector('.semi-modal', { hidden: true });
    await click('创建兑换码');
    await page.waitForSelector('.semi-modal');
    assert(
      await page.$eval('.semi-modal', (e) =>
        e.textContent.includes('可创建兑换码额度'),
      ),
    );
    await page.click('.semi-modal-close');
    await page.waitForSelector('.semi-modal', { hidden: true });
    await click('划转到余额');
    await page.waitForSelector('.semi-modal');
    await page.click('.semi-modal-close');
    await page.waitForSelector('.semi-modal', { hidden: true });
    await preset(0);
    await total('0.30');
    assert(await page.$eval('.wallet-pay-button', (e) => e.disabled));
    await method(0);
    await total('2.00');
    assert(!(await page.$eval('.wallet-pay-button', (e) => e.disabled)));
    if (!mobile) {
      holdQuotes = true;
      await preset(2);
      await page.waitForFunction(() =>
        document
          .querySelector('.wallet-payment-total')
          .textContent.includes('计算中'),
      );
      // Wait until the held requests have actually started, then supersede them.
      await new Promise((resolve) => setTimeout(resolve, 550));
      await preset(4);
      await total('200.00');
      holdQuotes = false;
      releaseOld.forEach((resolve) => resolve());
      await new Promise((resolve) => setTimeout(resolve, 200));
      await total('200.00');
      failQuotes = true;
      await preset(3);
      await page.waitForFunction(() =>
        document
          .querySelector('.wallet-payment-total')
          .textContent.includes('—'),
      );
      assert(await page.$eval('.wallet-pay-button', (e) => e.disabled));
      failQuotes = false;
      await click('重新获取报价');
      await page.waitForSelector('.semi-modal');
      await page.click('.semi-modal-close');
      await page.waitForSelector('.semi-modal', { hidden: true });
      failInfo = true;
      await page.reload({ waitUntil: 'networkidle0' });
      await page.waitForFunction(() =>
        document
          .querySelector('.wallet-recharge-card')
          .textContent.includes('加载失败'),
      );
      assert(await page.$('.wallet-redemption-card input'));
      failInfo = false;
      await click('重试', '.wallet-recharge-card');
      await page.waitForSelector('.wallet-payment-method');
    }
    withPlans = true;
    await page.reload({ waitUntil: 'networkidle0' });
    assert(await page.$('.wallet-payment-method'));
    await page.evaluate(() =>
      [...document.querySelectorAll('[role=tab]')]
        .find((e) => e.textContent.includes('订阅套餐'))
        .click(),
    );
    await page.waitForFunction(() =>
      document.body.textContent.includes('验收套餐'),
    );
    assert(
      await page.$eval(
        '.wallet-account-summary',
        (e) => e.getBoundingClientRect().height > 0,
      ),
    );
    assert.deepEqual(writes, []);
    assert.deepEqual(errors, []);
    await page.close();
    console.log(
      'PASS',
      mobile ? 'mobile' : 'desktop',
      'layout, currency, references, minimum, confirmation, records ownership, redemption, transfer, failure and stale response',
    );
  }
} catch (error) {
  const pages = await browser.pages();
  await pages
    .at(-1)
    ?.screenshot({ path: output + '/error.png', fullPage: true });
  throw error;
} finally {
  await browser.close();
}
