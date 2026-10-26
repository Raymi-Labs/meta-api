import assert from 'node:assert/strict';
import puppeteer from 'puppeteer-core';

const base = process.env.ANNOUNCEMENT_TEST_URL || 'http://127.0.0.1:5193';
const browser = await puppeteer.launch({
  executablePath:
    process.env.CHROME_PATH ||
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  headless: true,
});
const scope = '[data-testid="announcement-settings"]';
const history = '[data-testid="announcement-email-history"]';

try {
  for (const mobile of [false, true]) {
    const page = await browser.newPage();
    page.setDefaultTimeout(20000);
    const errors = [];
    const writes = [];
    const reads = [];
    let failSave = true;
    let failHistory = false;
    let holdNextOptions = false;
    let holdNextSave = false;
    let heldOptions;
    let heldSave;
    let resolveHeldOptions;
    let resolveHeldSave;
    const heldOptionsReady = new Promise((resolve) => {
      resolveHeldOptions = resolve;
    });
    const heldSaveReady = new Promise((resolve) => {
      resolveHeldSave = resolve;
    });
    let savedItems = [
      {
        id: 1,
        content: 'Existing announcement',
        publishDate: '2026-10-25T10:00:00Z',
        type: 'default',
        extra: '',
      },
    ];
    const user = {
      id: 7,
      username: 'announcement-fixture',
      role: 100,
      status: 1,
      group: 'default',
      setting: '{}',
      quota: 1000000,
      used_quota: 0,
      request_count: 0,
    };
    await page.setViewport({ width: mobile ? 390 : 1440, height: 1100 });
    page.on('pageerror', (error) => errors.push(error.message));
    await page.evaluateOnNewDocument(
      (user, mobile) => {
        localStorage.setItem('user', JSON.stringify(user));
        localStorage.setItem('i18nextLng', 'zh-CN');
        localStorage.setItem('site-language', 'zh-CN');
        localStorage.setItem('theme-mode', mobile ? 'dark' : 'light');
        localStorage.setItem(
          'newapi:user-onboarding:regular:v1:user:' + user.id,
          'done',
        );
      },
      user,
      mobile,
    );
    await page.setRequestInterception(true);
    page.on('request', async (request) => {
      const url = new URL(request.url());
      if (url.origin !== new URL(base).origin) return request.abort();
      if (!url.pathname.startsWith('/api/')) return request.continue();
      let body = { success: true, data: [] };
      if (url.pathname === '/api/status')
        body = {
          success: true,
          data: {
            system_name: 'New API',
            logo: '/logo.png',
            chats: '[]',
            quota_per_unit: 500000,
            quota_display_type: 'USD',
            price: 1,
            usd_exchange_rate: 1,
          },
        };
      if (url.pathname === '/api/user/self')
        body = { success: true, data: user };
      if (url.pathname === '/api/option/') {
        if (request.method() === 'PUT') {
          const payload = JSON.parse(request.postData());
          if (payload.key === 'console_setting.announcements') {
            writes.push(payload);
            if (failSave)
              body = { success: false, message: 'Fixture SMTP unavailable' };
            else {
              savedItems = JSON.parse(payload.value);
              body = {
                success: true,
                data: { email_jobs_queued: payload.email_sync ? 1 : 0 },
              };
            }
          } else body = { success: true };
        } else
          body = {
            success: true,
            data: [
              {
                key: 'console_setting.announcements',
                value: JSON.stringify(savedItems),
              },
              { key: 'console_setting.announcements_enabled', value: 'true' },
              { key: 'console_setting.api_info', value: '[]' },
              { key: 'console_setting.faq', value: '[]' },
              { key: 'console_setting.uptime_kuma_groups', value: '[]' },
            ],
          };
      }
      if (url.pathname === '/api/option/announcement-email/jobs') {
        const p = Number(url.searchParams.get('p') || 1);
        reads.push(p);
        body = failHistory
          ? { success: false, message: 'Fixture history unavailable' }
          : {
              success: true,
              data: {
                page: p,
                page_size: 10,
                total: 11,
                items: [
                  {
                    id: p,
                    content:
                      p === 1
                        ? 'Latest email job <img src=x onerror=alert(1)>'
                        : 'Earlier email job',
                    created_at: 1793000000,
                    total: 7,
                    pending: 0,
                    sending: 0,
                    submitted: 4,
                    failed: 1,
                    unknown: 1,
                    skipped: 1,
                  },
                ],
              },
            };
      }
      if (
        url.pathname === '/api/option/' &&
        request.method() === 'GET' &&
        holdNextOptions
      ) {
        holdNextOptions = false;
        heldOptions = { request, body };
        resolveHeldOptions();
        return;
      }
      if (
        url.pathname === '/api/option/' &&
        request.method() === 'PUT' &&
        holdNextSave &&
        JSON.parse(request.postData()).key === 'console_setting.announcements'
      ) {
        holdNextSave = false;
        heldSave = { request, body };
        resolveHeldSave();
        return;
      }
      await request.respond({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(body),
      });
    });
    const clickText = async (selector, text) => {
      await page.evaluate(
        (selector, text) => {
          const button = [
            ...document.querySelectorAll(selector + ' button'),
          ].find((element) => element.textContent.trim() === text);
          if (!button) throw new Error('Missing button: ' + text);
          button.click();
        },
        selector,
        text,
      );
    };
    const addAnnouncement = async (text) => {
      await clickText(scope, '添加公告');
      await page.waitForSelector('.semi-modal-content textarea', {
        visible: true,
      });
      await page.type('.semi-modal-content textarea', text);
      await clickText('.semi-modal-content', '保存');
      await page.waitForSelector('.semi-modal-content', { hidden: true });
      await page.waitForFunction(
        (scope, text) =>
          document.querySelector(scope).textContent.includes(text),
        {},
        scope,
        text,
      );
    };
    const saveDisabled = () =>
      page.$eval(
        scope,
        (element) =>
          [...element.querySelectorAll('button')].find(
            (button) => button.textContent.trim() === '保存设置',
          ).disabled,
      );
    await page.goto(base + '/console/setting?tab=dashboard', {
      waitUntil: 'networkidle0',
    });
    await page.waitForSelector(history);
    assert.equal(
      await page.$eval('#announcement-email-sync', (element) =>
        element.matches('input')
          ? element.checked
          : element.querySelector('input').checked,
      ),
      true,
    );
    assert.equal(
      await page.$(history + ' img'),
      null,
      'job content must render as text',
    );
    await addAnnouncement('New maintenance draft');
    await clickText(scope, '保存设置');
    await page.waitForFunction(() =>
      document.body.textContent.includes('Fixture SMTP unavailable'),
    );
    assert.equal(
      await saveDisabled(),
      false,
      'server failure must leave save enabled',
    );
    assert.equal(writes[0].email_sync, true);
    // Toggling the existing panel switch refreshes the parent options. Preserve the failed draft.
    await page.$eval(scope + ' [role="switch"]', (element) => element.click());
    await page.waitForNetworkIdle();
    assert(
      (await page.$eval(scope, (element) => element.textContent)).includes(
        'New maintenance draft',
      ),
    );
    failSave = false;
    await page.click('#announcement-email-sync');
    await page.waitForFunction(
      () => !document.querySelector('#announcement-email-sync input')?.disabled,
    );
    await clickText(scope, '保存设置');
    await page.waitForFunction(
      (scope) =>
        [...document.querySelectorAll(scope + ' button')].some(
          (element) =>
            element.textContent.trim() === '保存设置' && element.disabled,
        ),
      {},
      scope,
    );
    assert.equal(writes.at(-1).email_sync, false);
    assert.equal(savedItems.length, 2);
    assert.match(savedItems[1].id, /^[a-f0-9-]{36}$/);
    await addAnnouncement('Second maintenance draft');
    await page.click('#announcement-email-sync');
    await page.waitForFunction(
      () => !document.querySelector('#announcement-email-sync input')?.disabled,
    );
    // Retain the old options snapshot while a subsequent publication requests
    // fresh options. Return the fresh request first, then the stale request.
    holdNextOptions = true;
    await page.$eval(scope + ' [role="switch"]', (element) => element.click());
    await heldOptionsReady;
    holdNextSave = true;
    await clickText(scope, '保存设置');
    await heldSaveReady;
    await page.waitForFunction(
      (scope) => {
        const control = document.querySelector(scope + ' [role="switch"]');
        return (
          control.disabled || control.getAttribute('aria-disabled') === 'true'
        );
      },
      {},
      scope,
    );
    const freshOptionsResponse = page.waitForResponse(
      (response) =>
        new URL(response.url()).pathname === '/api/option/' &&
        response.request().method() === 'GET',
    );
    await heldSave.request.respond({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(heldSave.body),
    });
    await freshOptionsResponse;
    await page.waitForFunction(() =>
      document.body.textContent.includes(
        '公告已保存，1 条公告正在后台发送邮件',
      ),
    );
    await heldOptions.request.respond({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(heldOptions.body),
    });
    await page.waitForNetworkIdle();
    assert(
      (await page.$eval(scope, (element) => element.textContent)).includes(
        'Second maintenance draft',
      ),
      'late options response must not overwrite the newly published announcement',
    );
    assert.equal(
      await saveDisabled(),
      true,
      'new publication remains saved after late refresh',
    );
    assert.equal(writes.at(-1).email_sync, true);
    assert.equal(savedItems.length, 3);
    await page.$eval(history + ' [aria-label="Next"]', (element) =>
      element.click(),
    );
    await page.waitForFunction(
      (history) =>
        document
          .querySelector(history)
          .textContent.includes('Earlier email job'),
      {},
      history,
    );
    assert(reads.includes(2));
    failHistory = true;
    await clickText(history, '刷新');
    await page.waitForSelector(history + ' [role="alert"]');
    failHistory = false;
    await clickText(history, '刷新');
    await page.waitForFunction(
      (history) =>
        !document.querySelector(history + ' [role="alert"]') &&
        document
          .querySelector(history)
          .textContent.includes('Earlier email job'),
      {},
      history,
    );
    const bounds = await page.$eval(history, (element) => ({
      left: element.getBoundingClientRect().left,
      right: element.getBoundingClientRect().right,
    }));
    assert(
      bounds.left >= 0 && bounds.right <= (mobile ? 390 : 1440) + 1,
      JSON.stringify(bounds),
    );
    if (mobile)
      assert(
        await page.evaluate(
          () =>
            document.body.classList.contains('dark') ||
            document.documentElement.classList.contains('dark') ||
            document.body.getAttribute('theme-mode') === 'dark',
        ),
      );
    await page.$$eval('.semi-toast-close-button button', (buttons) =>
      buttons.forEach((button) => button.click()),
    );
    await page.waitForSelector('.semi-toast', { hidden: true });
    await page.$eval(scope, (element) => element.scrollIntoView());
    await page.screenshot({
      path: `/tmp/newapi-announcement-email-${mobile ? 'mobile-dark' : 'desktop'}.png`,
      fullPage: false,
    });
    if (mobile) {
      const scrolled = await page.$eval(history, (element) => {
        const targets = [...element.querySelectorAll('*')].filter(
          (node) =>
            node.scrollWidth > node.clientWidth &&
            /auto|scroll/.test(getComputedStyle(node).overflowX),
        );
        targets.forEach((node) => {
          node.scrollLeft = node.scrollWidth;
        });
        return targets.length;
      });
      assert(scrolled > 0, 'mobile email history must scroll horizontally');
      await page.waitForFunction(
        (history) => {
          const header = [...document.querySelectorAll(history + ' th')].find(
            (node) => node.textContent.includes('已跳过'),
          );
          return (
            header &&
            header.getBoundingClientRect().right <= innerWidth &&
            header.getBoundingClientRect().left >= 0
          );
        },
        {},
        history,
      );
      await page.$eval(history, (element) =>
        element.scrollIntoView({ block: 'center' }),
      );
      await page.screenshot({
        path: '/tmp/newapi-announcement-email-mobile-dark-status.png',
        fullPage: false,
      });
    }
    assert.deepEqual(errors, []);
    await page.close();
  }
  console.log(
    'Announcement browser checks passed: draft failure/retry, refresh protection and out-of-order responses, opt-in/out, UUIDs, background status, paging, history errors, escaping, mobile/dark. Screenshots: /tmp/newapi-announcement-email-desktop.png, /tmp/newapi-announcement-email-mobile-dark.png',
  );
} finally {
  await browser.close();
}
