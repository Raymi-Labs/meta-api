import assert from 'node:assert/strict';
import { mkdir, readFile } from 'node:fs/promises';
import puppeteer from 'puppeteer-core';

// Run against the real project Vite server; API data is synthetic and isolated.
const baseURL = process.env.THEME_TEST_URL || 'http://127.0.0.1:5173';
const prerendered = process.env.THEME_TEST_PRERENDERED === '1';
const entryPath = prerendered
  ? (
      await readFile(new URL('../dist/index.html', import.meta.url), 'utf8')
    ).match(/<script type="module"[^>]*src="([^"]+)"/)[1]
  : '/src/index.jsx';
const screenshotDir = new URL(
  `../../output/playwright/theme-init/${prerendered ? 'production' : 'development'}/`,
  import.meta.url,
);
await mkdir(screenshotDir, { recursive: true });
const browser = await puppeteer.launch({
  executablePath:
    process.env.CHROME_PATH ||
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  headless: true,
});

const snapshotTheme = () => ({
  dark: document.documentElement.classList.contains('dark'),
  body: document.body.getAttribute('theme-mode'),
  scheme: getComputedStyle(document.documentElement).colorScheme,
});

async function assertTheme(page, expected) {
  await page.waitForFunction(
    (dark) => document.documentElement.classList.contains('dark') === dark,
    {},
    expected === 'dark',
  );
  assert.deepEqual(await page.evaluate(snapshotTheme), {
    dark: expected === 'dark',
    body: expected === 'dark' ? 'dark' : null,
    scheme: expected,
  });
}

async function preparePage({
  stored,
  system = 'dark',
  blocked = false,
  noMedia = false,
  holdEntry = true,
}) {
  const page = await browser.newPage();
  const pageErrors = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));
  await page.setViewport({ width: 1280, height: 800 });
  await page.emulateMediaFeatures([
    { name: 'prefers-color-scheme', value: system },
  ]);
  const initialization = await page.evaluateOnNewDocument(
    ({ stored, blocked, noMedia }) => {
      localStorage.setItem('i18nextLng', 'zh-CN');
      if (stored === null) localStorage.removeItem('theme-mode');
      else localStorage.setItem('theme-mode', stored);
      if (blocked) {
        const getItem = Storage.prototype.getItem;
        const setItem = Storage.prototype.setItem;
        Storage.prototype.getItem = function (key) {
          if (key === 'theme-mode')
            throw new DOMException('disabled', 'SecurityError');
          return getItem.call(this, key);
        };
        Storage.prototype.setItem = function (key, value) {
          if (key === 'theme-mode')
            throw new DOMException('full', 'QuotaExceededError');
          return setItem.call(this, key, value);
        };
      }
      if (noMedia) window.matchMedia = undefined;
    },
    { stored, blocked, noMedia },
  );
  let heldEntry;
  await page.setRequestInterception(true);
  page.on('request', (request) => {
    const url = new URL(request.url());
    if (holdEntry && url.pathname === entryPath) {
      heldEntry = request;
      return;
    }
    if (url.pathname.startsWith('/api/')) {
      const data =
        url.pathname === '/api/status'
          ? {
              setup: true,
              system_name: 'Theme fixture',
              logo: '/yuanheng-logo.png',
              HeaderNavModules: JSON.stringify({
                home: true,
                console: true,
                pricing: false,
                about: false,
                docs: false,
              }),
            }
          : null;
      request.respond({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ success: true, data }),
      });
      return;
    }
    if (url.pathname === '/desktop/update/downloads.json') {
      request.respond({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ version: '0.1.0', packages: [] }),
      });
      return;
    }
    if (url.origin !== new URL(baseURL).origin) {
      request.abort();
      return;
    }
    request.continue();
  });
  return {
    page,
    pageErrors,
    stopSeeding: () =>
      page.removeScriptToEvaluateOnNewDocument(initialization.identifier),
    releaseEntry: () => {
      assert.ok(heldEntry, 'entry module must be held');
      return heldEntry.continue();
    },
  };
}

