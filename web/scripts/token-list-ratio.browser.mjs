// Local real-component QA with synthetic API data only. No upstream calls.
// Set PLAYWRIGHT_MODULE and optionally PLAYWRIGHT_CHANNEL for a bundled runtime.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createServer, loadConfigFromFile } from 'vite';
import { stop as stopEsbuild } from 'esbuild';

const { chromium } = await import(
  process.env.PLAYWRIGHT_MODULE || 'playwright'
);
const root = fileURLToPath(new URL('..', import.meta.url));
process.chdir(root);
const fixtureDir = await fs.mkdtemp(path.join(root, '.token-ratio-qa-'));
const outputDir = await fs.mkdtemp(path.join(os.tmpdir(), 'token-ratio-qa-'));
let server;
let browser;
let page;
const errors = [];
const requests = [];
const mutations = [];

const row = (id, name, group, ratio, status = 'fixed') => ({
  id,
  user_id: 901,
  name,
  group,
  group_ratio: ratio,
  group_ratio_status: status,
  key: `mock****${id}`,
  status: 1,
  unlimited_quota: true,
  used_quota: 5000,
  remain_quota: 100000,
  created_time: 1791244800,
  accessed_time: 1791248400,
  expired_time: -1,
  model_limits: '',
});
let fixtures = [
  row(1, '普通倍率', 'OpenAI · 优质', 0.45),
  row(2, '专属倍率', 'Claude · 优质', 0.18),
  row(3, '零倍率', '免费组', 0),
  row(4, '继承用户分组', '', 0.8),
  row(5, '自动分组', 'auto', null, 'auto'),
  row(6, '失效分组', '已移除', null, 'unavailable'),
  row(7, '旧后端', '旧分组', undefined, undefined),
  row(8, '非法负数', '错误组', -1),
  row(9, '非法文本', '错误组', '0.5'),
  row(10, '空倍率', '错误组', null),
  row(11, '第二页令牌', 'OpenAI · 优质', 0.25),
];
delete fixtures[6].group_ratio;
delete fixtures[6].group_ratio_status;

