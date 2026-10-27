// Real components, isolated synthetic APIs: no paid or production requests.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { createServer, loadConfigFromFile } from 'vite';
const { chromium } = await import(
  process.env.PLAYWRIGHT_MODULE || 'playwright'
);
const root = fileURLToPath(new URL('..', import.meta.url));
const prefix = `.token-routing-qa-${process.pid}`;
const files = [
  path.join(root, prefix + '.html'),
  path.join(root, prefix + '.jsx'),
];
const artifacts =
  process.env.TOKEN_ROUTING_ARTIFACTS_DIR ||
  fs.mkdtempSync(path.join(os.tmpdir(), 'token-routing-qa-'));
fs.mkdirSync(artifacts, { recursive: true });
let server, browser, page;
try {
  fs.writeFileSync(
    files[0],
    `<html><meta name="viewport" content="width=device-width, initial-scale=1"><div id="root"></div><script type="module" src="/${prefix}.jsx"></script></html>`,
  );
  fs.writeFileSync(
    files[1],
    `import React,{useState} from 'react';
import {createRoot} from 'react-dom/client';import {BrowserRouter} from 'react-router-dom';
import i18n from 'i18next';import {initReactI18next} from 'react-i18next';
import {UserContext} from './src/context/User';import {StatusContext} from './src/context/Status';
import EditTokenModal from './src/components/table/tokens/modals/EditTokenModal';
import TokenTestModal from './src/components/table/tokens/modals/TokenTestModal';import './src/index.css';
i18n.use(initReactI18next).init({lng:'zh',resources:{zh:{translation:{}}},interpolation:{escapeValue:false}});
function App(){const [visible,setVisible]=useState(true),[edit,setEdit]=useState({}),[test,setTest]=useState(false);
window.qa={edit:()=>{setEdit({id:99,status:1});setTest(false);setVisible(true)},create:()=>{setEdit({});setTest(false);setVisible(true)},test:()=>{setVisible(false);setTest(true)}};
return <BrowserRouter><UserContext.Provider value={[{user:{id:901,role:100}},()=>{}]}><StatusContext.Provider value={[{status:{quota_per_unit:500000,default_use_auto_group:true}},()=>{}]}><EditTokenModal visiable={visible} editingToken={edit} handleClose={()=>setVisible(false)} refresh={()=>{}}/><TokenTestModal visible={test} token={{id:99,name:'fixture'}} onCancel={()=>setTest(false)}/></StatusContext.Provider></UserContext.Provider></BrowserRouter>};createRoot(document.getElementById('root')).render(<App/>);`,
  );
  const { config } = await loadConfigFromFile(
    { command: 'serve', mode: 'development' },
    path.join(root, 'vite.config.js'),
  );
  server = await createServer({
    ...config,
    configFile: false,
    root,
    plugins: config.plugins.filter(
      (p) => !String(p?.name).includes('inspector'),
    ),
    server: { host: '127.0.0.1', port: 0, proxy: {} },
  });
  await server.listen();
  browser = await chromium.launch({
    headless: true,
    channel: process.env.PLAYWRIGHT_CHANNEL || undefined,
  });
  page = await browser.newPage({ viewport: { width: 1280, height: 960 } });
  page.setDefaultTimeout(10000);
  const errors = [],
    saves = [],
    tests = [];
  let failSave = false;
  let stored = {
    id: 99,
    name: 'fixture',
    group: 'default',
    routing_config: '',
    expired_time: -1,
    unlimited_quota: true,
    model_limits: '',
    channel_limits: '',
    auto_group_fallback: true,
  };
  page.on('pageerror', (e) => {
    errors.push(e.message);
    console.error('PAGE ERROR:', e.stack);
  });
  await page.addInitScript(() =>
    localStorage.setItem('user', JSON.stringify({ id: 901, role: 100 })),
  );
  await page.route('**/api/**', async (route) => {
    const request = route.request(),
      url = new URL(request.url());
    let data = null,
      extra = {};
    if (url.pathname === '/api/user/self/groups')
      data = {
        default: { desc: 'Primary', ratio: 1 },
        vip: { desc: 'Backup', ratio: 3 },
        team: { desc: 'Third', ratio: 5 },
        auto: { desc: 'Auto', ratio: 'auto' },
      };
    else if (url.pathname === '/api/token/models') {
      const detail = url.searchParams.get('detail') === 'true';
      const group = url.searchParams.get('group');
      const config = url.searchParams.get('routing_config');
      let names = group === 'vip' ? ['model-b'] : ['model-a'];
      if (config) {
        const groups = JSON.parse(config).groups || [];
        names = [
          ...(groups.includes('default') ? ['model-a'] : []),
          ...(groups.includes('vip') ? ['model-b'] : []),
        ];
      }
      data = detail
        ? names.map((name) => ({ name, supported_endpoint_types: ['openai'] }))
        : names;
      extra.groups = ['default', 'vip'];
    } else if (url.pathname === '/api/token/channels') data = [];
    else if (url.pathname === '/api/token/99') data = stored;
    else if (url.pathname === '/api/token/test/99')
      tests.push(request.postDataJSON());
    else if (
      url.pathname === '/api/token/' &&
      ['PUT', 'POST'].includes(request.method())
    ) {
      if (failSave) {
        await route.fulfill({
          json: { success: false, message: 'fixture save failed' },
        });
        return;
      }
      stored = { ...stored, ...request.postDataJSON() };
      saves.push(request.postDataJSON());
    } else if (url.pathname.includes('health'))
      data = { enabled: false, groups: [] };
    else data = {};
    await route.fulfill({ json: { success: true, data, ...extra } });
  });
  await page.goto(
    `http://127.0.0.1:${server.httpServer.address().port}/${prefix}.html`,
  );
  await page.locator('[data-testid="token-routing-editor"]').waitFor();
  const select = async (locator, text) => {
    await page.waitForTimeout(350);
    await locator.click();
    await page
      .locator('.semi-select-option')
      .filter({ hasText: text })
      .last()
      .click();
    await page
      .locator('.semi-modal-title:visible, .semi-sidesheet-title:visible')
      .first()
      .click();
    await page.waitForTimeout(350);
  };
  const groupSelect = () =>
    page.getByLabel('可用分组（可多选，按选择顺序路由）', { exact: true });
  await select(groupSelect(), 'default');
  await select(groupSelect(), 'vip');
  await select(page.locator('#model_limits'), 'model-a');
  await select(page.locator('#model_limits'), 'model-b');
  await select(page.getByLabel('model-a 可用分组', { exact: true }), 'default');
  await select(page.getByLabel('model-b 可用分组', { exact: true }), 'vip');
  await page.locator('#name').fill('multi-model-key');
  await page.screenshot({
    path: path.join(artifacts, 'create.png'),
    fullPage: true,
  });
  await page.getByRole('button', { name: /创建令牌/ }).click();
  await page.waitForFunction(
    () => !document.querySelector('.semi-modal-content'),
  );
  assert.equal(saves.length, 1);
  assert.equal(saves[0].group, 'default');
  assert.equal(saves[0].model_limits, 'model-a,model-b');
  assert.deepEqual(JSON.parse(saves[0].routing_config), {
    groups: ['default', 'vip'],
    model_groups: { 'model-a': ['default'], 'model-b': ['vip'] },
  });
  await page.evaluate(() => window.qa.edit());
  await page.getByLabel('model-b 可用分组', { exact: true }).waitFor();
  assert.match(await groupSelect().innerText(), /vip/);
  failSave = true;
  await page.locator('#name').fill('keep-draft');
  await page.locator('.token-editor__submit:visible').click();
  await page.waitForTimeout(350);
  assert.equal(await page.locator('#name').inputValue(), 'keep-draft');
  failSave = false;
  await page.locator('.token-editor__submit:visible').click();
  await page.waitForFunction(
    () => !document.querySelector('.semi-sidesheet-inner'),
  );
  assert.equal(saves.length, 2);
  assert.equal(saves[1].routing_config, saves[0].routing_config);
  await page.evaluate(() => window.qa.test());
  await page.getByLabel('测试分组', { exact: true }).waitFor();
  await select(page.getByLabel('测试分组', { exact: true }), 'vip');
  await page.waitForFunction(() => document.body.innerText.includes('model-b'));
  assert.equal(await page.getByText('model-a', { exact: true }).count(), 0);
  await page.getByRole('button', { name: '开始测试', exact: true }).click();
  await page.waitForTimeout(300);
  assert.deepEqual(tests, [{ model: 'model-b', group: 'vip' }]);
  await page.screenshot({
    path: path.join(artifacts, 'test-group.png'),
    fullPage: true,
  });
  await page.evaluate(() => window.qa.edit());
  await page.setViewportSize({ width: 390, height: 844 });
  await page.evaluate(() => document.body.setAttribute('theme-mode', 'dark'));
  await page.getByLabel('model-b 可用分组', { exact: true }).waitFor();
  await page.waitForTimeout(800);
  const bounds = await page
    .locator('.semi-sidesheet-inner:visible')
    .boundingBox();
  assert.ok(
    bounds.x >= -1 && bounds.x + bounds.width <= 391,
    JSON.stringify(bounds),
  );
  await page.screenshot({
    path: path.join(artifacts, 'mobile-dark.png'),
    fullPage: true,
  });
  assert.equal(
    await page.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth,
    ),
    false,
  );
  await page
    .locator('.token-editor__footer')
    .getByRole('button', { name: '取消', exact: true })
    .click();
  await page.waitForTimeout(400);
  await page.setViewportSize({ width: 1280, height: 960 });
  for (const group of ['default', 'auto']) {
    stored = {
      ...stored,
      group,
      routing_config: '',
      model_limits: 'model-a',
      auto_group_fallback: true,
      cross_group_retry: true,
    };
    await page.evaluate(() => window.qa.edit());
    await page.locator('#routing_enabled').waitFor();
    await page.waitForTimeout(500);
    assert.equal(
      await page.locator('#routing_enabled').getAttribute('aria-checked'),
      'false',
    );
    assert.equal(
      await page
        .getByLabel('可用分组（可多选，按选择顺序路由）', { exact: true })
        .count(),
      0,
    );
    await page.locator('.token-editor__submit:visible').click();
    await page.waitForFunction(
      () => !document.querySelector('.semi-sidesheet-inner'),
    );
    assert.equal(saves.at(-1).routing_config, '');
    assert.equal(saves.at(-1).group, group);
    assert.equal(saves.at(-1).auto_group_fallback, true);
    assert.equal(saves.at(-1).cross_group_retry, true);
  }
  assert.deepEqual(errors, []);
  console.log(
    `PASS: create, model groups, edit, failed save, group test, mobile, legacy fixed/auto compatibility. Screenshots: ${artifacts}`,
  );
} catch (error) {
  if (page) {
    await page.screenshot({
      path: path.join(artifacts, 'failure.png'),
      fullPage: true,
    });
    console.error(await page.locator('body').innerText());
  }
  throw error;
} finally {
  await browser?.close();
  await server?.close();
  for (const file of files) fs.rmSync(file, { force: true });
}