try {
  const cases = [
    { name: 'saved-dark', stored: 'dark', system: 'light', expected: 'dark' },
    { name: 'saved-light', stored: 'light', system: 'dark', expected: 'light' },
    { name: 'auto-dark', stored: 'auto', system: 'dark', expected: 'dark' },
    { name: 'auto-light', stored: 'auto', system: 'light', expected: 'light' },
    { name: 'missing', stored: null, expected: 'dark' },
    { name: 'corrupt', stored: 'invalid', expected: 'dark' },
    {
      name: 'blocked-storage',
      stored: 'light',
      blocked: true,
      expected: 'dark',
    },
    { name: 'no-media', stored: 'auto', noMedia: true, expected: 'light' },
  ];
  for (const scenario of cases) {
    const { page, releaseEntry, pageErrors } = await preparePage(scenario);
    const navigation = page.goto(`${baseURL}/download`, {
      waitUntil: 'domcontentloaded',
    });
    await page.waitForSelector('#root');
    await page.evaluate(
      () =>
        new Promise((resolve) =>
          requestAnimationFrame(() => requestAnimationFrame(resolve)),
        ),
    );
    if (!prerendered) {
      assert.equal(
        await page.$('.public-shell'),
        null,
        'React has not mounted at first-frame inspection',
      );
    }
    // createRoot replaces this marker when React actually takes over prerendered HTML.
    await page.evaluate(() => {
      const marker = document.createElement('span');
      marker.id = 'theme-test-before-mount';
      document.getElementById('root').appendChild(marker);
    });
    await assertTheme(page, scenario.expected);
    if (scenario.name.startsWith('saved-')) {
      await page.screenshot({
        path: new URL(`${scenario.name}-first-frame.png`, screenshotDir)
          .pathname,
      });
    }
    await releaseEntry();
    await navigation;
    // matchMedia removal affects unrelated UI dependencies; initialization is the scoped fallback.
    if (!scenario.noMedia) {
      await page.waitForFunction(
        () => !document.getElementById('theme-test-before-mount'),
      );
      await page.waitForSelector('.public-theme-button');
      await assertTheme(page, scenario.expected);
      if (scenario.name === 'corrupt' || scenario.blocked) {
        assert.equal(
          await page.$eval('.public-theme-button', (button) =>
            button.textContent.trim(),
          ),
          'A',
        );
      }
      if (scenario.blocked) {
        await page.click('.public-theme-button'); // auto -> dark
        await page.click('.public-theme-button'); // dark -> light, despite failed storage write
        await assertTheme(page, 'light');
      }
      assert.deepEqual(
        pageErrors,
        [],
        'theme scenarios must not cause runtime exceptions',
      );
    }
    console.log(
      `PASS first frame${scenario.noMedia ? '' : ' / React takeover'}: ${scenario.name}`,
    );
    await page.close();
  }

  const { page, stopSeeding, pageErrors } = await preparePage({
    stored: 'auto',
    system: 'light',
    holdEntry: false,
  });
  await page.goto(`${baseURL}/download`, { waitUntil: 'networkidle0' });
  await page.waitForSelector('.public-theme-button');
  await assertTheme(page, 'light');
  await page.emulateMediaFeatures([
    { name: 'prefers-color-scheme', value: 'dark' },
  ]);
  await assertTheme(page, 'dark');
  await page.click('.public-theme-button'); // auto -> dark
  await page.emulateMediaFeatures([
    { name: 'prefers-color-scheme', value: 'light' },
  ]);
  await assertTheme(page, 'dark');
  await page.click('.public-theme-button'); // dark -> light
  await assertTheme(page, 'light');
  assert.equal(
    await page.evaluate(() => localStorage.getItem('theme-mode')),
    'light',
  );
  // The on-new-document fixture no longer seeds storage: refresh must retain the user's choice.
  await stopSeeding();
  await page.reload({ waitUntil: 'networkidle0' });
  await assertTheme(page, 'light');
  await page.click('.public-theme-button'); // light -> auto
  await page.emulateMediaFeatures([
    { name: 'prefers-color-scheme', value: 'dark' },
  ]);
  await assertTheme(page, 'dark');
  assert.equal(
    await page.evaluate(() => localStorage.getItem('theme-mode')),
    'auto',
  );
  await page.waitForSelector('.desktop-download-page');
  assert.match(
    await page.$eval(
      '.desktop-download-page',
      (element) => getComputedStyle(element).backgroundImage,
    ),
    /rgb\(247, 251, 255\)/,
  );
  await page.screenshot({
    path: new URL('download-auto-dark.png', screenshotDir).pathname,
    fullPage: true,
  });
  assert.deepEqual(
    pageErrors,
    [],
    'theme switching and reload must not cause runtime exceptions',
  );
  console.log(
    'PASS live system changes, manual override, refresh, storage key and fixed-light download design',
  );
  await page.close();

  if (prerendered) {
    for (const route of ['login', 'register', 'pricing', 'download', 'about']) {
      const html = await readFile(
        new URL(`../dist/${route}/index.html`, import.meta.url),
        'utf8',
      );
      assert.ok(
        html.includes('id="theme-init"') &&
          html.includes('id="theme-body-init"'),
        `${route} keeps both bootstrap scripts`,
      );
      assert.ok(
        html.indexOf('id="theme-init"') < html.indexOf('type="module"'),
      );
      assert.ok(
        html.indexOf('id="theme-body-init"') < html.indexOf('id="root"'),
      );
      const { page, releaseEntry } = await preparePage({
        stored: 'dark',
        system: 'light',
      });
      const navigation = page.goto(`${baseURL}/${route}/`, {
        waitUntil: 'domcontentloaded',
      });
      await page.waitForSelector('#root');
      assert.ok(
        await page.$eval('#root', (root) => root.childElementCount > 0),
        `${route} has pre-rendered content before React`,
      );
      await assertTheme(page, 'dark');
      if (route === 'login') {
        await page.screenshot({
          path: new URL('login-dark-first-frame.png', screenshotDir).pathname,
        });
      }
      await releaseEntry();
      await navigation;
      await assertTheme(page, 'dark');
      await page.close();
      console.log(`PASS production pre-rendered first frame: /${route}`);
    }
  }
} finally {
  await browser.close();
}