try {
  await fs.writeFile(
    path.join(fixtureDir, 'index.html'),
    '<html><div id="root"></div><script type="module" src="./entry.jsx"></script></html>',
  );
  await fs.writeFile(
    path.join(fixtureDir, 'entry.jsx'),
    `import React from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import { UserContext } from '/src/context/User';
import { StatusContext } from '/src/context/Status';
import TokensPage from '/src/components/table/tokens';
import '/src/index.css';
i18n.use(initReactI18next).init({lng:'zh',resources:{zh:{translation:{}}},interpolation:{escapeValue:false}});
createRoot(document.getElementById('root')).render(
<BrowserRouter><UserContext.Provider value={[{user:{id:901,role:1,group:'default'}},()=>{}]}>
<StatusContext.Provider value={[{status:{quota_per_unit:500000}},()=>{}]}>
<main style={{padding:16}}><TokensPage/></main>
</StatusContext.Provider></UserContext.Provider></BrowserRouter>);`,
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
    // No missed API route may fall through to a running developer backend.
    plugins: [
      // The inspector opens an independent listener that Vite.close cannot
      // dispose. QA needs transforms/styles, not an editor-control service.
      ...config.plugins
        .flat(Infinity)
        .filter((plugin) => plugin?.name !== '@code-inspector/vite'),
      {
        name: 'token-ratio-qa-block-api',
        configureServer(vite) {
          vite.middlewares.use((req, res, next) => {
            if (/^\/(?:api|v1|mj|pg)\//.test(req.url || '')) {
              res.statusCode = 502;
              res.end('Unmocked request blocked by token ratio QA');
              return;
            }
            next();
          });
        },
      },
    ],
  });
  await server.listen();
  const address = server.httpServer.address();
  const baseURL = `http://127.0.0.1:${address.port}`;
  browser = await chromium.launch({
    headless: true,
    channel: process.env.PLAYWRIGHT_CHANNEL || undefined,
  });
  page = await browser.newPage({ viewport: { width: 1600, height: 1000 } });
  page.setDefaultTimeout(15000);
  page.on('pageerror', (error) => errors.push(error.message));
  await page.addInitScript(() => {
    localStorage.setItem('user', JSON.stringify({ id: 901, role: 1 }));
    localStorage.setItem('page-size', '10');
    localStorage.setItem('chats', '[]');
    localStorage.setItem(
      'table_compact_modes',
      JSON.stringify({ tokens: true }),
    );
    localStorage.setItem('quota_per_unit', '500000');
  });
  await page.route('**/*', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.origin !== baseURL) {
      if (url.protocol !== 'data:')
        errors.push(`Unexpected external request: ${url.origin}`);
      await route.abort();
      return;
    }
    if (!url.pathname.startsWith('/api/')) {
      await route.continue();
      return;
    }
    requests.push(url.pathname);
    if (request.method() !== 'GET') mutations.push(url.pathname);
    let data = [];
    if (
      url.pathname === '/api/token/' ||
      url.pathname === '/api/token/search'
    ) {
      const keyword = url.searchParams.get('keyword') || '';
      const group = url.searchParams.get('group') || '';
      const selected = fixtures.filter(
        (token) =>
          (!keyword || token.name === keyword) &&
          (!group || token.group === group),
      );
      const current = Number(url.searchParams.get('p') || 1);
      const size = Number(url.searchParams.get('size') || 10);
      data = {
        items: selected.slice((current - 1) * size, current * size),
        total: selected.length,
        page: current,
        page_size: size,
      };
    } else if (url.pathname === '/api/group/') {
      data = ['OpenAI · 优质', 'Claude · 优质', '免费组', 'auto'];
    } else if (url.pathname === '/api/health/groups') {
      data = { enabled: false, groups: [] };
    } else if (url.pathname === '/api/user/self/groups') {
      data = {};
    }
    await route.fulfill({ json: { success: true, data } });
  });
  await page.goto(`${baseURL}/${path.basename(fixtureDir)}/index.html`);
  const headers = page.locator('thead th');
  await page.getByRole('columnheader', { name: '倍率', exact: true }).waitFor();
  for (const label of ['显示全部列', '紧凑列表', '自适应列表']) {
    assert.equal(await page.getByText(label, { exact: true }).count(), 0);
  }
  assert.equal(
    await page.getByRole('heading', { name: '令牌管理' }).count(),
    0,
  );
  assert.equal(await page.locator('.tokens-page-title').count(), 0);
  const titles = await headers.allTextContents();
  for (const title of [
    '名称',
    '密钥',
    '可用模型',
    'IP限制',
    '运行限制',
    '创建时间',
    '最后使用时间',
    '过期时间',
  ])
    assert(titles.includes(title), title);

  const ratioIndex = titles.indexOf('倍率');
  assert.equal(ratioIndex, titles.indexOf('分组') + 1);
  const tokenRow = (name) => page.locator('tbody tr').filter({ hasText: name });
  const ratioCell = (name) => tokenRow(name).locator('td').nth(ratioIndex);
  for (const [name, expected] of [
    ['普通倍率', '0.45x'],
    ['专属倍率', '0.18x'],
    ['零倍率', '0x'],
    ['继承用户分组', '0.8x'],
    ['自动分组', '自动'],
    ['失效分组', '—'],
    ['旧后端', '—'],
    ['非法负数', '—'],
    ['非法文本', '—'],
    ['空倍率', '—'],
  ]) {
    await ratioCell(name).getByText(expected, { exact: true }).waitFor();
  }
  console.log(
    'PASS column order, ordinary/special/zero/inherited/auto/unknown/legacy/invalid values',
  );
  assert.equal(
    await page.getByRole('button', { name: '批量启用', exact: true }).count(),
    0,
  );
  await tokenRow('普通倍率').getByRole('checkbox').focus();
  await page.keyboard.press('Space');
  await page.getByRole('group', { name: '批量操作', exact: true }).waitFor();
  for (const label of ['批量启用', '批量禁用', '复制所选令牌', '删除所选令牌'])
    assert(
      await page.getByRole('button', { name: label, exact: true }).isEnabled(),
    );
  const actionBox = await page
    .getByRole('button', { name: '添加令牌', exact: true })
    .boundingBox();
  const filterBox = await page
    .locator('.tokens-toolbar__filters')
    .boundingBox();
  assert(
    Math.abs(filterBox.y - actionBox.y) < 2,
    'creation and filters share a row even with a batch selection',
  );
  await page.screenshot({
    path: path.join(outputDir, 'selected-toolbar.png'),
    fullPage: true,
  });
  await tokenRow('普通倍率').getByRole('checkbox').focus();
  await page.keyboard.press('Space');
  assert.equal(
    await page.getByRole('group', { name: '批量操作', exact: true }).count(),
    0,
  );
  console.log(
    'PASS batch controls appear only for selection; creation and filters share a row',
  );
  await ratioCell('普通倍率').hover();
  await page.getByText(/当前分组倍率，已包含适用的专属倍率/).waitFor();
  await page.mouse.move(0, 0);
  await page.screenshot({
    path: path.join(outputDir, 'desktop.png'),
    fullPage: true,
  });
  console.log(
    'PASS explanation distinguishes group multiplier from model price and time multiplier',
  );

  await page.locator('.semi-page-item').filter({ hasText: /^2$/ }).click();
  await ratioCell('第二页令牌').getByText('0.25x', { exact: true }).waitFor();
  assert.equal(await tokenRow('普通倍率').count(), 0);
  await page.getByPlaceholder('搜索关键字').fill('专属倍率');
  await page.getByRole('button', { name: '查询', exact: true }).click();
  await ratioCell('专属倍率').getByText('0.18x', { exact: true }).waitFor();
  assert.equal(await page.locator('tbody tr').count(), 1);
  assert(requests.includes('/api/token/search'));
  console.log(
    'PASS pagination and search use the ratio belonging to the returned token',
  );

  fixtures = fixtures.map((token) =>
    token.id === 2 ? { ...token, group_ratio: 0.22 } : token,
  );
  await page.getByRole('button', { name: '查询', exact: true }).click();
  await ratioCell('专属倍率').getByText('0.22x', { exact: true }).waitFor();
  console.log('PASS refresh replaces the prior multiplier');
  await page.getByPlaceholder('搜索关键字').fill('没有这个令牌');
  await page.getByRole('button', { name: '查询', exact: true }).click();
  await page.getByText('搜索无结果', { exact: true }).waitFor();
  await page.getByRole('button', { name: '重置', exact: true }).click();
  await ratioCell('普通倍率').getByText('0.45x', { exact: true }).waitFor();
  console.log('PASS empty result and reset');

  await page.setViewportSize({ width: 390, height: 844 });
  await page.locator('table').waitFor({ state: 'detached' });
  assert.equal(
    await page.getByRole('button', { name: /显示操作项/ }).count(),
    0,
  );
  await page.getByRole('button', { name: /筛选/ }).click();
  await page.getByRole('button', { name: '添加令牌', exact: true }).waitFor();
  const mobileFilters = await page
    .locator('.tokens-toolbar__filters')
    .boundingBox();
  assert(
    mobileFilters &&
      mobileFilters.x >= 0 &&
      mobileFilters.x + mobileFilters.width <= 390,
  );
  const mobileKeyword = await page.getByPlaceholder('搜索关键字').boundingBox();
  const mobileKey = await page
    .getByPlaceholder('密钥', { exact: true })
    .boundingBox();
  assert(
    mobileKey.y >= mobileKeyword.y + mobileKeyword.height,
    'mobile filters must stack',
  );
  await page.getByPlaceholder('搜索关键字').fill('普通倍率');
  await page
    .locator('.tokens-filter-row__mobile-actions')
    .getByRole('button', { name: '查询', exact: true })
    .click();
  await page.getByRole('button', { name: '重置', exact: true }).click();
  const firstCard = page
    .locator('.semi-card')
    .filter({
      has: page.getByText('普通倍率', { exact: true }),
    })
    .last();
  await firstCard.getByText('0.45x', { exact: true }).waitFor();
  const mobileTitles = await firstCard
    .locator('.semi-card-body > div > span')
    .allTextContents();
  assert.equal(mobileTitles.indexOf('倍率'), mobileTitles.indexOf('分组') + 1);
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
    'mobile page must not overflow horizontally',
  );
  await page.screenshot({
    path: path.join(outputDir, 'mobile.png'),
    fullPage: true,
  });
  console.log(
    'PASS mobile card shows the same multiplier without horizontal overflow',
  );
  await firstCard.locator('.token-mobile-select').click();
  await page.getByRole('group', { name: '批量操作', exact: true }).waitFor();
  await firstCard.locator('.token-mobile-select').click();
  await page.getByRole('button', { name: /高级筛选/ }).click();
  await page.locator('.semi-sidesheet-inner').waitFor();
  await page.keyboard.press('Escape');
  await page.setViewportSize({ width: 320, height: 844 });
  await page.evaluate(() => document.body.setAttribute('theme-mode', 'dark'));
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
    '320px no page overflow',
  );
  await page.screenshot({
    path: path.join(outputDir, 'mobile-dark-320.png'),
    fullPage: true,
  });
  assert.deepEqual(
    mutations,
    [],
    'reading the multiplier must not mutate any token',
  );
  assert(
    !requests.some((p) => p.endsWith('/key')),
    'must not fetch full token keys',
  );
  assert.deepEqual(errors, []);
  console.log(
    `PASS no browser errors, API mutations or key fetches; screenshots: ${outputDir}`,
  );
} catch (error) {
  if (page) {
    await page
      .screenshot({ path: path.join(outputDir, 'failure.png'), fullPage: true })
      .catch(() => {});
  }
  console.error(`QA failed; artifacts: ${outputDir}`);
  throw error;
} finally {
  await Promise.allSettled([browser?.close(), server?.close()]);
  await stopEsbuild();
  await fs.rm(fixtureDir, { recursive: true, force: true });
  console.log('CLEANUP browser and Vite closed; temporary fixture removed');
}
