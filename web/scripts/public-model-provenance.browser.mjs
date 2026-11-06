import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';
import { createServer, loadConfigFromFile } from 'vite';
import { stop as stopEsbuild } from 'esbuild';

// Pass the local output of TestPublicPricingMetadataSnapshot, never credentials.
const snapshot = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const root = fileURLToPath(new URL('..', import.meta.url));
process.chdir(root);
const { config } = await loadConfigFromFile(
  { command: 'serve', mode: 'development' },
  path.join(root, 'vite.config.js'),
);
const server = await createServer({
  ...config,
  configFile: false,
  root,
  server: { host: '127.0.0.1', port: 0, open: false, proxy: {} },
  plugins: config.plugins
    .flat(Infinity)
    .filter((p) => p?.name !== '@code-inspector/vite'),
});
await server.listen();
const base = `http://127.0.0.1:${server.httpServer.address().port}`;
let browser;
try {
  browser = await puppeteer.launch({
    headless: true,
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  });
  for (const [name, width] of [
    ['gpt-4o', 1440],
    ['flint-v1.0:free', 390],
  ]) {
    const page = await browser.newPage(),
      errors = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await page.setViewport({ width, height: 1000 });
    await page.evaluateOnNewDocument(() => {
      localStorage.setItem('i18nextLng', 'zh-CN');
      localStorage.setItem('site-language', 'zh-CN');
    });
    await page.setRequestInterception(true);
    page.on('request', async (req) => {
      const u = new URL(req.url());
      if (u.origin !== base) return req.abort();
      if (!u.pathname.startsWith('/api/')) return req.continue();
      let body = { success: true, data: [] };
      if (u.pathname === '/api/pricing')
        body = {
          success: true,
          ...snapshot,
          group_ratio: { 优选: 0.8, 企业级: 1, 福利: 0.6 },
          usable_group: { 优选: '优选', 企业级: '企业级', 福利: '福利' },
          time_ratio: {},
        };
      if (u.pathname === '/api/status')
        body.data = {
          system_name: '元衡',
          quota_per_unit: 500000,
          quota_display_type: 'CUSTOM',
          custom_currency_symbol: '⚡',
          custom_currency_exchange_rate: 1,
          HeaderNavModules: JSON.stringify({
            pricing: { enabled: true, requireAuth: false },
          }),
        };
      await req.respond({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(body),
      });
    });
    await page.goto(`${base}/model/${encodeURIComponent(name)}`, {
      waitUntil: 'networkidle0',
      timeout: 45000,
    });
    await page.waitForSelector('#official-pricing', { timeout: 20000 });
    await page.click('#official-pricing .model-section-help');
    await page.waitForSelector('.model-section-help-content');
    const text = await page.$eval('body', (e) => e.innerText);
    assert(!/openrouter|venice|muzeai|gettoken/i.test(text));
    const links = await page.$$eval('a[href]', (els) => els.map((e) => e.href));
    assert(!links.some((x) => /openrouter|venice/i.test(x)));
    if (name === 'gpt-4o')
      assert((await page.$$('#official-pricing tbody tr')).length > 0);
    else {
      assert(text.includes('暂无官方参考价'));
      assert.equal((await page.$$('#official-pricing tbody tr')).length, 0);
    }
    assert.deepEqual(errors, []);
    await page.screenshot({ path: `/tmp/public-provenance-${width}.png` });
    await page.close();
    console.log('PASS', name, width, 'price tooltip, tiers and links');
  }
} finally {
  await browser?.close();
  await server.close();
  stopEsbuild();
  console.log('Browser/server closed; no retained test processes.');
}
