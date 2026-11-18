// Run against the local Vite build with an isolated headless browser and synthetic responses.
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { extname, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';

const root = fileURLToPath(new URL('../dist/', import.meta.url));
const origin = 'https://reasoning-qa.invalid';
const user = { id: 1, role: 100, username: 'fixture', group: 'default' };
const item = {
  id: 1,
  model_name: 'fixture-model',
  name_rule: 0,
  status: 1,
  sync_official: 0,
  reasoning_profile: { supportedLevels: ['low', 'ultra'], defaultLevel: 'low' },
};
const browser = await puppeteer.launch({
  executablePath: process.env.CHROME_BIN,
  headless: true,
  pipe: true,
  args: [
    '--disable-background-networking',
    '--disable-component-update',
    '--disable-sync',
    '--no-first-run',
  ],
});
try {
  const page = await browser.newPage();
  page.setDefaultTimeout(10000);
  await page.setRequestInterception(true);
  let saved;
  page.on('request', async (req) => {
    try {
      const { pathname, origin: host } = new URL(req.url());
      if (host !== origin) return await req.abort();
      if (pathname.startsWith('/api/')) {
        let data = [];
        if (pathname === '/api/status')
          data = { setup: true, system_name: 'QA' };
        if (pathname === '/api/user/self') data = user;
        if (pathname === '/api/models/' && req.method() === 'PUT')
          saved = JSON.parse(req.postData());
        if (pathname === '/api/models/')
          data = { items: [item], total: 1, page: 1 };
        if (pathname === '/api/models/1') data = item;
        return await req.respond({
          contentType: 'application/json',
          body: JSON.stringify({ success: true, data }),
        });
      }
      const file = resolve(
        root,
        '.' +
          (pathname === '/console/models'
            ? '/index.html'
            : decodeURIComponent(pathname)),
      );
      if (!file.startsWith(resolve(root) + sep)) return await req.abort();
      const types = {
        '.html': 'text/html',
        '.css': 'text/css',
        '.js': 'application/javascript',
        '.svg': 'image/svg+xml',
        '.json': 'application/json',
      };
      return await req.respond({
        contentType: types[extname(file)] || 'application/octet-stream',
        body: await readFile(file),
      });
    } catch {
      if (!req.isInterceptResolutionHandled()) await req.abort();
    }
  });
  await page.evaluateOnNewDocument((user) => {
    localStorage.setItem('user', JSON.stringify(user));
    localStorage.setItem('site-language', 'zh-CN');
  }, user);
  await page.setViewport({ width: 1280, height: 720 });
  await page.goto(origin + '/console/models');
  async function clickText(selector, text) {
    const el = await page.waitForFunction(
      (sel, txt) =>
        Array.from(document.querySelectorAll(sel)).find(
          (el) => el.textContent.trim() === txt && el.getClientRects().length,
        ),
      {},
      selector,
      text,
    );
    await el.asElement().click();
  }
  await clickText('button', '编辑');
  const defaultSelector = '[id="reasoning_profile.defaultLevel"]';
  await page.waitForSelector(defaultSelector);
  await page.$eval(defaultSelector, (el) =>
    el.scrollIntoView({ block: 'center' }),
  );
  assert.match(
    await page.$eval(defaultSelector, (el) => el.textContent),
    /low/,
  );
  const control = await page.evaluateHandle(
    (sel) =>
      document
        .querySelector(sel)
        .closest('.semi-form-field')
        .querySelector('[role="combobox"], .semi-select'),
    defaultSelector,
  );
  await control.evaluate((el) =>
    el.scrollIntoView({ block: 'center', behavior: 'instant' }),
  );
  await page.waitForFunction(
    (sel) => {
      const el = document.querySelector(sel);
      const ctl = el
        .closest('.semi-form-field')
        .querySelector('[role="combobox"], .semi-select');
      const r = ctl.getBoundingClientRect();
      return (
        r.width > 0 &&
        r.height > 0 &&
        ctl.contains(
          document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2),
        )
      );
    },
    { timeout: 3000 },
    defaultSelector,
  );
  await control.asElement().click();
  await clickText('[role="option"]', 'ultra');
  await page.waitForFunction(
    (sel) => document.querySelector(sel)?.textContent.includes('ultra'),
    {},
    defaultSelector,
  );
  await clickText('button', '提交');
  await page.waitForFunction(
    () => !document.querySelector('[id="reasoning_profile.defaultLevel"]'),
  );
  assert.deepEqual(saved.reasoning_profile, {
    supportedLevels: ['low', 'ultra'],
    defaultLevel: 'ultra',
  });
  console.log('PASS: isolated admin edit -> actual save payload');
} finally {
  await browser.close();
}
