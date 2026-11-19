import assert from 'node:assert/strict';
import puppeteer from 'puppeteer-core';

const base = process.env.INVITE_TEST_URL || 'http://127.0.0.1:5187';
const browser = await puppeteer.launch({
  executablePath:
    process.env.CHROME_PATH ||
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  headless: true,
});
const registrations = [],
  visits = [],
  errors = [];
const status = {
  system_name: '元衡',
  footer_html: '',
  logo: '/logo.png',
  chats: '[]',
  password_register: true,
  password_login: true,
  server_address: base,
  quota_per_unit: 500000,
  price: 1,
  usd_exchange_rate: 5,
  quota_display_type: 'USD',
};
const stats = {
  from: '2026-09-23',
  to: '2026-10-22',
  timezone: 'UTC',
  clicks: 42,
  sources: { link: 2, text: 4, button: 6, banner: 30 },
  daily: [{ day: '2026-10-22', clicks: 42 }],
};
const user = {
  id: 7,
  username: 'invite-fixture',
  role: 1,
  group: 'default',
  aff_count: 3,
  quota: 500000,
  aff_quota: 0,
  aff_history_quota: 0,
};
let listEmpty = false,
  requestFail = false;
const writes = [],
  requests = [];
let failStats = false,
  oauth = false;
