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
    server: { host: '127.0.0.1', port: 0, open: false, proxy: {} },
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
    writes = [];
  page.on('pageerror', (e) => errors.push(e.message));
  let enabled = false,
    verified = true,
    fail = false,
    delay = 0,
    linked = false,
    entries = [];
  const hash = 'a'.repeat(64),
    lot = {
      id: 1,
      user_id: 7,
      source_type: 'topup',
      source_ref: 'receipt-qa',
      total_quota: 5000000,
      remaining_quota: 2500000,
      created_at: 1793534400,
      receipt_id: 0,
      receipt: null,
    };
  const metric = (name, id = 1) => ({
    name,
    income: verified && enabled ? id * 10 : null,
    cost: verified ? 5 : null,
    profit: verified && enabled ? id * 10 - 5 : null,
    margin: verified && enabled ? 50 : null,
    known_income: enabled ? id * 10 : 0,
    known_cost: 5,
    requests: 3,
    status: verified && enabled ? 'verified' : 'pending',
  });
  await page.addInitScript(() =>
    localStorage.setItem('user', JSON.stringify({ id: 1, role: 100 })),
  );
  await page.route('**/*', async (route) => {
    const req = route.request(),
      url = new URL(req.url()),
      p = url.pathname;
    if (url.origin !== base) return route.abort();
    if (!p.startsWith('/api/')) return route.continue();
    let data = {};
    if (req.method() === 'GET' && p.endsWith('/cash/report')) {
      const id = Number(p.split('/')[3]);
      data = {
        config: { enabled_at: enabled ? 1 : 0 },
        summary: metric('total', id),
        bindings: linked ? [{ key_hash: hash, site_only: true }] : [],
        days: [metric('2026-11-01', id)],
        models: Array.from({ length: 23 }, (_, i) =>
          metric(
            i === 0 ? '=formula' : `model-${String(i).padStart(2, '0')}`,
            id,
          ),
        ),
        channels: [],
        issues:
          verified && enabled
            ? []
            : [{ code: 'funding_unknown', message: '缺少到账凭证', count: 1 }],
        cash_flow: {
          site_confirmed_receipts: lot.receipt ? 20 : 0,
          account_payments: entries.length * 7,
          pending_funding_lots: lot.receipt ? 0 : 1,
        },
        sync: { last_success: 1793616000 },
        quota_per_unit: 500000,
      };
      if (delay) await new Promise((r) => setTimeout(r, id === 1 ? delay : 1));
      return route.fulfill({
        json: fail
          ? { success: false, message: 'synthetic report failure' }
          : { success: true, data },
      });
    }
    if (req.method() === 'GET' && p.endsWith('/cash/bindings'))
      data = [
        {
          key_hash: hash,
          channels: [{ id: 3, name: 'QA channel' }],
          binding: linked ? { site_only: true } : null,
        },
      ];
    else if (req.method() === 'GET' && p.endsWith('/cash/entries'))
      data = { items: entries, total: entries.length };
    else if (req.method() === 'GET' && p === '/api/cash_accounting/funding') {
      const items =
        url.searchParams.get('pending') === 'true' && lot.receipt ? [] : [lot];
      data = { items, total: items.length, quota_per_unit: 500000 };
    } else if (req.method() === 'GET' && p.endsWith('/receipts'))
      data = {
        items: lot.receipt ? [lot.receipt] : [],
        total: lot.receipt ? 1 : 0,
      };
    else if (req.method() === 'POST') {
      const body = req.postDataJSON();
      writes.push({ path: p, body });
      if (p.endsWith('/enable')) enabled = true;
      else if (p.endsWith('/cash/bindings')) linked = true;
      else if (p.endsWith('/receipt')) {
        lot.receipt_id = 1;
        lot.receipt = {
          ...body,
          id: 1,
          net_cny_micros: 20000000,
          created_by: 1,
          created_at: 1793616000,
        };
      } else if (p.endsWith('/cash/entries')) {
        data = {
          ...body,
          id: 1,
          credits_nanos: 1000000000,
          cost_cny_micros: 7000000,
        };
        entries.push(data);
      } else if (p.endsWith('/void')) {
        entries[0].voided_at = 1793616000;
        entries[0].voided_by = 1;
        entries[0].void_reason = body.reason;
      } else if (!p.endsWith('/sync')) throw Error(`Unexpected write: ${p}`);
    } else
      return route.fulfill({
        status: 404,
        json: { success: false, message: `Unexpected path: ${p}` },
      });
    await route.fulfill({ json: { success: true, data } });
  });
  const button = (n) =>
      n === '保存'
        ? page.getByRole('button').filter({ hasText: /^保存$/ })
        : page.getByRole('button', { name: n, exact: true }),
    textbox = (n) => page.getByRole('textbox', { name: n, exact: true }),
    tab = (n) => page.getByRole('tab', { name: n, exact: true });
  await page.goto(`${base}/${path.basename(fixture)}/index.html`);
  await page.evaluate(() =>
    window.qa.setAccounts([
      { id: 1, name: 'Cash QA', account_type: 'openrouter', enabled: true },
    ]),
  );
  await button('启用现金收益').click();
  await button('保存').click();
  await page.getByText('¥10.00', { exact: true }).first().waitFor();
  assert.equal(await page.getByText('model-22', { exact: true }).count(), 0);
  await page.locator('.semi-page-item').filter({ hasText: /^2$/ }).click();
  await page.getByText('model-22', { exact: true }).waitFor();
  await textbox('搜索').fill('=formula');
  await page.getByText('=formula', { exact: true }).waitFor();
  const downloading = page.waitForEvent('download');
  await button('导出 CSV').click();
  const download = await downloading;
  assert.ok(
    (await fs.readFile(await download.path(), 'utf8')).includes("'=formula"),
  );
  await textbox('搜索').fill('');
  await tab('待核对').click();
  await button('核对可关联渠道').click();
  await button('关联').click();
  await page.getByRole('switch', { name: '仅用于本站', exact: true }).click();
  await button('保存').click();
  await button('录入上游账单').click();
  await textbox('OpenRouter credits (USD)').fill('1');
  await textbox('账单编号').fill('upstream-qa-1');
  await textbox('原币金额（不含手续费）').fill('7');
  await textbox('凭证引用').fill('bank-qa');
  await button('保存').click();
  await page.getByText('upstream-qa-1', { exact: true }).waitFor();
  assert.equal(
    writes.filter((w) => w.path.endsWith('/cash/entries')).length,
    1,
  );
  await button('核实').last().click();
  await textbox('原币金额（不含手续费）').fill('20');
  await textbox('凭证引用').fill('settlement-qa');
  await button('保存').click();
  await page
    .getByText('receipt-qa', { exact: true })
    .waitFor({ state: 'hidden' });
  await page.getByRole('switch', { name: '仅待核实', exact: true }).click();
  await button('审计记录').click();
  await page.getByText('settlement-qa', { exact: true }).waitFor();
  await page.keyboard.press('Escape');
  await button('作废').click();
  await textbox('作废原因').fill('duplicate-proof');
  await button('保存').click();
  await page.getByText(/duplicate-proof/).waitFor();
  await tab('现金流').click();
  await page.getByText('全站客户净到账', { exact: true }).waitFor();
  await tab('现金收益').click();
  await page.getByText('¥10.00', { exact: true }).first().waitFor();
  await page.waitForFunction(() => !document.querySelector('.semi-toast'));
  assert.ok(
    (await page.getByText('OpenRouter 现金收益', { exact: true }).boundingBox())
      .y < 350,
    'excessive tab content gap',
  );
  await page.screenshot({
    path: path.join(output, 'cash-profit-desktop.png'),
    fullPage: true,
  });
  verified = false;
  await button('刷新').click();
  await page.getByRole('button', { name: /查看待核对项/ }).waitFor();
  assert.equal(await page.getByText('50.00%', { exact: true }).count(), 0);
  fail = true;
  await button('刷新').click();
  await page.getByText('synthetic report failure', { exact: true }).waitFor();
  assert.equal(await page.getByText('¥10.00', { exact: true }).count(), 0);
  fail = false;
  verified = true;
  delay = 800;
  await button('刷新').click();
  await page.evaluate(() =>
    window.qa.setAccounts([
      {
        id: 2,
        name: 'Other account',
        account_type: 'openrouter',
        enabled: true,
      },
    ]),
  );
  await page.getByText('¥20.00', { exact: true }).first().waitFor();
  await page.waitForTimeout(900);
  assert.equal(await page.getByText('¥10.00', { exact: true }).count(), 0);
  await page.waitForFunction(() => !document.querySelector('.semi-toast'));
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({
    path: path.join(output, 'cash-profit-mobile.png'),
    fullPage: true,
  });
  assert.ok(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
    'mobile overflow',
  );
  await page.evaluate(() =>
    localStorage.setItem('user', JSON.stringify({ id: 2, role: 10 })),
  );
  await tab('待核对').click();
  assert.equal(await button('录入上游账单').count(), 0);
  assert.equal(await button('核实').count(), 0);
  await page.screenshot({
    path: path.join(output, 'cash-review-admin-mobile.png'),
    fullPage: true,
  });
  assert.ok(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
    'review mobile overflow',
  );
  assert.deepEqual(errors, []);
  const receipt = writes.find((w) => w.path.endsWith('/receipt')).body;
  assert.equal(receipt.gross_amount, '20');
  assert.equal(receipt.currency, 'CNY');
  assert.equal(typeof receipt.received_at, 'number');
  assert.ok(receipt.request_id);
  console.log(
    'OpenRouter cash: enable, binding, entries, receipts, audit, void, scope, paging, export, failure, race, permissions and mobile passed',
  );
} finally {
  await browser?.close();
  await server?.close();
  stopEsbuild();
  await fs.rm(fixture, { recursive: true, force: true });
}
