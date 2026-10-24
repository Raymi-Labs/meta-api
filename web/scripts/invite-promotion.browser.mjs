import assert from 'node:assert/strict';
import puppeteer from 'puppeteer-core';
import { buildInvitePromotionHTML } from '../src/helpers/invitePromotion.js';
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
    let body = { success: true, data: [] };
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
    await setup(page, false, mobile);
    await page.goto(base + '/register?aff=fixture7', {
      waitUntil: 'networkidle0',
    });
    await page.waitForSelector('section[aria-label="元衡优势"]');
    await page.waitForFunction(
      () =>
        document.querySelector('.invite-benefits__character img')
          ?.naturalWidth === 480,
    );
    assert.equal(
      await page.evaluate(() =>
        document.documentElement.classList.contains('dark'),
      ),
      mobile,
    );
    assert.equal(
      await page.evaluate(() => localStorage.getItem('aff')),
      'fixture7',
    );
    assert.ok(await page.$('input[name="username"]'));
    assert.ok(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
    );
    await page.click('a[href="#invite-register-form"]');
    assert.ok(
      await page.evaluate(() => {
        const r = document
          .querySelector('#invite-register-form')
          .getBoundingClientRect();
        return r.top < innerHeight && r.bottom > 0;
      }),
    );
    await page.waitForFunction(
      () =>
        Number(
          getComputedStyle(
            document.querySelector('.invite-benefits__character'),
          ).opacity,
        ) >= 0.99,
    );
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.screenshot({
      path: `/tmp/newapi-invite-${mobile ? 'mobile' : 'desktop'}.png`,
      fullPage: true,
    });
    if (!mobile) {
      await page.type('input[name="username"]', 'invite-user');
      await page.type('input[name="password"]', 'testpassword123');
      await page.type('input[name="password2"]', 'testpassword123');
      await click(page, '注册');
      await page.waitForFunction(() => location.pathname === '/login');
      assert.equal(registrations.at(-1)?.aff_code, 'fixture7');
    }
    await page.goto(base + '/register', { waitUntil: 'networkidle0' });
    assert.equal(await page.$('section[aria-label="元衡优势"]'), null);
    await page.close();
  }
  for (const [width, language] of [
    [320, 'zh-CN'],
    [390, 'fr'],
    [1024, 'zh-CN'],
    [768, 'zh-CN'],
    [1440, 'en'],
  ]) {
    const page = await browser.newPage();
    await page.setViewport({ width, height: 900 });
    await setup(page);
    await page.evaluateOnNewDocument((language) => {
      if (window === window.top) localStorage.setItem('i18nextLng', language);
    }, language);
    await page.goto(base + '/register?aff=fixture7', {
      waitUntil: 'networkidle0',
    });
    await page.waitForSelector('.invite-benefits__models');
    assert.ok(
      await page.evaluate(() => {
        const el = document.querySelector('.invite-benefits');
        return (
          el.getBoundingClientRect().right <= innerWidth &&
          [...el.children]
            .filter(
              (child) => !child.classList.contains('invite-benefits__orbit'),
            )
            .every(
              (child) =>
                child.getBoundingClientRect().right <=
                el.getBoundingClientRect().right,
            )
        );
      }),
    );
    await page.close();
  }
  const motionPage = await browser.newPage();
  await setup(motionPage);
  await motionPage.emulateMediaFeatures([
    { name: 'prefers-reduced-motion', value: 'reduce' },
  ]);
  await motionPage.goto(base + '/register?aff=fixture7', {
    waitUntil: 'networkidle0',
  });
  assert.ok(
    await motionPage.evaluate(() =>
      [
        ...document.querySelectorAll(
          '.invite-benefits__crystal,.invite-benefits__character',
        ),
      ].every((el) => getComputedStyle(el).animationName === 'none'),
    ),
  );
  await motionPage.emulateMediaFeatures([
    { name: 'prefers-reduced-motion', value: 'no-preference' },
  ]);
  assert.ok(
    await motionPage.evaluate(() =>
      [...document.querySelectorAll('.invite-benefits *')].every(
        (el) => getComputedStyle(el).animationIterationCount !== 'infinite',
      ),
    ),
  );
  await motionPage.close();
  oauth = true;
  const p = await browser.newPage();
  await setup(p);
  await p.goto(base + '/register?aff=fixture7', { waitUntil: 'networkidle0' });
  await p.waitForSelector('section[aria-label="元衡优势"]');
  assert.ok(
    (await p.evaluate(() => document.body.innerText)).includes('GitHub'),
  );
  await p.close();
  oauth = false;
  for (const mobile of [false, true]) {
    const page = await browser.newPage();
    await page.setViewport({
      width: mobile ? 390 : 1440,
      height: mobile ? 844 : 1000,
    });
    await setup(page, true, mobile);
    await page.goto(base + '/console/topup', { waitUntil: 'networkidle0' });
    await click(page, '推广素材与点击统计');
    await page.waitForFunction(() =>
      document
        .querySelector('[aria-label="推广点击统计"]')
        ?.innerText.includes('42'),
    );
    const iframe = await page.waitForSelector('iframe[title="广告预览"]');
    assert.equal(await iframe.evaluate((el) => el.getAttribute('sandbox')), '');
    const frame = await iframe.contentFrame();
    assert.ok(await frame.$('a[href*="src=banner"]'));
    await frame.waitForFunction(
      () => document.querySelector('img')?.naturalWidth === 1536,
    );
    assert.ok(
      await frame.evaluate(() => {
        const im = document.querySelector('img');
        return (
          new URL(im.src).pathname ===
            '/images/yuanheng-invite-character.jpg' &&
          !new URL(im.src).search &&
          im.referrerPolicy === 'no-referrer'
        );
      }),
    );
    await frame.click('a');
    assert.deepEqual(visits, []);
    for (const [label, source] of [
      ['文字链接', 'text'],
      ['推广按钮', 'button'],
      ['横幅卡片', 'banner'],
    ]) {
      await page.evaluate(
        (label) =>
          [...document.querySelectorAll('.semi-radio')]
            .find((e) => e.textContent.includes(label))
            .click(),
        label,
      );
      await page.waitForFunction(
        (source) =>
          document
            .querySelector('textarea[aria-label="HTML 广告代码"]')
            ?.value.includes(`src=${source}`),
        {},
        source,
      );
    }
    await click(page, '复制 HTML 代码');
    assert.ok(
      await page.evaluate(() => window.__copiedHTML?.includes('src=banner')),
    );
    await page.evaluate(() =>
      document.querySelector('.semi-modal-body').scrollTo(0, 0),
    );
    await page.screenshot({
      path: `/tmp/newapi-invite-materials-${mobile ? 'mobile' : 'desktop'}.png`,
      fullPage: true,
    });
    failStats = true;
    await click(page, '刷新');
    await page.waitForSelector('[role="alert"]');
    assert.ok(
      (await page.evaluate(() => document.body.innerText)).includes(
        '点击统计加载失败',
      ),
    );
    failStats = false;
    await click(page, '刷新');
    await page.waitForFunction(() =>
      document
        .querySelector('[aria-label="推广点击统计"]')
        ?.innerText.includes('42'),
    );
    assert.ok(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
    );
    await page.close();
  }
  for (const width of [320, 680]) {
    const page = await browser.newPage();
    await page.setViewport({ width, height: 650 });
    await setup(page);
    await page.setContent(
      buildInvitePromotionHTML(base + '/i/fixture7', 'banner'),
    );
    await page.waitForFunction(
      () => document.querySelector('img')?.naturalWidth === 1536,
    );
    assert.ok(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    );
    await page.screenshot({
      path: `/tmp/newapi-invite-banner-${width}.png`,
      fullPage: true,
    });
    await page.evaluate(() => {
      document.querySelector('img').src = '/missing-character-fixture.jpg';
    });
    assert.ok(
      await page.evaluate(
        () =>
          document.querySelector('a').innerText.includes('CC Max') &&
          document.querySelector('a').href.includes('src=banner'),
      ),
    );
    await page.close();
  }
  assert.deepEqual(errors, []);
  console.log(
    'Invitation UI: desktop/mobile, email attribution, OAuth display, normal registration, HTML sandbox, variants, stats/error/retry passed.',
  );
} catch (error) {
  console.log('Page errors:', errors);
  const page = (await browser.pages()).at(-1);
  await page?.screenshot({
    path: '/tmp/newapi-invite-error.png',
    fullPage: true,
  });
  throw error;
} finally {
  await browser.close();
}