async function setup(page, loggedIn = false, dark = false) {
  page.setDefaultTimeout(10000);
  page.on('pageerror', (error) => errors.push(error.message));
  await page.evaluateOnNewDocument(
    (user, status, loggedIn, dark) => {
      if (window !== window.top) return;
      Object.defineProperty(navigator, 'clipboard', {
        value: {
          writeText: async (text) => {
            window.__copiedHTML = text;
          },
        },
      });
      localStorage.setItem('i18nextLng', 'zh-CN');
      localStorage.setItem('site-language', 'zh-CN');
      localStorage.setItem('status', JSON.stringify(status));
      localStorage.setItem('system_name', '元衡');
      localStorage.setItem('theme-mode', dark ? 'dark' : 'light');
      if (loggedIn) {
        localStorage.setItem('user', JSON.stringify(user));
        localStorage.setItem(
          'newapi:user-onboarding:regular:v1:user:7',
          'done',
        );
      }
    },
    user,
    status,
    loggedIn,
    dark,
  );
  await page.setRequestInterception(true);
  page.on('requestfailed', (req) =>
    console.error(
      'Failed local fixture request:',
      new URL(req.url()).pathname,
      req.failure()?.errorText,
    ),
  );
  page.on('response', (response) => {
    if (response.status() >= 400)
      console.error(
        'Fixture response:',
        response.status(),
        new URL(response.url()).pathname,
      );
  });
  page.on('request', async (req) => {
    const url = new URL(req.url());
    if (url.origin !== base) return req.abort();
    if (url.pathname.startsWith('/i/')) {
      visits.push(url.pathname);
      return req.respond({
        status: 302,
        headers: { location: '/register?aff=fixture7' },
      });
    }
    if (!url.pathname.startsWith('/api/')) return req.continue();
    requests.push(url.pathname + url.search);
    if (req.method() === 'POST')
      writes.push({
        path: url.pathname,
        body: JSON.parse(req.postData() || '{}'),
      });
    let body = { success: true, data: [] };
    if (url.pathname === '/api/admin/invite/tier/settings')
      body.data = {
        enabled: false,
        require_identity_for_payout: false,
        pool_ratio: 0.16,
        first_level_rates: [0.05, 0.07, 0.1, 0.11, 0.125, 0.13],
        second_level_matrix: [
          [0.03, 0.03, 0.03, 0.03],
          [0.04, 0.04, 0.04, 0.04],
          [0.04, 0.04, 0.04, 0.04],
          [0.07, 0.06, 0.05, 0.04],
          [0.07, 0.05, 0.04, 0.02],
          [0.1, 0.08, 0.06, 0.04],
        ],
        leader_slot_rates: [0.03, 0.03, 0.02, 0.02],
        qualified_bonus_rate: 0.01,
        department_commission_rate: 0.035,
        qualified_threshold_money: 50,
        second_level_monthly_cap_money: 200,
        commissioner_retention: 30,
        register_rewards: [0.2, 1, 1, 2, 2, 2],
        min_withdrawal_amounts: [5000, 5000, 5000, 100, 100, 100],
        promotion_thresholds: [10, 30],
      };
    if (
      url.pathname.startsWith('/api/admin/invite/') &&
      !url.pathname.endsWith('settings')
    )
      body.data = { items: [], total: 0 };
    if (url.pathname === '/api/invite/tier/status')
      body.data = {
        enabled: true,
        user_id: 7,
        tier: 3,
        first_rate: 0.11,
        qualified_count: 30,
        next_tier_remaining: 0,
        minimum_withdrawal_cents: 100,
        identity_status: 'missing',
        require_identity: true,
        department_id: 0,
        debt_amount: 0,
      };
    if (
      url.pathname === '/api/invite/departments' ||
      url.pathname === '/api/invite/department/apply' ||
      url.pathname === '/api/invite/rebind'
    )
      body.data = { items: [], total: 0 };
    if (url.pathname === '/api/invite/commission/statements') {
      const p = Number(url.searchParams.get('p') || 1);
      body = requestFail
        ? { success: false, message: 'fixture unavailable' }
        : {
            success: true,
            data: {
              total: listEmpty ? 0 : 25,
              items: listEmpty
                ? []
                : Array.from(
                    { length: Math.min(10, 25 - (p - 1) * 10) },
                    (_, i) => ({
                      id: (p - 1) * 10 + i + 1,
                      period_key: `2026-${(p - 1) * 10 + i + 1}`,
                      status: 2,
                      due_amount: 10000,
                      paid_amount: 0,
                      deducted_amount: 0,
                      carry_over_amount: 0,
                    }),
                  ),
            },
          };
    }
    if (url.pathname === '/api/invite/commission/ledgers')
      body.data = {
        total: 1,
        items: [
          {
            id: 1,
            topup_trade_no: 'pay***',
            commission_level: 1,
            status: 'pending',
            base_quota: 100000,
            commission_quota: 11000,
            excluded_base_quota: 1000,
            payout_at: 1796000000,
          },
        ],
      };

    switch (url.pathname) {
      case '/api/status':
        body.data = {
          ...status,
          github_oauth: oauth,
          github_client_id: oauth ? 'fixture' : '',
        };
        break;
      case '/api/user/self':
        body.data = user;
        break;
      case '/api/user/register':
        registrations.push(JSON.parse(req.postData()));
        body = { success: true };
        break;
      case '/api/user/aff':
        body.data = 'fixture7';
        break;
      case '/api/user/invite-promotion/self':
        body = failStats
          ? { success: false, message: 'unavailable' }
          : { success: true, data: stats };
        break;
      case '/api/user/topup/info':
        body.data = {
          enable_online_topup: false,
          min_topup: 1,
          pay_methods: [],
        };
        break;
      case '/api/subscription/self':
        body.data = { subscriptions: [], active_subscriptions: [] };
        break;
    }
    await req.respond({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(body),
    });
  });
}
async function click(page, text) {
  await page.waitForFunction(
    (text) =>
      [...document.querySelectorAll('button')].some(
        (b) => b.textContent.trim() === text && !b.disabled,
      ),
    {},
    text,
  );
  await page.evaluate(
    (text) =>
      [...document.querySelectorAll('button')]
        .filter((b) => b.textContent.trim() === text && !b.disabled)
        .at(-1)
        .click(),
    text,
  );
}

