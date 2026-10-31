import assert from 'node:assert/strict';
import puppeteer from 'puppeteer-core';
const base = process.env.INVITE_TEST_URL || 'http://127.0.0.1:5191';
const browser = await puppeteer.launch({
  executablePath:
    process.env.CHROME_PATH ||
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  headless: true,
});
try {
  for (const width of [1440, 390]) {
    const page = await browser.newPage();
    page.setDefaultTimeout(20000);
    await page.setViewport({ width, height: 1000 });
    const errors = [],
      mutations = [],
      queries = [];
    let walletCalls = 0,
      rewards = 0,
      fail = false;
    const user = {
      id: 7,
      username: 'realtime-test',
      role: 1,
      group: 'default',
      quota: 50000000,
      transferable_quota: 10000000,
      aff_quota: 0,
      aff_history_quota: 0,
      aff_count: 22,
    };
    const status = {
      system_name: 'New API',
      chats: '[]',
      price: 1,
      usd_exchange_rate: 1,
      quota_per_unit: 500000,
      quota_display_type: 'USD',
    };
    page.on('pageerror', (e) => errors.push(e.message));
    await page.evaluateOnNewDocument(
      (user, status) => {
        for (const [k, v] of Object.entries({
          user: JSON.stringify(user),
          status: JSON.stringify(status),
          'site-language': 'zh-CN',
          i18nextLng: 'zh-CN',
          quota_per_unit: '500000',
          'newapi:user-onboarding:regular:v1:user:7': 'done',
        }))
          localStorage.setItem(k, v);
        const intervals = new Map(),
          set = window.setInterval.bind(window),
          clear = window.clearInterval.bind(window);
        window.__refreshIntervals = intervals;
        window.__visible = true;
        Object.defineProperty(document, 'visibilityState', {
          get: () => (window.__visible ? 'visible' : 'hidden'),
        });
        window.setInterval = (fn, ms, ...args) => {
          if (ms !== 15000) return set(fn, ms, ...args);
          const id = set(() => {}, 3600000);
          intervals.set(id, fn);
          return id;
        };
        window.clearInterval = (id) => {
          intervals.delete(id);
          clear(id);
        };
        window.__refresh = () =>
          Promise.all([...intervals.values()].map((fn) => fn()));
      },
      user,
      status,
    );
    await page.setRequestInterception(true);
    page.on('request', async (req) => {
      const url = new URL(req.url());
      if (url.origin !== base) return req.abort();
      if (!url.pathname.startsWith('/api/')) return req.continue();
      if (req.method() !== 'GET' && !url.pathname.endsWith('/amount'))
        mutations.push(url.pathname);
      let body = { success: true, data: [] };
      switch (url.pathname) {
        case '/api/status':
          body.data = status;
          break;
        case '/api/user/self':
          walletCalls++;
          body = fail
            ? { success: false, message: 'fixture failure' }
            : {
                success: true,
                data: {
                  ...user,
                  aff_quota: rewards * 500000,
                  aff_history_quota: rewards * 500000,
                },
              };
          break;
        case '/api/user/topup/info':
          body.data = {
            enable_online_topup: true,
            min_topup: 10,
            amount_options: [10, 30, 100],
            discount: {},
            pay_methods: [{ type: 'alipay', name: '支付宝' }],
          };
          break;
        case '/api/user/amount':
          body = { message: 'success', data: '10.00', currency: 'CNY' };
          break;
        case '/api/subscription/self':
          body.data = { subscriptions: [], active_subscriptions: [] };
          break;
        case '/api/user/aff':
          body.data = 'fixture';
          break;
        case '/api/user/invite-recharge-commissions/self': {
          const p = Number(url.searchParams.get('p') || 1);
          queries.push(p);
          body = fail
            ? { success: false, message: 'fixture failure' }
            : {
                success: true,
                data: {
                  summary: { recharge_total_quota: rewards * 500000 },
                  invitees: {
                    total: 22,
                    items: [
                      {
                        alias: `匿名用户-${p}`,
                        registered_date: '2026-10-31',
                        commission_level: 1,
                        recharge_total_money: 100,
                        recharge_commission_quota: rewards * 500000,
                      },
                    ],
                  },
                },
              };
          break;
        }
      }
      await req.respond({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(body),
      });
    });
    await page.goto(base + '/console/topup', { waitUntil: 'networkidle0' });
    await page.waitForSelector('[title="查看邀请返佣明细"]');
    await page.click('[title="查看邀请返佣明细"]');
    await page.waitForFunction(() =>
      document.body.innerText.includes('匿名用户-1'),
    );
    assert(
      !(await page.evaluate(() =>
        document.body.innerText.includes('每天 24 点'),
      )),
    );
    rewards = 12;
    await page.evaluate(() => window.__refresh());
    await page.waitForFunction(() =>
      document
        .querySelector('.semi-modal-content')
        ?.innerText.includes('12.00'),
    );
    assert(
      await page.evaluate(() =>
        Array.from(document.querySelectorAll('button')).some(
          (b) => b.textContent.includes('划转到余额') && !b.disabled,
        ),
      ),
    );
    await page.evaluate(() => {
      const e = [...document.querySelectorAll('.semi-page-item')].find(
        (e) => e.textContent.trim() === '2',
      );
      if (!e) throw Error('missing page 2');
      e.click();
    });
    await page.waitForFunction(() =>
      document.body.innerText.includes('匿名用户-2'),
    );
    await page.evaluate(() => window.__refresh());
    assert.equal(queries.at(-1), 2);
    const before = [walletCalls, queries.length];
    await page.evaluate(async () => {
      window.__visible = false;
      document.dispatchEvent(new Event('visibilitychange'));
      await window.__refresh();
    });
    assert.deepEqual([walletCalls, queries.length], before);
    rewards = 18;
    await page.evaluate(() => {
      window.__visible = true;
      document.dispatchEvent(new Event('visibilitychange'));
    });
    await page.waitForFunction(() =>
      document
        .querySelector('.semi-modal-content')
        ?.innerText.includes('18.00'),
    );
    fail = true;
    await page.evaluate(() => window.__refresh());
    await page.waitForFunction(() =>
      document
        .querySelector('.semi-modal-content')
        ?.innerText.includes('加载失败'),
    );
    assert(
      await page.evaluate(() =>
        document
          .querySelector('.semi-modal-content')
          .innerText.includes('18.00'),
      ),
    );
    fail = false;
    await page.evaluate(() => window.__refresh());
    await page.waitForFunction(
      () =>
        !document
          .querySelector('.semi-modal-content')
          ?.innerText.includes('加载失败'),
    );
    const modalBounds = await page.$eval('.semi-modal-content', (e) => {
      const b = e.getBoundingClientRect();
      return { left: b.left, right: b.right };
    });
    assert(
      modalBounds.left >= 0 && modalBounds.right <= width,
      JSON.stringify(modalBounds),
    );
    await page.screenshot({
      path: `/tmp/invite-realtime-${width}.png`,
      fullPage: true,
    });
    await page.click('.semi-modal-close');
    await page.waitForFunction(() => window.__refreshIntervals.size === 1);
    const closed = queries.length;
    await page.evaluate(() => window.__refresh());
    assert.equal(queries.length, closed);
    assert.deepEqual(mutations, []);
    assert.deepEqual(errors, []);
    await page.close();
    console.log(
      `PASS ${width}: wallet + details refresh, visibility, pagination, failure retention, disposal`,
    );
  }
} finally {
  await browser.close();
}
