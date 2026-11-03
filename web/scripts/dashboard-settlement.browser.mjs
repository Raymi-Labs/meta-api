import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import puppeteer from 'puppeteer-core';

// Isolated built-app verification. All API traffic is mocked and non-local
// requests are blocked. Never use production credentials or paid requests.
const dist = path.resolve(process.argv[2] || 'dist');
const server = createServer(async (req, res) => {
  const pathname = new URL(req.url, 'http://localhost').pathname;
  const file = pathname.startsWith('/assets/')
    ? path.join(dist, pathname)
    : path.join(dist, 'index.html');
  if (!file.startsWith(dist + path.sep)) {
    res.writeHead(403).end();
    return;
  }
  try {
    const content = await readFile(file);
    const types = {
      '.js': 'application/javascript',
      '.css': 'text/css',
      '.html': 'text/html',
      '.svg': 'image/svg+xml',
    };
    res.setHeader(
      'Content-Type',
      types[path.extname(file)] || 'application/octet-stream',
    );
    res.end(content);
  } catch {
    res.writeHead(404).end();
  }
});
await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
const base = `http://127.0.0.1:${server.address().port}`;
let browser;
try {
  browser = await puppeteer.launch({
    headless: true,
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  });
  for (const role of [1, 10]) {
    const page = await browser.newPage();
    await page.setViewport({ width: role === 1 ? 1440 : 390, height: 1000 });
    const requests = [],
      errors = [];
    let mode = 'normal';
    let delayedRequest;
    page.on('pageerror', (e) => errors.push(e.message));
    const user = {
      id: 777,
      username: 'audit-fixture',
      role,
      group: 'default',
      quota: 500000,
      setting: '{}',
    };
    const status = {
      system_name: 'New API',
      quota_per_unit: 500000,
      quota_display_type: 'CUSTOM',
      custom_currency_symbol: '⚡',
      custom_currency_exchange_rate: 1,
      enable_data_export: true,
      uptime_enabled: false,
    };
    await page.evaluateOnNewDocument(
      (user, status) => {
        localStorage.setItem('user', JSON.stringify(user));
        localStorage.setItem('status', JSON.stringify(status));
        localStorage.setItem('quota_per_unit', '500000');
        localStorage.setItem('quota_display_type', 'CUSTOM');
        localStorage.setItem('enable_data_export', 'true');
        localStorage.setItem('i18nextLng', 'zh-CN');
        localStorage.setItem('theme-mode', 'light');
        localStorage.setItem(
          `newapi:user-onboarding:regular:v1:user:${user.id}`,
          'done',
        );
      },
      user,
      status,
    );
    await page.setRequestInterception(true);
    page.on('request', async (request) => {
      try {
        const url = new URL(request.url());
        if (url.origin !== base) return await request.abort();
        if (!url.pathname.startsWith('/api/')) return await request.continue();
        requests.push(url.pathname);
        let body = { success: true, data: [] };
        if (url.pathname === '/api/status') body.data = status;
        if (url.pathname === '/api/user/self') body.data = user;
        if (url.pathname === '/api/option/') body.data = [];
        if (
          url.pathname === '/api/data/self/' ||
          url.pathname === '/api/data/'
        ) {
          const requestMode = mode;
          const start = Number(url.searchParams.get('start_timestamp'));
          const end = Number(url.searchParams.get('end_timestamp'));
          const quota = mode === 'negative' ? -500000 : 92473150;
          body.data = [
            {
              model_name: 'fixture',
              created_at: end - 60,
              quota,
              count: 221,
              token_used: 27082015,
            },
          ];
          if (mode !== 'legacy')
            body.summary = {
              source: 'retained_settlement_logs_v1',
              start_timestamp: start,
              end_timestamp: end,
              quota,
              requests: 221,
              tokens: 27082015,
              token_quota: 92473150,
              other_quota: quota - 92473150,
              priced_tokens: 27082015,
              input_tokens: 26898923,
              cache_read_tokens: 21815522,
              unclassified_records: 0,
              cache_included_records: 221,
              cache_excluded_records: 0,
              price_included_records: 221,
              price_excluded_records: 0,
              cache_rate: 21815522 / 26898923,
              unit_price: (92473150 / 27082015) * 1000000,
            };
          if (mode === 'error')
            body = {
              success: false,
              message: '统计记录过多，请缩短时间范围或指定用户',
            };
          if (mode === 'unknown') {
            body.summary.unclassified_records = 221;
            body.summary.cache_included_records = 0;
            body.summary.cache_excluded_records = 221;
            body.summary.price_included_records = 0;
            body.summary.price_excluded_records = 221;
            body.summary.cache_rate = null;
            body.summary.unit_price = null;
          }
          if (mode === 'partial') {
            body.summary.unclassified_records = 21;
            body.summary.cache_included_records = 200;
            body.summary.cache_excluded_records = 21;
            body.summary.price_included_records = 190;
            body.summary.price_excluded_records = 31;
            body.summary.tokens = 12936310889;
            body.data[0].token_used = 12936310889;
          }
          if (requestMode === 'slow') {
            body.summary.requests = 999;
            body.data[0].count = 999;
            delayedRequest?.();
            await new Promise((resolve) => setTimeout(resolve, 1200));
          }
        }
        await request.respond({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(body),
        });
      } catch {
        /* A navigation can cancel a fixture response. */
      }
    });
    const load = async (next) => {
      mode = next;
      await page.goto(base + '/console', {
        waitUntil: 'networkidle0',
        timeout: 30000,
      });
      await page.waitForSelector('.dashboard-stat-grid', { timeout: 15000 });
      await page.waitForFunction(() =>
        [...document.querySelectorAll('.dashboard-stat-card')].every(
          (e) => e.getAttribute('aria-busy') === 'false',
        ),
      );
    };
    await load('normal');
    await page.waitForFunction(() =>
      document
        .querySelector('.dashboard-stat-grid')
        ?.textContent.includes('6.8291'),
    );
    let text = await page.$eval('.dashboard-stat-grid', (e) => e.textContent);
    assert(
      text.includes('27,082,015') &&
        text.includes('221') &&
        !text.includes('983.1660'),
    );
    assert(!requests.some((p) => p.startsWith('/api/log/')));
    assert(requests.includes(role === 1 ? '/api/data/self/' : '/api/data/'));
    if (role === 1) assert(!requests.includes('/api/data/'));
    mode = 'slow';
    const slowStarted = new Promise((resolve) => {
      delayedRequest = resolve;
    });
    await page.select('.dashboard-range-select', '7d');
    await slowStarted;
    mode = 'normal';
    await page.select('.dashboard-range-select', '24h');
    await page.waitForFunction(() =>
      document
        .querySelector('.dashboard-stat-grid')
        ?.textContent.includes('6.8291'),
    );
    await new Promise((resolve) => setTimeout(resolve, 1400));
    text = await page.$eval('.dashboard-stat-grid', (e) => e.textContent);
    assert(
      text.includes('221') && !text.includes('999'),
      'late response replaced current snapshot',
    );
    await load('negative');
    await page.waitForFunction(() =>
      document
        .querySelector('.dashboard-stat-grid')
        ?.textContent.includes('-1.00'),
    );
    await load('unknown');
    await page.waitForFunction(() =>
      document.body.textContent.includes('部分日志用量口径不明确'),
    );
    text = await page.$eval('.dashboard-stat-grid', (e) => e.textContent);
    assert(!text.includes('6.8291') && text.includes('—'));
    await load('partial');
    text = await page.$eval('.dashboard-stat-grid', (e) => e.textContent);
    assert(text.includes('6.8291') && text.includes('部分记录'));
    assert(text.includes('200/221') && text.includes('190/221'));
    assert(text.includes('129.36亿'));
    const tokenBox = await page.$eval('.dashboard-stat-token-value', (e) => ({
      title: e.title,
      width: e.clientWidth,
      scroll: e.scrollWidth,
      lineHeight: parseFloat(getComputedStyle(e).lineHeight),
      height: e.clientHeight,
    }));
    assert.equal(tokenBox.title, '12,936,310,889');
    assert(tokenBox.scroll <= tokenBox.width, 'large tokens overflow card');
    assert(tokenBox.height < tokenBox.lineHeight * 2, 'large tokens wrap');
    await page.screenshot({
      path: `/tmp/dashboard-partial-role-${role}.png`,
      fullPage: false,
    });
    for (const next of ['legacy', 'error']) {
      await load(next);
      await page.waitForSelector('.dashboard-load-error');
      text = await page.$eval('.dashboard-stat-grid', (e) => e.textContent);
      assert(!text.includes('6.8291') && !text.includes('27,082,015'));
    }
    assert.deepEqual(errors, []);
    await page.close();
    console.log(
      `PASS role=${role}: corrected cards, unified request, late-response isolation, signed refunds, ambiguous usage, legacy response and error states`,
    );
  }
} finally {
  await browser?.close();
  await new Promise((resolve) => server.close(resolve));
  console.log('Browser and fixture server closed; no retained test processes.');
}
