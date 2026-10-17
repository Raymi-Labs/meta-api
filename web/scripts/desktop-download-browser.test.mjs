import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import puppeteer from 'puppeteer-core';

// Exercise the real project shell with public, synthetic catalog/download data.
const baseURL = process.env.DOWNLOAD_TEST_URL || 'http://127.0.0.1:5187';
const screenshotDir = process.env.DOWNLOAD_SCREENSHOT_DIR;
if (screenshotDir) await mkdir(screenshotDir, { recursive: true });
const downloadDir = await mkdtemp(join(tmpdir(), 'desktop-download-test-'));
const catalog = {
  version: '1.2.3',
  pub_date: '2026-10-17T00:00:00Z',
  packages: [
    ['macos-arm64', 'aarch64.dmg'],
    ['macos-x64', 'x64.dmg'],
    ['windows-x64', 'x64-setup.exe'],
  ].map(([id, suffix]) => ({
    id,
    filename: `YuanHeng.Desktop_1.2.3_${suffix}`,
    size: 23416225,
    url: `https://meta-api.vip/desktop/update/releases/1.2.3/YuanHeng.Desktop_1.2.3_${suffix}`,
  })),
};
const browser = await puppeteer.launch({
  executablePath:
    process.env.CHROME_PATH ||
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  headless: true,
});
try {
  for (const [width, language, dark] of [
    [1440, 'zh', false],
    [320, 'en', true],
  ]) {
    const page = await browser.newPage();
    const errors = [];
    const downloads = [];
    let catalogMode = 'ready';
    page.on('pageerror', (error) => errors.push(error.message));
    await page.setViewport({ width, height: 1100 });
    await page.emulateMediaFeatures([
      { name: 'prefers-color-scheme', value: dark ? 'dark' : 'light' },
    ]);
    await page.evaluateOnNewDocument(
      (language, dark) => {
        localStorage.setItem('i18nextLng', language);
        localStorage.setItem('theme-mode', dark ? 'dark' : 'light');
      },
      language,
      dark,
    );
    const session = await page.createCDPSession();
    await session.send('Browser.setDownloadBehavior', {
      behavior: 'allow',
      downloadPath: downloadDir,
    });
    await page.setRequestInterception(true);
    page.on('request', async (request) => {
      const url = new URL(request.url());
      if (url.origin !== new URL(baseURL).origin) {
        if (
          catalog.packages.some((item) =>
            url.pathname.endsWith(`/${item.filename}`),
          )
        ) {
          const filename = url.pathname.split('/').at(-1);
          downloads.push(url.href);
          await request.respond({
            status: 200,
            contentType: 'application/octet-stream',
            headers: {
              'Content-Disposition': `attachment; filename="${filename}"`,
            },
            body: `fixture:${url.href}`,
          });
        } else await request.abort();
      } else if (url.pathname === '/desktop/update/downloads.json') {
        const body =
          catalogMode === 'custom'
            ? {
                ...catalog,
                packages: catalog.packages.map((item) => ({
                  ...item,
                  url: item.url.replace('meta-api.vip', 'custom.example.com'),
                })),
              }
            : catalog;
        await request.respond({
          status: catalogMode === 'error' ? 503 : 200,
          contentType: 'application/json',
          body: JSON.stringify(body),
        });
      } else if (url.pathname.startsWith('/api/')) {
        await request.respond({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            success: true,
            data:
              url.pathname === '/api/status'
                ? { setup: true, system_name: '元衡 API', logo: '/logo.png' }
                : [],
          }),
        });
      } else await request.continue();
    });
    const ready = () => page.waitForSelector('article.desktop-download-card');
    await page.goto(`${baseURL}/download`, { waitUntil: 'networkidle0' });
    await ready();
    assert.equal(
      await page.$$eval('.desktop-download-action', (links) => links.length),
      3,
    );
    assert.equal(
      await page.$$eval('.desktop-download-backups a', (links) => links.length),
      6,
    );
    assert.equal(await page.$('a a'), null);
    assert.equal(
      await page.$eval(
        '.desktop-download-backups span',
        (el) => el.textContent,
      ),
      language === 'en' ? 'Alternative downloads' : '备用线路',
    );
    const links = await page.$$eval('article:first-child a', (nodes) =>
      nodes.map((a) => a.href),
    );
    assert.equal(new URL(links[0]).search, '?origin=1');
    assert.equal(new URL(links[1]).host, 'download.meta-api.vip');
    assert.equal(new URL(links[2]).host, 'github.com');
    // Keyboard navigation reaches the main action and both independent backups.
    await page.focus('article:first-child .desktop-download-action');
    for (const link of links) {
      assert.equal(
        await page.evaluate(() => document.activeElement.href),
        link,
      );
      await page.keyboard.press('Tab');
    }
    // Each link actually delivers its own attachment and leaves the page intact.
    for (let i = 0; i < links.length; i++) {
      const filename = new URL(links[i]).pathname.split('/').at(-1);
      const path = join(downloadDir, filename);
      const anchors = await page.$$('article:first-child a');
      await anchors[i].click();
      let contents;
      for (let attempt = 0; attempt < 60; attempt++) {
        try {
          contents = await readFile(path, 'utf8');
          break;
        } catch {
          /* await browser download */
        }
        await new Promise((resolve) => setTimeout(resolve, 100));
      }
      assert.equal(contents, `fixture:${links[i]}`);
      await rm(path);
      assert.equal(page.url(), `${baseURL}/download`);
    }
    assert.deepEqual(downloads, links);
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth > innerWidth,
      ),
      false,
    );
    if (screenshotDir)
      await page.screenshot({
        path: join(screenshotDir, `download-${width}-${language}.png`),
        fullPage: true,
      });
    await page.reload({ waitUntil: 'networkidle0' });
    await ready();
    catalogMode = 'error';
    await page.reload({ waitUntil: 'networkidle0' });
    await page.waitForSelector('.desktop-download-empty button');
    assert.equal(await page.$('article.desktop-download-card'), null);
    catalogMode = 'ready';
    await page.click('.desktop-download-empty button');
    await ready();
    catalogMode = 'custom';
    await page.reload({ waitUntil: 'networkidle0' });
    await ready();
    assert.equal(await page.$('.desktop-download-backups'), null);
    assert.equal(
      await page.$eval('.desktop-download-action', (a) => new URL(a.href).host),
      'custom.example.com',
    );
    assert.deepEqual(errors, []);
    await page.close();
  }
  console.log(
    'desktop download browser: links, attachments, keyboard, mobile, language, refresh, error retry and custom URLs passed',
  );
} finally {
  await browser.close();
  await rm(downloadDir, { recursive: true, force: true });
}
