import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';
import { preview } from 'vite';
import { stop as stopEsbuild } from 'esbuild';

const root = fileURLToPath(new URL('..', import.meta.url));
process.chdir(root);
// Build first: bunx vite build --outDir /tmp/meta-native-chat-production
// Use production mode so React development effect replay does not compete
// for the playground's cross-tab session lock.
const server = await preview({
  root,
  configFile: false,
  build: {
    outDir:
      process.env.SIDEBAR_PREVIEW_DIR || '/tmp/meta-native-chat-production',
  },
  preview: { host: '127.0.0.1', port: 0 },
});
const base = `http://127.0.0.1:${server.httpServer.address().port}`;
let browser;
try {
  browser = await puppeteer.launch({
    headless: true,
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  });
  for (const [width, role, dark, hidden = false, brokenChats = false] of [
    [1440, 100, false],
    [1440, 100, true],
    [390, 1, false],
    [390, 1, true, false, true],
    [1440, 1, false, true],
  ]) {
    const page = await browser.newPage(),
      errors = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await page.setViewport({ width, height: 1100 });
    await page.evaluateOnNewDocument(
      (role, dark, brokenChats) => {
        localStorage.setItem(
          'user',
          JSON.stringify({ id: 7, role, username: 'fixture' }),
        );
        localStorage.setItem('i18nextLng', 'zh-CN');
        localStorage.setItem('site-language', 'zh-CN');
        localStorage.setItem('default_collapse_sidebar', 'false');
        localStorage.setItem('theme-mode', dark ? 'dark' : 'light');
        localStorage.setItem(
          'chats',
          brokenChats
            ? '{invalid'
            : JSON.stringify([{ 测试会话: 'https://example.invalid/' }]),
        );
        localStorage.setItem(
          'newapi:user-onboarding:regular:v1:user:7',
          'done',
        );
      },
      role,
      dark,
      brokenChats,
    );
    await page.setRequestInterception(true);
    page.on('request', async (req) => {
      const u = new URL(req.url());
      if (u.origin !== base) return req.abort();
      if (!u.pathname.startsWith('/api/')) return req.continue();
      let data = [];
      if (u.pathname === '/api/user/self')
        data = { id: 7, role, username: 'fixture', setting: '{}' };
      if (u.pathname === '/api/status')
        data = {
          system_name: '元衡',
          quota_per_unit: 500000,
          enable_data_export: true,
          chats: [{ 测试会话: 'https://example.invalid/' }],
          SidebarModulesAdmin: JSON.stringify({
            chat: { enabled: true, imagePlayground: true, chat: !hidden },
          }),
        };
      if (u.pathname === '/api/token/' || u.pathname === '/api/log/')
        data = { items: [], total: 0, page: 1, page_size: 10 };
      await req.respond({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ success: true, data }),
      });
    });
    await page.goto(base + '/console/token', { waitUntil: 'networkidle0' });
    if (width < 768) await page.click('[aria-label="打开侧边栏"]');
    await page.waitForSelector(
      '.sidebar-nav [data-onboarding="sidebar-token"]',
    );
    const measure = () =>
      page.$$eval(
        '.sidebar-nav .semi-navigation-item-normal, .sidebar-nav .semi-navigation-sub-title',
        (els) =>
          els
            .filter((e) => e.getBoundingClientRect().height > 0)
            .map((e) => {
              const icon = e.querySelector('.sidebar-icon-container'),
                text = e.querySelector('[data-onboarding]'),
                arrow = e.querySelector(
                  '.semi-navigation-item-icon-toggle-right svg',
                );
              const center = (x) => {
                const r = x.getBoundingClientRect();
                return r.y + r.height / 2;
              };
              return {
                text: text?.textContent,
                difference:
                  icon && text ? Math.abs(center(icon) - center(text)) : 0,
                arrowDifference:
                  arrow && text ? Math.abs(center(arrow) - center(text)) : 0,
                height: e.getBoundingClientRect().height,
              };
            }),
      );
    await page.waitForFunction(
      () =>
        Array.from(
          document.querySelectorAll(
            '.sidebar-nav .semi-navigation-item-normal',
          ),
        ).filter((el) => el.getBoundingClientRect().height > 0).length >= 7,
    );
    const toolGroup = await page.$eval(
      '[data-onboarding="sidebar-chat-section"]',
      (el) => ({
        title: el.parentElement.querySelector('.sidebar-group-label')
          ?.textContent,
        labels: el.innerText,
      }),
    );
    assert.equal(toolGroup.title, '创作空间');
    assert.ok(toolGroup.labels.includes('元衡生图'));
    assert.ok(!toolGroup.labels.includes('聊天'));
    let metrics = await measure();
    if (width === 1440 && !dark) {
      const oldStyle = await page.addStyleTag({
        content:
          '.sidebar-nav .semi-navigation-item-normal{align-items:normal}.sidebar-nav .semi-navigation-item-text{display:block}',
      });
      const oldMetrics = await measure();
      assert(
        oldMetrics.some((m) => m.difference >= 4),
        'fixture must reproduce the old misalignment',
      );
      await oldStyle.evaluate((el) => el.remove());
      metrics = await measure();
    }
    assert(metrics.length >= 7);
    for (const m of metrics) {
      assert(m.difference < 1, JSON.stringify(m));
      assert(m.arrowDifference < 1, JSON.stringify(m));
      assert(m.height >= 40);
    }
    assert.equal(
      await page.$eval(
        '.sidebar-nav .semi-navigation-item-selected [data-onboarding]',
        (e) => e.dataset.onboarding,
      ),
      'sidebar-token',
    );
    assert.equal(
      !!(await page.$('[data-onboarding="sidebar-channel"]')),
      role === 100,
    );
    assert.equal(await page.$('.sidebar-nav .semi-navigation-sub-title'), null);
    assert.equal(await page.$('[data-onboarding="sidebar-chat0"]'), null);
    const nativeLink = await page.$('.sidebar-nav a[href="/playground"]');
    assert.equal(!!nativeLink, !hidden);
    if (!hidden) {
      assert.equal(
        await page.$eval(
          '[data-onboarding="sidebar-chat"]',
          (el) => el.textContent,
        ),
        '模型体验',
      );
    }
    await page.screenshot({
      path: `/tmp/sidebar-native-chat-${width}-${dark}-${hidden}.png`,
    });
    if (width >= 768) {
      await page.focus('.sidebar-collapse-button button');
      await page.keyboard.press('Enter');
      await page.waitForFunction(() =>
        document.body.classList.contains('sidebar-collapsed'),
      );
      const heights = await page.$$eval(
        '.sidebar-nav .semi-navigation-item-collapsed',
        (els) =>
          els.map((e) => e.getBoundingClientRect().height).filter(Boolean),
      );
      assert(heights.length > 0);
      heights.forEach((h) => assert.equal(h, 44));
      await page.focus('.sidebar-collapse-button button');
      await page.keyboard.press('Enter');
      await page.waitForFunction(
        () => !document.body.classList.contains('sidebar-collapsed'),
      );
      metrics = await measure();
      for (const m of metrics)
        assert(m.difference < 1 && m.arrowDifference < 1, JSON.stringify(m));
    }
    await page.click('.sidebar-nav a[href="/console/log"]');
    await page.waitForFunction(() => location.pathname === '/console/log');
    if (!hidden) {
      if (width < 768) await page.click('[aria-label="打开侧边栏"]');
      await page.focus('.sidebar-nav a[href="/playground"]');
      await page.keyboard.press('Enter');
      await page
        .waitForFunction(() => location.pathname === '/playground', {
          timeout: 10000,
        })
        .catch(async (error) => {
          console.error('Destination:', page.url(), 'Errors:', errors);
          await page.screenshot({
            path: '/tmp/native-chat-navigation-error.png',
          });
          throw error;
        });
      await page
        .waitForSelector('textarea, [contenteditable="true"]', {
          timeout: 10000,
        })
        .catch(async (error) => {
          console.error(
            'Worktable:',
            page.url(),
            'Errors:',
            errors,
            'Text:',
            await page.$eval('body', (el) => el.innerText.slice(-1500)),
          );
          await page.screenshot({
            path: '/tmp/native-chat-navigation-error.png',
          });
          throw error;
        });
      assert.equal(await page.$('iframe[title="Token Frame"]'), null);
    }
    assert.deepEqual(errors, []);
    console.log(
      `PASS ${width}px role=${role} dark=${dark}: native chat link, old submenu removed, visibility, selected state, navigation and collapse`,
    );
    await page.close();
  }
} finally {
  await browser?.close();
  await new Promise((resolve) => server.httpServer.close(resolve));
  stopEsbuild();
  console.log('Browser and server closed; no retained test processes.');
}
