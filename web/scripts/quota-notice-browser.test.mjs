import assert from 'node:assert/strict';
import puppeteer from 'puppeteer-core';
const baseURL = process.env.QUOTA_NOTICE_TEST_URL || 'http://127.0.0.1:5186';
const browser = await puppeteer.launch({
  executablePath:
    process.env.CHROME_PATH ||
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  headless: true,
});
try {
  for (const [mobile, admin] of [
    ...(process.env.ADMIN_ONLY
      ? []
      : [
          [false, false],
          [true, false],
        ]),
    [false, true],
  ]) {
    const page = await browser.newPage();
    const errors = [];
    const calls = [];
    let fail = false;
    let saved = null;
    const user = {
      id: 7,
      username: 'quota-fixture',
      display_name: '测试用户',
      role: admin ? 100 : 1,
      status: 1,
      DeletedAt: null,
      group: 'default',
      email: 'fixture@example.invalid',
      quota: 10000,
      used_quota: 0,
      request_count: 0,
      setting: '{}',
    };
    await page.setViewport({ width: mobile ? 390 : 1440, height: 1000 });
    page.on('pageerror', (e) => errors.push(e.message));
    await page.evaluateOnNewDocument(
      (u, dark) => {
        localStorage.setItem('user', JSON.stringify(u));
        localStorage.setItem('i18nextLng', 'zh');
        localStorage.setItem('theme-mode', dark ? 'dark' : 'light');
        localStorage.setItem(
          'newapi:user-onboarding:regular:v1:user:' + u.id,
          'done',
        );
      },
      user,
      mobile,
    );
    await page.setRequestInterception(true);
    page.on('request', async (req) => {
      const url = new URL(req.url());
      if (url.origin !== new URL(baseURL).origin) {
        await req.abort();
        return;
      }
      if (!url.pathname.startsWith('/api/')) {
        await req.continue();
        return;
      }
      calls.push(url.pathname + url.search);
      let body = { success: true, data: [] };
      if (url.pathname === '/api/status')
        body = {
          success: true,
          data: {
            system_name: 'New API',
            logo: '/logo.png',
            quota_per_unit: 500000,
            chats: '[]',
            quota_display_type: 'USD',
            price: 1,
            usd_exchange_rate: 1,
          },
        };
      if (url.pathname === '/api/user/self')
        body = { success: true, data: user };
      if (url.pathname === '/api/user/')
        body = {
          success: true,
          data: { items: [{ ...user, id: 8, role: 1 }], page: 1, total: 1 },
        };
      if (url.pathname === '/api/user/8')
        body = { success: true, data: { ...user, id: 8, role: 1 } };
      if (url.pathname === '/api/group/')
        body = { success: true, data: ['default'] };
      if (url.pathname === '/api/user/setting') {
        saved = JSON.parse(req.postData());
        user.setting = JSON.stringify(saved);
        body = { success: true };
      }
      if (
        url.pathname === '/api/user/quota-notices' ||
        url.pathname === '/api/user/8/quota-notices'
      ) {
        const p = Number(url.searchParams.get('p') || 1);
        body = fail
          ? { success: false, message: 'fixture failure' }
          : {
              success: true,
              data: {
                total: 11,
                page: p,
                items: [
                  {
                    id: p === 1 ? 101 : 91,
                    created_at: 1792800000000,
                    status: p === 1 ? 'submitted' : 'unknown',
                    medium: 'email',
                    recipient: 'fi***@example.invalid',
                    attempts: 1,
                    summary: JSON.stringify([
                      {
                        scope: 'wallet:0',
                        count: 584,
                        token_name: '<img src=x onerror=alert(1)>',
                        model_name: 'fixture',
                        request_id: 'request-fixture',
                      },
                    ]),
                  },
                ],
              },
            };
      }
      await req.respond({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(body),
      });
    });
    await page.goto(baseURL + (admin ? '/console/user' : '/console/personal'), {
      waitUntil: 'networkidle0',
    });
    if (admin) {
      await page.waitForFunction(() =>
        Array.from(document.querySelectorAll('button')).some(
          (e) => e.textContent.trim() === '编辑',
        ),
      );
      await page.evaluate(() =>
        Array.from(document.querySelectorAll('button'))
          .find((e) => e.textContent.trim() === '编辑')
          .click(),
      );
      await page.waitForSelector('[data-testid="quota-notice-history"]');
      await page.waitForFunction(() =>
        document
          .querySelector('[data-testid="quota-notice-history"]')
          .textContent.includes('#101'),
      );
      assert(calls.some((p) => p.startsWith('/api/user/8/quota-notices')));
      await page.waitForFunction(
        () =>
          document
            .querySelector('[data-testid="quota-notice-history"]')
            .getBoundingClientRect().right <= window.innerWidth,
      );
      await page.screenshot({
        path: '/tmp/newapi-quota-notice-admin.png',
        fullPage: true,
      });
      assert.deepEqual(errors, []);
      await page.close();
      continue;
    }
    const card = '[data-testid="quota-notice-history"]';
    await page.waitForSelector(card);
    assert((await page.$eval(card, (e) => e.textContent)).includes('#101'));
    assert(
      (await page.$eval(card, (e) => e.textContent)).includes('已提交通知服务'),
    );
    assert.equal(await page.$(card + ' img'), null);
    const switchSelector = '[id="quotaBlockedNotifyEnabled"]';
    await page.waitForSelector(switchSelector);
    assert.equal(
      await page.$eval(switchSelector, (e) => e.getAttribute('aria-checked')),
      'true',
    );
    await page.click(switchSelector);
    await page.evaluate(() =>
      Array.from(document.querySelectorAll('button'))
        .find((e) => e.textContent.trim() === '保存设置')
        .click(),
    );
    await page.waitForFunction(() =>
      document.body.textContent.includes('设置保存成功'),
    );
    assert.equal(saved.quota_blocked_notify_enabled, false);
    await page.reload({ waitUntil: 'networkidle0' });
    assert.equal(
      await page.$eval(switchSelector, (e) => e.getAttribute('aria-checked')),
      'false',
    );
    await page.$eval(card + ' [aria-label="Next"]', (e) => e.click());
    await page.waitForFunction(
      (sel) => document.querySelector(sel).textContent.includes('#91'),
      {},
      card,
    );
    assert(
      (await page.$eval(card, (e) => e.textContent)).includes('发送结果未知'),
    );
    fail = true;
    await page.$eval(card + ' button', (e) => e.click());
    await page.waitForSelector(card + ' [role="alert"]');
    fail = false;
    await page.$eval(card + ' button', (e) => e.click());
    await page.waitForFunction(
      (sel) => !document.querySelector(sel + ' [role="alert"]'),
      {},
      card,
    );
    assert(!calls.some((p) => /\/api\/user\/\d+\/quota-notices/.test(p)));
    const bounds = await page.$eval(card, (e) => ({
      left: e.getBoundingClientRect().left,
      right: e.getBoundingClientRect().right,
    }));
    assert(bounds.left >= 0 && bounds.right <= (mobile ? 390 : 1440) + 1);
    await page.screenshot({
      path: `/tmp/newapi-quota-notice-${mobile ? 'mobile-dark' : 'desktop'}.png`,
      fullPage: true,
    });
    assert.deepEqual(errors, []);
    await page.close();
  }
  console.log(
    'quota notices: settings persistence, audit pagination, error recovery, escaping, mobile/dark and self-only requests passed',
  );
} finally {
  await browser.close();
}
