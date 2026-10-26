import assert from 'node:assert/strict';
import puppeteer from 'puppeteer-core';

// Run after `bun run build` and `bunx vite preview --host 127.0.0.1 --port 5193`.
// All account/API responses are isolated fixtures; no production submission.
const base = process.env.DESKTOP_FEEDBACK_TEST_URL || 'http://127.0.0.1:5193';
const browser = await puppeteer.launch({
  executablePath:
    process.env.CHROME_PATH ||
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  headless: true,
});
const pageErrors = [];
const queries = [];
const patches = [];
let failList = false;
let failSave = false;
let row = {
  id: 11,
  userId: 7,
  reportId: 'a7149591-4139-4c52-a0a2-1fa4ae45958b',
  description: '模型切换失败 <script>window.__feedbackInjected=true</script>',
  appVersion: '0.1.108',
  platform: 'macos',
  context: { view: 'home', tool: 'dsh' },
  errors: [
    {
      occurredAt: '2026-10-26T01:00:00Z',
      source: 'toast',
      message: 'error sending request for https://meta-api.vip/api/user/models',
    },
  ],
  status: 'pending',
  createdAt: 1792976400,
  updatedAt: 1792976400,
};
const status = {
  system_name: '元衡',
  footer_html: '',
  logo: '/logo.png',
  chats: '[]',
  server_address: base,
  quota_per_unit: 500000,
  price: 1,
  usd_exchange_rate: 5,
  quota_display_type: 'USD',
  setup: true,
};

async function fixture(role = 10, width = 1280) {
  const page = await browser.newPage();
  await page.setViewport({ width, height: 900 });
  page.on('pageerror', (error) => pageErrors.push(error.message));
  page.setDefaultTimeout(12000);
  await page.evaluateOnNewDocument(
    (status, role) => {
      localStorage.setItem('site-language', 'zh-CN');
      localStorage.setItem('i18nextLng', 'zh-CN');
      localStorage.setItem('status', JSON.stringify(status));
      localStorage.setItem(
        'user',
        JSON.stringify({
          id: 7,
          username: 'feedback-fixture',
          role,
          group: 'default',
        }),
      );
      localStorage.setItem('newapi:user-onboarding:regular:v1:user:7', 'done');
      localStorage.setItem('newapi:user-onboarding:admin:v1:user:7', 'done');
      localStorage.setItem('theme-mode', 'light');
    },
    status,
    role,
  );
  await page.setRequestInterception(true);
  page.on('request', async (request) => {
    const url = new URL(request.url());
    if (url.origin !== base) return request.abort();
    if (!url.pathname.startsWith('/api/')) return request.continue();
    let result = { success: true, data: {} };
    if (url.pathname === '/api/status')
      result = { success: true, data: status };
    else if (url.pathname === '/api/user/self')
      result.data = {
        id: 7,
        username: 'feedback-fixture',
        role,
        group: 'default',
        permissions: {},
        setting: '{}',
      };
    else if (url.pathname === '/api/notice') result.data = '';
    else if (url.pathname === '/api/desktop-feedback/') {
      queries.push(url.search);
      result = failList
        ? { success: false, message: 'fixture list unavailable' }
        : {
            success: true,
            data: {
              page: Number(url.searchParams.get('p')),
              page_size: 20,
              total: 1,
              items: [row],
            },
          };
    } else if (url.pathname === '/api/desktop-feedback/11') {
      if (request.method() === 'PATCH') {
        const body = JSON.parse(request.postData());
        patches.push(body);
        if (failSave)
          result = { success: false, message: 'fixture save unavailable' };
        else {
          row = { ...row, status: body.status };
          result.data = row;
        }
      } else result.data = row;
    }
    return request.respond({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(result),
    });
  });
  await page.goto(`${base}/console/desktop-feedback`, {
    waitUntil: 'networkidle0',
  });
  return page;
}
async function clickText(page, text) {
  const clicked = await page.evaluate((text) => {
    const button = [...document.querySelectorAll('button')].find(
      (button) => button.textContent.trim() === text,
    );
    if (!button) return false;
    button.click();
    return true;
  }, text);
  assert.ok(clicked, `button ${text}`);
}
try {
  const page = await fixture();
  await page.waitForFunction(() =>
    document.body.innerText.includes('模型切换失败'),
  );
  assert.equal(await page.evaluate(() => window.__feedbackInjected), undefined);
  await page.type('input[aria-label="版本"]', '0.1.108');
  await clickText(page, '查询');
  await page.waitForFunction(
    () => !document.querySelector('.semi-spin-spinning'),
  );
  await page.waitForNetworkIdle();
  assert.ok(queries.some((query) => query.includes('appVersion=0.1.108')));
  await clickText(page, '查看详情');
  await page.waitForFunction(() =>
    document.body.innerText.includes('error sending request'),
  );
  assert.equal(await page.evaluate(() => window.__feedbackInjected), undefined);
  await page.screenshot({
    path: '/tmp/newapi-desktop-feedback.png',
    fullPage: true,
  });
  await page.click('.semi-modal .semi-select');
  await page.waitForSelector('.semi-select-option');
  for (const option of await page.$$('.semi-select-option')) {
    if (
      (await option.evaluate((element) => element.textContent.trim())) ===
      '已解决'
    ) {
      await option.click();
      break;
    }
  }
  await page.waitForFunction(() =>
    [...document.querySelectorAll('.semi-modal button')].some(
      (button) => button.textContent.trim() === '保存' && !button.disabled,
    ),
  );
  failSave = true;
  await clickText(page, '保存');
  await page.waitForFunction(() =>
    document.body.innerText.includes('fixture save unavailable'),
  );
  assert.equal(row.status, 'pending');
  failSave = false;
  await clickText(page, '保存');
  await page.waitForNetworkIdle();
  assert.deepEqual(patches, [{ status: 'resolved' }, { status: 'resolved' }]);
  assert.equal(row.status, 'resolved');
  await page.click('.semi-modal-close');
  failList = true;
  await clickText(page, '刷新');
  await page.waitForFunction(() =>
    document.body.innerText.includes('fixture list unavailable'),
  );
  assert.equal(
    await page.evaluate(() =>
      document
        .querySelector('.semi-table')
        ?.textContent.includes('模型切换失败'),
    ),
    false,
  );
  failList = false;
  await clickText(page, '刷新');
  await page.waitForFunction(() =>
    document.body.innerText.includes('模型切换失败'),
  );
  await page.setViewport({ width: 390, height: 844 });
  await page.screenshot({
    path: '/tmp/newapi-desktop-feedback-mobile.png',
    fullPage: true,
  });
  await page.close();
  const before = queries.length;
  const ordinary = await fixture(1);
  assert.ok(!ordinary.url().includes('/console/desktop-feedback'));
  assert.equal(
    queries.length,
    before,
    'ordinary users never load admin feedback',
  );
  await ordinary.close();
  assert.deepEqual(pageErrors, []);
  console.log(
    'Desktop feedback browser checks passed: filters, details, escaped text, failed/successful status save, failed/retried listing, mobile layout, admin route.',
  );
} finally {
  await browser.close();
}
