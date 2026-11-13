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
const fixture = await fs.mkdtemp(path.join(root, '.openrouter-qa-'));
const output = path.join(root, '../output/playwright/openrouter');
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
    saved = [];
  let accounts = [],
    mode = 'normal',
    delay = 0;
  page.on('pageerror', (e) => errors.push(e.message));
  const rows = Array.from({ length: 25 }, (_, i) => ({
    date: '2026-11-01',
    model: `model-${String(i).padStart(2, '0')}`,
    provider_name: 'Provider A',
    usage: 0.1,
    byok_usage_inference: 5,
    requests: 2,
    prompt_tokens: 10,
    completion_tokens: 20,
  }));
  await page.route('**/*', async (route) => {
    const req = route.request(),
      url = new URL(req.url());
    if (url.origin !== base) {
      await route.abort();
      return;
    }
    if (!url.pathname.startsWith('/api/')) {
      await route.continue();
      return;
    }
    if (url.pathname.endsWith('/cash/report')) {
      await route.fulfill({
        json: { success: false, message: 'Cash tested separately' },
      });
      return;
    }
    if (url.pathname.endsWith('/openrouter/finance')) {
      const id = Number(url.pathname.split('/')[3]);
      const selectedMode = mode;
      if (delay) await new Promise((resolve) => setTimeout(resolve, delay));
      if (selectedMode === 'pending') {
        mode = 'normal';
        await route.fulfill({
          json: { success: true, data: { refreshing: true } },
        });
        return;
      }
      if (selectedMode === 'failure') {
        await route.fulfill({
          json: { success: false, message: 'synthetic failure' },
        });
        return;
      }
      await route.fulfill({
        json: {
          success: true,
          data: {
            credits: { total_credits: id === 2 ? 200 : 50, total_usage: 10 },
            activity:
              selectedMode === 'partial'
                ? null
                : selectedMode === 'empty'
                  ? []
                  : rows,
            activity_error:
              selectedMode === 'partial'
                ? 'OpenRouter HTTP 403: Management Key required'
                : '',
            start_date: '2026-10-03',
            end_date: '2026-11-01',
            site_activity: rows.map((r) => ({
              date: r.date,
              model: r.model,
              quota: 500000,
              requests: 1,
            })),
            fetched_at: 1793616000,
          },
        },
      });
      return;
    }
    if (req.method() === 'POST' || req.method() === 'PUT') {
      const body = req.postDataJSON();
      saved.push(body);
      const item = {
        ...body,
        id: body.id || accounts.length + 1,
        access_token: undefined,
        access_token_masked: 'test***',
      };
      accounts = body.id
        ? accounts.map((a) => (a.id === body.id ? item : a))
        : [...accounts, item];
      await route.fulfill({ json: { success: true, data: item } });
      return;
    }
    if (req.method() === 'DELETE') {
      accounts = accounts.filter(
        (a) => a.id !== Number(url.pathname.split('/').at(-1)),
      );
      await route.fulfill({ json: { success: true } });
      return;
    }
    await route.fulfill({ json: { success: true, data: accounts } });
  });
  await page.addInitScript(() => {
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
  await page.goto(`${base}/${path.basename(fixture)}/index.html`);
  await page
    .getByText('添加 OpenRouter Management Key 后查看充值、余额和消费明细', {
      exact: true,
    })
    .waitFor();
  await page
    .getByRole('button', { name: '新增 OpenRouter 账户', exact: true })
    .click();
  await page
    .getByRole('textbox', { name: '账户名称', exact: true })
    .fill('Primary');
  await page
    .getByRole('textbox', { name: 'Management Key', exact: true })
    .fill('synthetic-test-key');
  await page
    .getByRole('button')
    .filter({ hasText: /^保存$/ })
    .click();
  await page.getByRole('tab', { name: '模型消耗看板', exact: true }).click();
  await page.getByText('$40.000000', { exact: true }).waitFor();
  await page.getByText('⚡25.000000', { exact: true }).waitFor();
  await page.getByRole('button', { name: '按日期', exact: true }).click();
  await page.getByText('2026-11-01', { exact: true }).waitFor();
  await page.getByRole('button', { name: '按模型', exact: true }).click();
  assert.equal(saved[0].account_type, 'openrouter');
  assert.equal(await page.getByText('$2.500000', { exact: true }).count(), 1);
  await page
    .getByText('model-24', { exact: true })
    .waitFor({ state: 'hidden' });
  assert.equal(
    await page.getByText('model-24', { exact: true }).count(),
    0,
    'model24 pagination/filter',
  );
  await page.locator('.semi-page-item').filter({ hasText: /^2$/ }).click();
  await page.getByText('model-24', { exact: true }).waitFor();
  await page
    .getByRole('textbox', { name: '搜索模型', exact: true })
    .fill('model-01');
  await page.getByText('model-01', { exact: true }).waitFor();
  await page
    .getByText('model-24', { exact: true })
    .waitFor({ state: 'hidden' });
  assert.equal(
    await page.getByText('model-24', { exact: true }).count(),
    0,
    'model24 pagination/filter',
  );
  await page
    .getByRole('textbox', { name: '搜索模型', exact: true })
    .fill('missing');
  await page
    .getByText('model-01', { exact: true })
    .waitFor({ state: 'hidden' });
  assert.equal(
    await page.getByText('model-01', { exact: true }).count(),
    0,
    'model01 empty search',
  );
  await page.getByRole('textbox', { name: '搜索模型', exact: true }).fill('');
  await page.getByRole('button', { name: '编辑', exact: true }).click();
  await page
    .getByRole('textbox', { name: '账户名称', exact: true })
    .fill('Primary renamed');
  await page
    .getByRole('button')
    .filter({ hasText: /^保存$/ })
    .click();
  await page.getByText('Primary renamed', { exact: true }).waitFor();
  assert.equal(saved.at(-1).access_token, '');
  await page.getByText('$40.000000', { exact: true }).waitFor();
  await page.waitForFunction(
    () =>
      !document.querySelector('.semi-modal-mask') &&
      !document.querySelector('.semi-toast'),
  );
  await page.screenshot({
    path: path.join(output, 'desktop.png'),
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({
    path: path.join(output, 'mobile.png'),
    fullPage: true,
  });
  assert.ok(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth + 1,
    ),
    'page overflow',
  );
  await page.getByRole('button', { name: '编辑', exact: true }).click();
  await page.getByRole('switch', { name: '启用', exact: true }).click();
  await page
    .getByRole('button')
    .filter({ hasText: /^保存$/ })
    .click();
  await page.getByText('已停用', { exact: true }).waitFor();
  assert.equal(saved.at(-1).enabled, false);
  await page.getByRole('button', { name: '编辑', exact: true }).click();
  await page.getByRole('switch', { name: '启用', exact: true }).click();
  await page
    .getByRole('button')
    .filter({ hasText: /^保存$/ })
    .click();
  await page.getByText('$40.000000', { exact: true }).waitFor();
  mode = 'pending';
  await page.getByRole('button', { name: '刷新', exact: true }).click();
  await page
    .getByText('正在后台准备快照，完成后自动显示', { exact: true })
    .waitFor();
  await page.getByText('$40.000000', { exact: true }).waitFor();
  await page.getByText('后台每 5 分钟更新快照', { exact: false }).waitFor();
  mode = 'partial';
  await page.getByRole('button', { name: '刷新', exact: true }).click();
  await page
    .getByText('OpenRouter HTTP 403: Management Key required', { exact: true })
    .waitFor();
  await page.getByText('$40.000000', { exact: true }).waitFor();
  mode = 'failure';
  await page.getByRole('button', { name: '刷新', exact: true }).click();
  await page.getByText('synthetic failure', { exact: true }).waitFor();
  assert.equal(
    await page.getByText('$40.000000', { exact: true }).count(),
    1,
    'failed refresh must retain displayed credits',
  );
  mode = 'empty';
  await page.getByRole('button', { name: '刷新', exact: true }).click();
  await page.getByText('$0.000000', { exact: true }).waitFor();
  mode = 'normal';
  delay = 600;
  await page.getByRole('button', { name: '刷新', exact: true }).click();
  accounts = [
    { id: 2, name: 'Secondary', account_type: 'openrouter', enabled: true },
  ];
  await page.evaluate((a) => window.qa.setAccounts(a), accounts);
  await page.getByText('$190.000000', { exact: true }).waitFor();
  assert.equal(
    await page.getByText('$40.000000', { exact: true }).count(),
    0,
    'stale credits',
  );
  delay = 0;
  await page.getByRole('button', { name: '删除', exact: true }).click();
  await page
    .getByRole('button')
    .filter({ hasText: /^确认删除$/ })
    .click();
  await page
    .getByText('添加 OpenRouter Management Key 后查看充值、余额和消费明细', {
      exact: true,
    })
    .waitFor();
  assert.deepEqual(errors, []);
  console.log(
    'PASS: create/edit/delete, key preservation, pagination/filter reset, USD/BYOK, empty/partial/failure, account race, desktop/mobile',
    output,
  );
} finally {
  await browser?.close();
  await server?.close();
  stopEsbuild();
  await fs.rm(fixture, { recursive: true, force: true });
}
