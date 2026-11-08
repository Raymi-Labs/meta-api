// Real settings component with synthetic APIs; never connects to a live backend.
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
const dir = await fs.mkdtemp(path.join(root, '.group-settings-qa-'));
const output = await fs.mkdtemp(path.join(os.tmpdir(), 'group-settings-qa-'));
let server, browser, page;
const errors = [],
  writes = [];
let failNext = true;
const options = {
  GroupRatio: '{"default":1,"premium":2}',
  UserGroups: '{"default":"","vip":"VIP"}',
  UserUsableGroups: '{"default":"","premium":""}',
  GroupGroupRatio: '{"vip":{"default":0.5,"premium":0.5}}',
  'group_ratio_setting.group_special_usable_group':
    '{"vip":{"+:default":"test","+:premium":"test"}}',
  AutoGroups: '["default"]',
  DefaultUseAutoGroup: false,
};
try {
  await fs.writeFile(
    path.join(dir, 'index.html'),
    '<div id="root"></div><script type="module" src="./entry.jsx"></script>',
  );
  await fs.writeFile(
    path.join(dir, 'entry.jsx'),
    `import React,{useState} from 'react';import{createRoot}from'react-dom/client';import i18n from'i18next';import{initReactI18next}from'react-i18next';import GroupRatioSettings from'/src/pages/Setting/Ratio/GroupRatioSettings';import{API}from'/src/helpers';import'/src/index.css';i18n.use(initReactI18next).init({lng:'zh-CN',resources:{'zh-CN':{translation:{}}},interpolation:{escapeValue:false}});function Fixture(){const [options,setOptions]=useState(${JSON.stringify(options)});return <main style={{padding:16}}><GroupRatioSettings options={options} refresh={async()=>{const r=await API.get('/api/fixture-options');setOptions(r.data.data)}}/></main>}createRoot(document.getElementById('root')).render(<Fixture/>);`,
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
        name: 'block-live-api',
        configureServer(v) {
          v.middlewares.use((req, res, next) => {
            if (/^\/(api|v1)\//.test(req.url || '')) {
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
  browser = await chromium.launch({
    headless: true,
    channel: process.env.PLAYWRIGHT_CHANNEL || 'chrome',
  });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1100 },
  });
  page = await context.newPage();
  page.setDefaultTimeout(12000);
  page.on('pageerror', (e) => errors.push(e.message));
  await context.route('**/api/**', async (route) => {
    const req = route.request(),
      pathname = new URL(req.url()).pathname;
    let body = { success: true, data: [] };
    if (req.method() === 'PUT') {
      const update = req.postDataJSON();
      writes.push(update);
      if (failNext) {
        failNext = false;
        body = { success: false, message: 'fixture failure' };
      } else if (update.key === 'UserChannelGroups')
        Object.assign(options, JSON.parse(update.value));
      else options[update.key] = update.value;
    } else if (pathname === '/api/fixture-options') body.data = { ...options };
    else if (pathname === '/api/group/user_groups')
      body.data = Object.entries(JSON.parse(options.UserGroups)).map(
        ([name, desc]) => ({
          name,
          desc,
          registered: true,
          user_count: 0,
          plan_count: 0,
          rule_count: 0,
        }),
      );
    else assert.equal(pathname, '/api/group/stale_token_groups');
    await route.fulfill({ json: body });
  });
  await page.goto(
    `http://127.0.0.1:${server.httpServer.address().port}/${path.basename(dir)}/index.html`,
  );
  const save = page.getByRole('button', {
    name: '保存分组相关设置',
    exact: true,
  });
  await save.waitFor();
  const user = await page.locator('input[value="vip"]').elementHandle();
  const channel = await page.locator('input[value="premium"]').elementHandle();
  assert.ok(user && channel);
  await user.fill('');
  await save.click();
  assert.equal(writes.length, 0);
  await user.fill('default');
  await save.click();
  assert.equal(writes.length, 0);
  await user.fill('premium');
  await channel.fill('vip');
  await save.click();
  await page.waitForFunction(() =>
    document.body.textContent.includes('fixture failure'),
  );
  assert.equal(writes.length, 1);
  assert.equal(writes[0].key, 'UserChannelGroups');
  assert.equal(await user.inputValue(), 'premium');
  assert.equal(await channel.inputValue(), 'vip');
  await save.click();
  await page.waitForResponse((r) => r.url().endsWith('/api/fixture-options'));
  assert.deepEqual(JSON.parse(options.UserGroups), {
    default: '',
    premium: 'VIP',
  });
  assert.deepEqual(JSON.parse(options.GroupRatio), { default: 1, vip: 2 });
  const section = (name) =>
    page
      .locator('.semi-form-section')
      .filter({ has: page.getByText(name, { exact: true }) });
  const choose = async (select, names) => {
    await select.click();
    for (const name of names)
      await page
        .locator('.semi-select-option:visible')
        .filter({ hasText: new RegExp('^\\s*' + name + '\\s*$') })
        .click();
    await page.mouse.click(2, 2);
    await page
      .locator('.semi-select-option:visible')
      .first()
      .waitFor({ state: 'hidden' });
  };
  for (const title of ['分组特殊倍率', '分组特殊可用分组']) {
    const area = section(title);
    await area.getByText('vip', { exact: true }).click();
    const loaded = area.locator('.semi-select-multiple').first();
    assert.match(await loaded.innerText(), /default/);
    assert.match(await loaded.innerText(), /premium/);
    const addSelect = area.locator('.semi-select-multiple').last();
    await choose(addSelect, ['default', 'premium']);
    await area.getByText('添加分组规则', { exact: true }).click();
    for (const name of ['default', 'premium']) {
      const label = area
        .locator('.cursor-pointer')
        .filter({ has: page.getByText(name, { exact: true }) });
      await label.click();
      const group = label.locator('..');
      const target = group.locator('.semi-select-multiple').first();
      await choose(target, ['default', 'vip']);
    }
  }
  await save.click();
  await page.waitForResponse((r) => r.url().endsWith('/api/fixture-options'));
  const ratios = JSON.parse(options.GroupGroupRatio),
    usable = JSON.parse(
      options['group_ratio_setting.group_special_usable_group'],
    );
  for (const userGroup of ['default', 'premium']) {
    assert.deepEqual(ratios[userGroup], { default: 1, vip: 1 });
    assert.deepEqual(usable[userGroup], { default: '', vip: '' });
  }
  console.log(
    'desktop flow passed',
    JSON.stringify({ ratios, usable, errors }),
  );
  await page.screenshot({
    path: path.join(output, 'desktop.png'),
    fullPage: false,
    animations: 'disabled',
    timeout: 30000,
  });
  await page.close();
  page = await context.newPage();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(
    `http://127.0.0.1:${server.httpServer.address().port}/${path.basename(dir)}/index.html`,
  );
  await page
    .getByRole('button', { name: '保存分组相关设置', exact: true })
    .waitFor();
  await page.screenshot({
    path: path.join(output, 'mobile.png'),
    fullPage: false,
    animations: 'disabled',
    timeout: 30000,
  });
  const mobileRules = section('分组特殊倍率');
  await mobileRules.scrollIntoViewIfNeeded();
  await mobileRules.getByText('vip', { exact: true }).click();
  await page.screenshot({
    path: path.join(output, 'mobile-rules.png'),
    fullPage: false,
    animations: 'disabled',
  });
  assert.deepEqual(errors, []);
  console.log(
    JSON.stringify({ result: 'passed', writes: writes.length, output }),
  );
} catch (error) {
  console.error(error);
  if (page) {
    console.error(
      await page
        .locator('body')
        .innerText({ timeout: 1000 })
        .catch(() => 'no document'),
    );
    await page
      .screenshot({
        path: path.join(output, 'failure.png'),
        fullPage: true,
        timeout: 1000,
      })
      .catch(() => {});
  }
  throw error;
} finally {
  await browser?.close();
  await server?.close();
  stopEsbuild();
  await fs.rm(dir, { recursive: true, force: true });
}
