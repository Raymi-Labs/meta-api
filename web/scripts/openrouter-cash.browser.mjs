// Isolated real-component regression; all API requests use synthetic data.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createServer, loadConfigFromFile } from 'vite';
import { stop as stopEsbuild } from 'esbuild';
const { chromium } = await import(
  process.env.PLAYWRIGHT_MODULE || 'playwright'
);
const root = fileURLToPath(new URL('..', import.meta.url));
const fixture = await fs.mkdtemp(path.join(root, '.cash-qa-'));
const output = path.join(root, '../output/playwright/openrouter-cash');
let server, browser;
try {
  await fs.mkdir(output, { recursive: true });
  await fs.writeFile(
    path.join(fixture, 'index.html'),
    '<html><div id="root"></div><script type="module" src="./entry.jsx"></script></html>',
  );
  await fs.writeFile(
    path.join(fixture, 'entry.jsx'),
    `
import React,{useState} from 'react';import {createRoot} from 'react-dom/client';
import i18n from 'i18next';import {initReactI18next} from 'react-i18next';
import Dashboard from '/src/pages/UpstreamAccounts/components/OpenRouterAccountDashboard';
import {API} from '/src/helpers';import '@douyinfe/semi-ui/dist/css/semi.min.css';import '/src/index.css';
i18n.use(initReactI18next).init({lng:'zh-CN',resources:{'zh-CN':{translation:{}}},interpolation:{escapeValue:false}});
function App(){const [accounts,setAccounts]=useState([]),[active,setActive]=useState(true);
window.qa={setAccounts,setActive};
return <main style={{padding:16}}><Dashboard accounts={accounts} reloadAccounts={async()=>{const res=await API.get('/api/upstream_accounts');setAccounts(res.data.data)}} active={active} t={i18n.t.bind(i18n)}/></main>}
createRoot(document.getElementById('root')).render(<App/>);`,
  );
  const { config } = await loadConfigFromFile(
    { command: 'serve', mode: 'development' },
    path.join(root, 'vite.config.js'),
  );
  server = await createServer({
    ...config,
    configFile: false,
    root,
    // Fixture is immutable; filesystem sync events must not reload an active test.
    server: {
      host: '127.0.0.1',
      port: 0,
      open: false,
      proxy: {},
      watch: null,
      hmr: false,
    },
    plugins: [
      ...config.plugins
        .flat(Infinity)
        .filter((p) => p?.name !== '@code-inspector/vite'),
      {
        name: 'deny-api',
        configureServer(v) {
          v.middlewares.use((req, res, next) => {
            if (req.url?.startsWith('/api/')) {
              res.statusCode = 502;
              res.end('unmocked API');
              return;
            }
            next();
          });
        },
      },
    ],
  });
  await server.listen();
  const base = `http://127.0.0.1:${server.httpServer.address().port}`;
  browser = await chromium.launch({
    headless: true,
    channel: process.env.PLAYWRIGHT_CHANNEL || undefined,
  });
  const page = await browser.newPage({
    viewport: { width: 1440, height: 1000 },
  });
  page.setDefaultTimeout(15000);
  const errors = [],
    requests = [];
  page.on('pageerror', (e) => errors.push(e.message));
  let pending = true,
    fail = false,
    delay = 0,
    partial = false;
  await page.addInitScript(() => {
    localStorage.setItem('user', JSON.stringify({ id: 1, role: 10 }));
    localStorage.setItem('quota_per_unit', '500000');
    localStorage.setItem('quota_display_type', 'CUSTOM');
    localStorage.setItem(
      'status',
      JSON.stringify({
        custom_currency_symbol: '⚡',
        custom_currency_exchange_rate: 1,
      }),
    );
  });
  await page.route('**/*', async (route) => {
    const req = route.request(),
      url = new URL(req.url()),
      p = url.pathname;
    if (url.origin !== base) return route.abort();
    if (!p.startsWith('/api/')) return route.continue();
    requests.push(p);
    assert.ok(p.endsWith('/openrouter/finance'), 'unexpected API ' + p);
    assert.equal(req.method(), 'GET');
    if (pending) {
      pending = false;
      return route.fulfill({
        json: { success: true, data: { refreshing: true } },
      });
    }
    const id = Number(p.split('/')[3]);
    const data = {
      fetched_at: 1793616000,
      start_date: '2026-10-15',
      end_date: '2026-11-13',
      channel_activity: Array.from({ length: 23 }, (_, i) => ({
        date: '2026-11-13',
        channel_id: i + 1,
        channel_name: `channel-${id}-${String(i).padStart(2, '0')}`,
        quota: id * 500000,
        requests: id * 10,
      })),
      site_activity: [
        {
          date: '2026-11-13',
          model: '=formula',
          quota: id * 500000 * 23,
          requests: id * 10 * 23,
        },
      ],
      activity: [
        { date: '2026-11-13', model: '=formula', usage: 5, requests: 10 },
      ],
      credits: { total_credits: id * 100, total_usage: 40 },
      credits_error: partial ? 'synthetic upstream failure' : '',
    };
    if (delay) await new Promise((r) => setTimeout(r, id === 1 ? delay : 1));
    return route.fulfill({
      json: fail
        ? { success: false, message: 'synthetic report failure' }
        : { success: true, data },
    });
  });
  const button = (n) => page.getByRole('button', { name: n, exact: true });
  const tab = (n) => page.getByRole('tab', { name: n, exact: true });
  await page.goto(`${base}/${path.basename(fixture)}/index.html`);
  await page.evaluate(() =>
    window.qa.setAccounts([
      { id: 1, name: 'Account', account_type: 'openrouter', enabled: true },
    ]),
  );
  assert.equal(await tab('现金收益').count(), 0);
  assert.equal(await tab('现金流').count(), 0);
  assert.equal(await tab('模型消耗看板').count(), 0);
  await page
    .getByText('正在后台准备快照，完成后自动显示', { exact: true })
    .waitFor();
  await page.getByText('channel-1-00 (#1)', { exact: true }).waitFor();
  assert.equal(await button('启用现金收益').count(), 0);
  assert.equal(await tab('待核对').count(), 0);
  await page.locator('.semi-page-item').filter({ hasText: /^2$/ }).click();
  await page.getByText('channel-1-22 (#23)', { exact: true }).waitFor();
  await page
    .getByRole('textbox', { name: '搜索', exact: true })
    .fill('channel-1-00');
  await page.getByText('channel-1-00 (#1)', { exact: true }).waitFor();
  const downloading = page.waitForEvent('download');
  await button('导出 CSV').click();
  const csv = await fs.readFile(await (await downloading).path(), 'utf8');
  assert.ok(csv.includes('⚡1.000000'));
  assert.ok(
    csv.includes('本站消耗') && !csv.includes('利润') && !csv.includes('核实'),
  );
  const before = requests.filter((p) =>
    p.endsWith('/openrouter/finance'),
  ).length;
  await button('按模型').click();
  await page.getByText('=formula', { exact: true }).waitFor();
  const modelDownload = page.waitForEvent('download');
  await button('导出 CSV').click();
  assert.ok(
    (await fs.readFile(await (await modelDownload).path(), 'utf8')).includes(
      "'=formula",
    ),
  );
  await button('按日期').click();
  await page.getByText('2026-11-13', { exact: true }).waitFor();
  await button('7 天').click();
  await page.getByLabel('开始日期', { exact: true }).fill('2026-11-08');
  await page.getByText('⚡23.000000', { exact: true }).first().waitFor();
  await button('30 天').click();
  await button('按渠道').click();
  await page.getByText('$100.000000', { exact: true }).waitFor();
  assert.equal(
    requests.filter((p) => p.endsWith('/openrouter/finance')).length,
    before,
  );
  assert.equal(
    await page.getByText('全站客户净到账', { exact: true }).count(),
    0,
  );
  partial = true;
  await button('刷新').click();
  await page.getByText('synthetic upstream failure', { exact: true }).waitFor();
  fail = true;
  await button('刷新').click();
  await page.getByText('synthetic report failure', { exact: true }).waitFor();
  await page.getByText('$100.000000', { exact: true }).waitFor();
  fail = false;
  partial = false;
  delay = 600;
  await button('刷新').click();
  await page.evaluate(() =>
    window.qa.setAccounts([
      { id: 2, name: 'Other', account_type: 'openrouter', enabled: true },
    ]),
  );
  await page.getByText('$200.000000', { exact: true }).waitFor();
  await page.waitForTimeout(700);
  assert.equal(await page.getByText('$100.000000', { exact: true }).count(), 0);
  await page.screenshot({
    path: path.join(output, 'unified-desktop.png'),
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  assert.equal(await tab('现金收益').count(), 0);
  assert.equal(await tab('现金流').count(), 0);
  assert.equal(await tab('模型消耗看板').count(), 0);
  await page.getByRole('textbox', { name: '搜索', exact: true }).fill('');
  await page.getByText('channel-2-00 (#1)', { exact: true }).waitFor();
  assert.ok(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  );
  await page.screenshot({
    path: path.join(output, 'unified-mobile.png'),
    fullPage: true,
  });
  assert.ok(!(await page.locator('body').innerText()).includes('NaN'));
  assert.ok((await page.getByText('⚡2.000000', { exact: true }).count()) > 0);
  assert.deepEqual(errors, []);
  assert.equal(
    requests.some(
      (p) =>
        p.includes('/funding') ||
        p.endsWith('/entries') ||
        p.endsWith('/bindings'),
    ),
    false,
  );
  console.log(
    'PASS: unified page, pending polling, channel/model/date and range reuse, pagination/search/CSV, credits, failures, account isolation, mobile',
  );
} finally {
  await browser?.close();
  await server?.close();
  stopEsbuild();
  await fs.rm(fixture, { recursive: true, force: true });
}