try {
  for (const mobile of [false, true]) {
    const page = await browser.newPage();
    await page.setViewport({
      width: mobile ? 390 : 1440,
      height: mobile ? 844 : 1000,
    });
    await setup(page, true, mobile);
    await page.goto(base + '/console/topup', { waitUntil: 'networkidle0' });
    await page.waitForFunction(() =>
      document.body.innerText.includes('宣传专员'),
    );
    await click(page, '推广者申请与实名');
    await page.waitForFunction(() =>
      document.body.innerText.includes('实名认证'),
    );
    await page
      .locator('.semi-modal button[aria-label="Close"]')
      .click()
      .catch(async () => {
        await page.keyboard.press('Escape');
      });
    await click(page, '申请提前发放');
    await page.waitForFunction(() => document.body.innerText.includes('每周'));
    await page.evaluate(() => {
      for (const b of document.querySelectorAll('.semi-modal-close')) b.click();
    });
    await page.waitForFunction(
      () =>
        !document.querySelector(
          '.semi-modal-wrap:not([style*="display: none"])',
        ),
    );
    // Use existing invitation entry to open statement and ledger tabs.
    await page.click(
      '[title="\u67e5\u770b\u9080\u8bf7\u8fd4\u4f63\u660e\u7ec6"]',
    );
    await page.waitForFunction(() =>
      document.body.innerText.includes('周期账单与明细'),
    );
    await page.waitForFunction(() =>
      document.body.innerText.includes('2026-1'),
    );
    await page.evaluate(() => {
      const b = document.querySelector('.semi-modal [aria-label="Next"]');
      if (b) b.click();
    });
    await page.waitForFunction(() =>
      document.body.innerText.includes('2026-11'),
    );
    await click(page, '查看明细');
    await page.waitForFunction(() =>
      document.body.innerText.includes('pay***'),
    );
    assert.ok(
      requests.some(
        (x) => x.includes('ledgers?') && x.includes('statement_id='),
      ),
    );
    await page.screenshot({
      path: `/tmp/meta-invite-tier-${mobile ? 'mobile' : 'desktop'}.png`,
      fullPage: true,
    });
    listEmpty = true;
    await page.evaluate(() =>
      [...document.querySelectorAll('[role="tab"]')]
        .find((e) => e.textContent.trim() === '周期账单')
        ?.click(),
    );
    await page.waitForFunction(() =>
      document.body.innerText.includes('暂无数据'),
    );
    requestFail = true;
    await click(page, '刷新');
    await page.waitForFunction(() =>
      document.body.innerText.includes('fixture unavailable'),
    );
    requestFail = false;
    listEmpty = false;
    await click(page, '刷新');
    await page.waitForFunction(() =>
      document.body.innerText.includes('2026-1'),
    );
    await page.keyboard.press('Escape');
    await page.close();
  }

  user.role = 100;
  const admin = await browser.newPage();
  await admin.setViewport({ width: 1440, height: 1000 });
  await setup(admin, true, false);
  await admin.goto(base + '/console/setting?tab=operation', {
    waitUntil: 'networkidle0',
  });
  await admin.waitForFunction(() =>
    document.body.innerText.includes('邀请档位与结算'),
  );
  await admin.evaluate(() =>
    [...document.querySelectorAll('.semi-card')]
      .find((e) => e.innerText.includes('邀请档位与结算'))
      ?.scrollIntoView(),
  );
  await admin.screenshot({
    path: '/tmp/meta-invite-tier-admin.png',
    fullPage: true,
  });
  await admin.evaluate(() => {
    [...document.querySelectorAll('[role="tab"]')]
      .find((e) => e.textContent.includes('实名审核'))
      ?.click();
  });
  await admin.waitForFunction(() =>
    document.body.innerText.includes('证件号码'),
  );
  await admin.close();
  assert.deepEqual(errors, []);
  console.log(
    'invite tier desktop/mobile, pagination, statement drilldown, identity and early payout dialogs passed',
  );
} catch (e) {
  console.error(errors);
  for (const p of await browser.pages()) {
    console.log(await p.evaluate(() => document.body.innerText));
    await p.screenshot({ path: '/tmp/invite-browser-failure.png' });
  }
  throw e;
} finally {
  await browser.close();
}
