import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';
import { createServer, loadConfigFromFile } from 'vite';
import { stop as stopEsbuild } from 'esbuild';
import { officialPricingUnavailable } from '../src/helpers/modelOfficialPricing.js';

// Read-only output of TestOfficialPricingCatalogCoverage; no production writes.
const report = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const reviews = JSON.parse(
  fs.readFileSync(
    new URL('../../model/data/official_pricing_reviews.json', import.meta.url),
  ),
);
const root = fileURLToPath(new URL('..', import.meta.url));
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
const data = Object.entries(reviews).map(([name, review]) => ({
  model_name: name,
  enable_groups: ['Public'],
  supported_endpoint_types: ['openai'],
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 2,
  official_pricing: report.prices[name] || {},
  official_pricing_review: report.prices[name] ? undefined : review,
}));
data.push({
  model_name: 'free-manufacturer-fixture',
  enable_groups: ['Public'],
  supported_endpoint_types: ['openai'],
  quota_type: 0,
  model_ratio: 0,
  completion_ratio: 0,
  official_pricing: {
    currency: 'USD',
    tiers: [{ label: 'Standard', unit: 'million_tokens', input: 0, output: 0 }],
  },
  official_pricing_review: {
    code: 'source_available',
    checked_at: '2026-11-15',
  },
});
const cases = [
  'gemini-4-carbon',
  'gemini-4-hydrogen',
  'gemini-omni-1.2-flash',
  'deepseek-v4.1-flash-1017',
  'glimpse',
  'qwen-turbo',
  'gpt-6.2-sol',
  'flint-v1.0:free',
  'muse-video',
  'free-manufacturer-fixture',
];
let browser;
try {
  browser = await puppeteer.launch({
    headless: true,
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  });
  for (const width of [1440, 390]) {
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
          data,
          group_ratio: { Public: 1 },
          usable_group: { Public: 'Public' },
          vendors: [],
          supported_endpoint: {
            openai: { path: '/v1/chat/completions', method: 'POST' },
          },
        };
      if (u.pathname === '/api/status')
        body.data = {
          system_name: 'Meta API',
          quota_per_unit: 500000,
          quota_display_type: 'USD',
          usd_exchange_rate: 1,
          price: 2,
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
    for (const name of cases) {
      await page.goto(`${base}/model/${encodeURIComponent(name)}`, {
        waitUntil: 'networkidle0',
        timeout: 45000,
      });
      await page.waitForSelector('#official-pricing');
      const m = data.find((m) => m.model_name === name);
      const text = await page.$eval(
        '#official-pricing',
        (el) => el.textContent,
      );
      if (m.official_pricing.tiers?.length) {
        assert.equal(
          await page.$$eval('#official-pricing tbody tr', (els) => els.length),
          m.official_pricing.tiers.length,
          name,
        );
        assert(!text.includes('待核实') && !text.includes('待同步'), name);
        if (name === 'gemini-omni-1.2-flash') {
          assert(text.includes('M video tokens'));
          assert(text.includes('$12'));
          assert(text.includes('$9'));
        }
        if (name === 'free-manufacturer-fixture') assert(text.includes('$0'));
      } else {
        const message = officialPricingUnavailable(m.official_pricing_review);
        assert(text.includes(message), name);
        assert(text.includes('2026-11-15'), name);
        assert(
          (
            await page.$eval('.model-group-row', (el) => el.textContent)
          ).includes(message),
          name,
        );
      }
      assert(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth + 1,
        ),
        name + ' overflow',
      );
      if (
        ['gemini-omni-1.2-flash', 'qwen-turbo', 'flint-v1.0:free'].includes(
          name,
        )
      ) {
        await page.$eval('#official-pricing', (el) =>
          el.scrollIntoView({ block: 'start' }),
        );
        await page.screenshot({
          path: `/tmp/official-gap-${name.replaceAll(':', '-')}-${width}.png`,
        });
      }
      console.log('PASS', name, width);
    }
    assert.deepEqual(errors, []);
    await page.close();
  }
} finally {
  await browser?.close();
  await server.close();
  await stopEsbuild();
}
