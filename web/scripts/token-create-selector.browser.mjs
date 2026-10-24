// Real form with synthetic API fixtures only. Never connects to production.
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
process.chdir(root);
const prefix = `.token-selector-qa-${process.pid}`;
const files = [
  path.join(root, prefix + '.html'),
  path.join(root, prefix + '.jsx'),
];
const artifacts = process.env.TOKEN_SELECTOR_ARTIFACTS_DIR
  ? path.resolve(process.env.TOKEN_SELECTOR_ARTIFACTS_DIR)
  : fs.mkdtempSync(path.join(os.tmpdir(), 'token-selector-qa-'));
fs.mkdirSync(artifacts, { recursive: true });
let server, browser, page;
const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
try {
  fs.writeFileSync(
    files[0],
    `<html><meta name="viewport" content="width=device-width, initial-scale=1"><div id="root"></div><script type="module" src="/${prefix}.jsx"></script></html>`,
  );
  fs.writeFileSync(
    files[1],
    `import React,{useState} from 'react';
import {createRoot} from 'react-dom/client';
import {BrowserRouter} from 'react-router-dom';
import i18n from 'i18next';import {initReactI18next} from 'react-i18next';
import {UserContext} from './src/context/User';import {StatusContext} from './src/context/Status';
import EditTokenModal from './src/components/table/tokens/modals/EditTokenModal';import './src/index.css';
i18n.use(initReactI18next).init({lng:'zh',resources:{zh:{translation:{}}},interpolation:{escapeValue:false}});
function App(){const [visible,setVisible]=useState(true),[id,setId]=useState(901),[edit,setEdit]=useState({});
window.qa={switchUser:setId,close:()=>setVisible(false),open:()=>{setEdit({});setVisible(true)},edit:(sellable=false)=>{setEdit({id:99,source_type:sellable?'sellable_token':undefined,status:1});setVisible(true)}};
return <BrowserRouter><UserContext.Provider value={[{user:{id,role:100}},()=>{}]}><StatusContext.Provider value={[{status:{quota_per_unit:500000,default_use_auto_group:true}},()=>{}]}><EditTokenModal visiable={visible} editingToken={edit} handleClose={()=>setVisible(false)} refresh={()=>{}}/></StatusContext.Provider></UserContext.Provider></BrowserRouter>};createRoot(document.getElementById('root')).render(<App/>);`,
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
      (plugin) => !String(plugin?.name).includes('inspector'),
    ),
    server: { host: '127.0.0.1', port: 0, proxy: {} },
  });
  await server.listen();
  browser = await chromium.launch({
    headless: true,
    channel: process.env.PLAYWRIGHT_CHANNEL || undefined,
  });
  const context = await browser.newContext({
    viewport: { width: 1280, height: 900 },
  });
  page = await context.newPage();
  page.setDefaultTimeout(8000);
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  let groups = {
    'OpenAI · 优质': { desc: '日常编程与通用任务', ratio: 0.45 },
    'OpenAI · 独享账号': {
      desc: '适合持续编程。后台配置的说明直接展示，不作稳定性承诺。'.repeat(4),
      ratio: 0.55,
    },
    'Claude · 优质': { desc: '<script>不得执行</script> 后台原文', ratio: 0 },
    auto: { desc: '按配置选择可用分组', ratio: '自动' },
  };
  const currentHour = Math.floor(Date.now() / 3600000) * 3600;
  const healthStats = (requestCount, successCount) => ({
    request_count: requestCount,
    success_count: successCount,
    failure_count: requestCount - successCount,
    success_rate: requestCount ? (successCount / requestCount) * 100 : null,
    avg_ttft_ms: successCount ? 5200 : null,
    ttft_count: successCount,
    cache_hit_rate: successCount ? 60 : null,
    cache_sample_count: successCount,
  });
  const healthGroups = [
    ['OpenAI · 优质', 40, 38],
    ['OpenAI · 独享账号', 7, 1],
    ['Claude · 优质', 0, 0],
  ].map(([group, requestCount, successCount]) => {
    const classifiedStats = (hasCalls) => ({
      ...healthStats(hasCalls ? requestCount : 0, hasCalls ? successCount : 0),
      service_failure_count: hasCalls && requestCount > 0 ? 2 : 0,
      unknown_failure_count: hasCalls && requestCount === 7 ? 4 : 0,
      client_canceled_count: hasCalls && requestCount === 7 ? 3 : 0,
      excluded_user_error_count: hasCalls && requestCount === 7 ? 5 : 0,
      excluded_count: hasCalls && requestCount === 7 ? 5 : 0,
      observed_count: hasCalls
        ? requestCount + (requestCount === 7 ? 5 : 0)
        : 0,
    });
    return {
      group,
      ...classifiedStats(true),
      series: Array.from({ length: 24 }, (_, index) => ({
        ts: currentHour - (23 - index) * 3600,
        ...classifiedStats(index === 0),
      })),
    };
  });
  let healthVariant = 'classified';
  const healthFixture = () => {
    const result = structuredClone(healthGroups);
    const claude = result.find(({ group }) => group === 'Claude · 优质');
    if (healthVariant.startsWith('boundary-')) {
      const count = Number(healthVariant.slice('boundary-'.length));
      const boundary = {
        ...healthStats(count, count),
        service_failure_count: 0,
        unknown_failure_count: 0,
        client_canceled_count: 0,
        excluded_user_error_count: 0,
        excluded_count: 0,
        observed_count: count,
      };
      Object.assign(claude, boundary);
      Object.assign(claude.series[0], boundary);
    } else if (healthVariant === 'excluded-only') {
      const excluded = {
        ...healthStats(0, 0),
        service_failure_count: 0,
        unknown_failure_count: 0,
        client_canceled_count: 0,
        excluded_user_error_count: 4,
        excluded_count: 4,
        observed_count: 4,
      };
      Object.assign(claude, excluded);
      Object.assign(claude.series[0], excluded);
    } else if (healthVariant === 'legacy') {
      const legacy = healthStats(10, 2);
      const fields = [
        'service_failure_count',
        'unknown_failure_count',
        'client_canceled_count',
        'excluded_user_error_count',
        'excluded_count',
        'observed_count',
      ];
      Object.assign(claude, legacy);
      Object.assign(claude.series[0], legacy);
      for (const stats of [claude, ...claude.series])
        for (const field of fields) delete stats[field];
    }
    return result;
  };
  let failGroups = false,
    failModels = false,
    failSave = false,
    releaseModel,
    releaseGroups,
    holdSave = false,
    releaseSave;
  let holdModel = false,
    holdGroups = false;
  let savedToken = {
    id: 99,
    name: 'existing',
    group: 'removed',
    expired_time: -1,
    model_limits: 'legacy-model',
    model_limits_enabled: true,
    channel_limits: '77',
    channel_limits_enabled: true,
    remain_quota: 500000,
    unlimited_quota: false,
    allow_ips: '127.0.0.1',
    max_concurrency: 2,
    window_seconds: 60,
    window_request_limit: 10,
  };
  const writes = [],
    modelRequests = [];
  await page.route('**/api/**', async (route) => {
    const req = route.request(),
      url = new URL(req.url());
    let data = [];
    if (req.method() === 'PUT' || req.method() === 'POST') {
      writes.push(req.postDataJSON());
      if (holdSave) {
        holdSave = false;
        await new Promise((r) => {
          releaseSave = r;
        });
      }
      await route.fulfill({
        json: {
          success: !failSave,
          message: failSave ? 'synthetic save failure' : '',
          data: {},
        },
      });
      return;
    }
    if (url.pathname === '/api/user/self/groups') {
      data = structuredClone(groups);
      if (holdGroups) {
        holdGroups = false;
        await new Promise((r) => {
          releaseGroups = r;
        });
      }
      if (failGroups) {
        await route.fulfill({
          json: { success: false, message: 'synthetic groups failure' },
        });
        return;
      }
    } else if (url.pathname === '/api/token/models') {
      const group = url.searchParams.get('group');
      modelRequests.push(group);
      data = [group === 'Claude · 优质' ? 'claude-test' : 'openai-test'];
      if (holdModel && group === 'OpenAI · 优质') {
        holdModel = false;
        data = ['stale-model'];
        await new Promise((r) => {
          releaseModel = r;
        });
      }
      if (failModels) {
        await route.fulfill({
          json: { success: false, message: 'synthetic models failure' },
        });
        return;
      }
    } else if (url.pathname === '/api/token/channels')
      data = [
        {
          id: 77,
          name: 'saved channel',
          available: false,
          matched_groups: [],
          matched_models: [],
        },
      ];
    else if (url.pathname === '/api/token/99')
      data = structuredClone(savedToken);
    else if (url.pathname === '/api/health/groups')
      data = { enabled: true, groups: healthFixture() };
    await route.fulfill({ json: { success: true, data } });
  });
  await page.addInitScript(() => {
    window.qaClipboard = { fail: false, values: [] };
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: {
        writeText: async (text) => {
          if (window.qaClipboard.fail)
            throw new Error('synthetic denied clipboard');
          window.qaClipboard.values.push(text);
        },
      },
    });
    localStorage.setItem('quota_per_unit', '500000');
    localStorage.setItem('user', JSON.stringify({ id: 901, role: 100 }));
    localStorage.setItem(
      'token-group-favorites:v1:901',
      JSON.stringify(['OpenAI · 独享账号', 'removed']),
    );
  });
  const ready = async () => {
    await page.getByRole('radio').first().waitFor();
    await page.waitForFunction(
      () =>
        document
          .querySelector('.token-group-picker__list')
          ?.getAttribute('aria-busy') === 'false',
    );
  };
  const name = () => page.getByPlaceholder('请输入名称', { exact: true });
  const create = () => page.getByRole('button', { name: /创建令牌/ });
  const choose = async (group) => {
    await page
      .getByRole('radio', {
        name: new RegExp(group.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')),
      })
      .check();
  };
  const enabled = async (button) => {
    for (let i = 0; i < 80; i++) {
      if (await button.isEnabled()) return;
      await delay(50);
    }
    throw Error('submit remained disabled');
  };
  await page.goto(
    `http://127.0.0.1:${server.httpServer.address().port}/${prefix}.html`,
  );
  await ready();
  assert.equal(await create().isEnabled(), false);
  assert.equal(await page.getByRole('radio', { checked: true }).count(), 0);
  assert.equal(await page.getByText('0×', { exact: true }).count(), 1);
  assert.equal(await page.getByText('动态', { exact: true }).count(), 0);
  const row = (group) =>
    page
      .locator('.token-group-picker__row')
      .filter({ has: page.getByRole('radio', { name: group, exact: true }) });
  const order = () =>
    page.getByRole('radio').evaluateAll((nodes) => nodes.map((n) => n.value));
  assert.deepEqual(await order(), [
    'OpenAI · 独享账号',
    'OpenAI · 优质',
    'Claude · 优质',
  ]);
  assert.equal(await page.locator('#auto_group_fallback').getAttribute('aria-checked'), 'true');
  await choose('OpenAI · 独享账号');
  await enabled(create());
  for (const value of ['', '   ']) {
    await name().fill(value);
    await create().click();
    await page
      .locator('.token-editor__name .semi-input-wrapper-error')
      .waitFor();
    assert(await page.getByText('请输入令牌名称', { exact: true }).isVisible());
    assert(await name().evaluate((el) => el === document.activeElement));
    assert.equal(writes.length, 0);
    const colors = await page
      .locator('.token-editor__name .semi-input-wrapper')
      .evaluate((el) => ({
        actual: getComputedStyle(el).borderTopColor,
        error: getComputedStyle(document.body)
          .getPropertyValue('--semi-color-danger')
          .trim(),
      }));
    assert(colors.actual && colors.error);
  }
  await page.screenshot({
    path: path.join(artifacts, 'name-required.png'),
    fullPage: true,
  });
  await name().fill('simple token');
  await page
    .locator('.token-editor__name .semi-input-wrapper-error')
    .waitFor({ state: 'hidden' });
  await row('OpenAI · 优质')
    .locator('.token-group-picker__description')
    .click();
  assert.equal(
    await page.getByRole('radio', { checked: true }).inputValue(),
    'OpenAI · 优质',
  );
  await row('OpenAI · 独享账号').locator('.token-group-picker__ratio').click();
  assert.equal(
    await page.getByRole('radio', { checked: true }).inputValue(),
    'OpenAI · 独享账号',
  );
  await row('OpenAI · 优质').click({ position: { x: 4, y: 4 } });
  assert.equal(
    await page.getByRole('radio', { checked: true }).inputValue(),
    'OpenAI · 优质',
  );
  await page.getByRole('button', { name: '展开说明', exact: true }).click();
  assert.equal(
    await page.getByRole('radio', { checked: true }).inputValue(),
    'OpenAI · 优质',
  );
  await page.getByRole('button', { name: '收起说明', exact: true }).click();
  await row('OpenAI · 独享账号').locator('strong').click();
  await enabled(create());
  // Selecting description text must not accidentally change the token group.
  await row('OpenAI · 优质')
    .locator('.token-group-picker__description')
    .evaluate((element) => {
      const range = document.createRange();
      range.selectNodeContents(element);
      window.getSelection().removeAllRanges();
      window.getSelection().addRange(range);
      element.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });
  assert.equal(
    await page.getByRole('radio', { checked: true }).inputValue(),
    'OpenAI · 独享账号',
  );
  await page.evaluate(() => window.getSelection().removeAllRanges());
  await page.getByText('查看详情', { exact: true }).click();
  const copyButton = page.getByRole('button', {
    name: '复制模型名称 openai-test',
    exact: true,
  });
  await copyButton.waitFor();
  const beforeCopyRequests = modelRequests.length;
  await copyButton.click();
  await page
    .getByText('已复制模型名称：openai-test', { exact: true })
    .waitFor();
  assert.deepEqual(await page.evaluate(() => window.qaClipboard.values), [
    'openai-test',
  ]);
  assert.equal(writes.length, 0);
  assert.equal(modelRequests.length, beforeCopyRequests);
  await page.evaluate(() => (window.qaClipboard.fail = true));
  await copyButton.click();
  await page
    .getByText('复制失败，请手动选择模型名称复制', { exact: true })
    .waitFor();
  assert.equal(
    await page
      .getByText('已复制模型名称：openai-test', { exact: true })
      .count(),
    0,
  );
  await page.evaluate(() => (window.qaClipboard.fail = false));
  await copyButton.focus();
  await page.keyboard.press('Enter');
  await page
    .getByText('已复制模型名称：openai-test', { exact: true })
    .waitFor();
  assert.deepEqual(await page.evaluate(() => window.qaClipboard.values), [
    'openai-test',
    'openai-test',
  ]);
  const health = page.locator('.token-group-health');
  const healthSummary = health.locator('.token-group-health-line');
  const lowRate = healthSummary.locator('strong.group-health-text-limited');
  assert.equal(await lowRate.textContent(), '14.29%');
  assert.equal(
    await healthSummary.getByText('样本不足', { exact: true }).count(),
    1,
  );
  assert.equal(
    await healthSummary.locator('.group-health-count-notice').textContent(),
    '未归因 4 次 · 已排除 5 次',
  );
  assert.match(
    await healthSummary
      .locator('.token-group-health-metrics')
      .getAttribute('title'),
    /已观测 12 次 · 计入统计 7 次.*成功 1 · 服务失败 2 · 未归因失败 4.*客户端断开 3 次（包含在未归因失败中）.*已排除用户错误 5 次/,
  );
  assert.notEqual(
    await lowRate.evaluate((element) => getComputedStyle(element).color),
    await healthSummary
      .locator('strong:not([class])')
      .first()
      .evaluate((element) => getComputedStyle(element).color),
    'token health color must not be overridden by default metric styling',
  );
  await health.getByRole('button', { name: /趋势/ }).click();
  for (const text of ['成功率 ≥ 95%', '80% ≤ 成功率 < 95%', '成功率 < 80%'])
    assert(
      await health
        .locator('.group-health-legend')
        .getByText(text, { exact: true })
        .isVisible(),
    );
  assert(
    await health
      .getByText(
        '计入统计少于 30 次时显示灰色；达到 30 次后按实际成功率着色。',
        {
          exact: true,
        },
      )
      .isVisible(),
  );
  const lowHour = health.locator('button.group-health-bar-limited');
  assert.equal(await lowHour.count(), 1);
  assert.equal(
    await health.locator('button.group-health-bar-empty').count(),
    23,
  );
  assert.match(
    await lowHour.getAttribute('aria-label'),
    /14\.29%.*样本不足.*计入统计 7 次.*服务失败 2.*未归因失败 4.*客户端断开 3 次.*已排除用户错误 5 次/,
  );
  await lowHour.click();
  assert.equal(
    await health
      .locator('.group-health-bucket strong.group-health-text-limited')
      .textContent(),
    '14.29%',
  );
  assert(
    await health
      .locator('.group-health-bucket-header')
      .getByText('样本不足', { exact: true })
      .isVisible(),
  );
  assert.equal(
    await page.getByRole('radio', { checked: true }).inputValue(),
    'OpenAI · 独享账号',
  );
  await health.screenshot({
    path: path.join(artifacts, 'token-health-limited.png'),
    animations: 'disabled',
    style: '.semi-toast-wrapper { visibility:hidden !important; }',
  });
  const outcomeDetails = health.locator('details.group-health-outcomes');
  for (const width of [1280, 390, 320]) {
    await page.setViewportSize({ width, height: 900 });
    await page.evaluate((dark) => {
      document.documentElement.classList.toggle('dark', dark);
      document.body.setAttribute('theme-mode', dark ? 'dark' : 'light');
    }, width < 1280);
    await outcomeDetails.locator('summary').click();
    assert.match(
      await outcomeDetails.textContent(),
      /成功 1 · 服务失败 2 · 未归因失败 4/,
    );
    assert.match(
      await outcomeDetails.textContent(),
      /客户端断开 3 次（包含在未归因失败中）/,
    );
    assert.match(await outcomeDetails.textContent(), /已排除用户错误 5 次/);
    assert(await outcomeDetails.locator('.group-health-caption').isVisible());
    assert(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
      `health details must not overflow at ${width}px`,
    );
    assert(
      await health
        .locator('.group-health-number')
        .evaluateAll((numbers) =>
          numbers.every((number) => number.scrollWidth <= number.clientWidth),
        ),
      `metric values must fit their columns at ${width}px`,
    );
    if (width < 1280) {
      await health.locator('.group-health-metrics').scrollIntoViewIfNeeded();
      await page.screenshot({
        path: path.join(artifacts, `token-health-metrics-${width}.png`),
        animations: 'disabled',
        style: '.semi-toast-wrapper { visibility:hidden !important; }',
      });
      await outcomeDetails.scrollIntoViewIfNeeded();
    }
    await (width < 1280 ? page : health).screenshot({
      path: path.join(artifacts, `token-health-classified-${width}.png`),
      animations: 'disabled',
      style: '.semi-toast-wrapper { visibility:hidden !important; }',
    });
    await outcomeDetails.locator('summary').click();
  }
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.evaluate(() => {
    document.documentElement.classList.remove('dark');
    document.body.setAttribute('theme-mode', 'light');
  });
  console.log(
    'PASS token low-sample rate stays numeric and gray, shared legend and hourly colors; selection unchanged',
  );
  await page.getByText('查看详情', { exact: true }).click();
  console.log(
    'PASS whole-row selection, independent expansion, text selection and truthful clipboard feedback',
  );
  const requestsBefore = modelRequests.length;
  await page
    .getByRole('button', { name: '取消收藏 OpenAI · 独享账号', exact: true })
    .click();
  assert.equal(
    await page.getByRole('radio', { checked: true }).inputValue(),
    'OpenAI · 独享账号',
  );
  assert.equal(modelRequests.length, requestsBefore);
  assert.equal(writes.length, 0);
  await page
    .getByRole('button', {
      name: '收藏当前分组 OpenAI · 独享账号',
      exact: true,
    })
    .click();
  await page.getByRole('button', { name: /^Claude · 1$/ }).click();
  assert(
    await page
      .getByText('已选分组：OpenAI · 独享账号', { exact: true })
      .isVisible(),
  );
  await page.getByRole('button', { name: '全部', exact: true }).click();
  await page
    .getByRole('button', { name: '收藏当前分组 OpenAI · 优质', exact: true })
    .click();
  await page
    .getByRole('button', { name: '☆ 我的收藏分组', exact: true })
    .click();
  assert.deepEqual(await order(), ['OpenAI · 独享账号', 'OpenAI · 优质']);
  await page.getByRole('button', { name: '全部', exact: true }).click();
  await page
    .getByRole('textbox', { name: '搜索分组', exact: true })
    .fill('日常');
  assert.equal(await page.getByRole('radio').count(), 1);
  assert(
    await page
      .getByText('已选分组：OpenAI · 独享账号', { exact: true })
      .isVisible(),
  );
  await page.getByRole('textbox', { name: '搜索分组', exact: true }).fill('');
  await page.getByRole('button', { name: '展开说明', exact: true }).click();
  await page.getByRole('button', { name: '收起说明', exact: true }).click();
  await page.getByRole('button', { name: /^OpenAI · 2$/ }).click();
  await delay(400);
  await page.screenshot({
    path: path.join(artifacts, 'desktop.png'),
    fullPage: true,
    animations: 'disabled',
  });
  await page.setViewportSize({ width: 1280, height: 720 });
  const compactBox = await create().boundingBox();
  assert(
    compactBox && compactBox.y >= 0 && compactBox.y + compactBox.height <= 720,
  );
  await page.setViewportSize({ width: 1280, height: 900 });
  failSave = true;
  await create().click();
  await delay(200);
  assert.equal(await name().inputValue(), 'simple token');
  assert(
    await page
      .getByText('已选分组：OpenAI · 独享账号', { exact: true })
      .isVisible(),
  );
  failSave = false;
  await create().click();
  await delay(200);
  assert.equal(writes.at(-1).group, 'OpenAI · 独享账号');
  assert.equal(writes.at(-1).auto_group_fallback, true);
  assert.equal(writes.at(-1).expired_time, -1);
  assert.equal(writes.at(-1).unlimited_quota, true);
  console.log(
    'PASS create, descriptions, zero/auto ratios, filter/star independence, failed submit retains data',
  );
  await page.evaluate(() => window.qa.edit());
  await page.getByText('已选分组：removed', { exact: true }).waitFor();
  const submit = page.getByRole('button', { name: /提交/ });
  await enabled(submit);
  await submit.click();
  await delay(200);
  for (const key of [
    'group',
    'model_limits',
    'channel_limits',
    'remain_quota',
    'allow_ips',
    'max_concurrency',
    'window_seconds',
    'window_request_limit',
  ])
    assert.equal(writes.at(-1)[key], savedToken[key], key);
  assert.equal(writes.at(-1).auto_group_fallback, false);
  savedToken.group = '';
  await page.evaluate(() => window.qa.edit());
  await enabled(submit);
  await submit.click();
  await delay(200);
  assert.equal(writes.at(-1).group, '');
  savedToken.group = 'auto';
  savedToken.cross_group_retry = true;
  await page.evaluate(() => window.qa.edit());
  await enabled(submit);
  assert.equal(await page.locator('#cross_group_retry').getAttribute('aria-checked'), 'true');
  await submit.click();
  await delay(200);
  assert.equal(writes.at(-1).group, 'auto');
  assert.equal(writes.at(-1).cross_group_retry, true);
  savedToken.group = 'removed';
  await page.evaluate(() => window.qa.edit(true));
  await enabled(submit);
  await submit.click();
  await delay(200);
  assert.deepEqual(Object.keys(writes.at(-1)).sort(), [
    'group',
    'id',
    'name',
    'status',
  ]);
  console.log(
    'PASS unchanged edit preserves unavailable/empty group and limits; sellable restricted payload',
  );
  await page.evaluate(() => window.qa.open());
  await ready();
  await name().fill('race test');
  holdModel = true;
  await choose('OpenAI · 优质');
  for (let i = 0; !releaseModel && i < 80; i++) await delay(25);
  assert(releaseModel);
  await row('Claude · 优质').locator('.token-group-picker__ratio').click();
  assert.equal(
    await page.getByRole('radio', { checked: true }).inputValue(),
    'Claude · 优质',
  );
  await enabled(create());
  releaseModel();
  await delay(150);
  await page.getByText('查看详情', { exact: true }).click();
  await page
    .getByRole('button', { name: '复制模型名称 claude-test', exact: true })
    .waitFor({ state: 'visible' });
  assert.equal(await page.getByText(/stale-model/).count(), 0);
  const emptyRate = healthSummary.locator('strong.group-health-text-empty');
  assert.equal(await emptyRate.textContent(), '—');
  assert.equal(
    await healthSummary.getByText('样本不足', { exact: true }).count(),
    0,
  );
  assert.equal(
    await emptyRate.evaluate((element) => getComputedStyle(element).color),
    await healthSummary
      .locator('.token-group-health-metrics')
      .evaluate((element) => getComputedStyle(element).color),
    'empty health rate should use the muted gray color',
  );
  await health.getByRole('button', { name: /趋势/ }).click();
  assert.equal(
    await health.locator('button.group-health-bar-empty').count(),
    24,
  );
  assert.equal(
    await health.locator('button.group-health-bar-healthy').count(),
    0,
  );
  await health.locator('button.group-health-bar-empty').first().click();
  assert(
    await health
      .locator('.group-health-bucket .group-health-inline')
      .getByText('暂无调用', { exact: true })
      .isVisible(),
  );
  assert.equal(await health.getByText('样本不足', { exact: true }).count(), 0);
  assert.equal(
    await page.getByRole('radio', { checked: true }).inputValue(),
    'Claude · 优质',
  );
  await health.screenshot({
    path: path.join(artifacts, 'token-health-empty.png'),
    animations: 'disabled',
    style: '.semi-toast-wrapper { visibility:hidden !important; }',
  });
  console.log(
    'PASS token zero-sample gray, no limited-sample notice, empty hourly details; selection unchanged',
  );
  await create().click();
  await delay(200);
  assert.equal(writes.at(-1).group, 'Claude · 优质');
  console.log('PASS model response race cannot overwrite latest group');
  await page.evaluate(() => window.qa.open());
  await ready();
  await name().fill('package');
  await choose('OpenAI · 优质');
  await enabled(create());
  await page.locator('summary').filter({ hasText: '额度与有效期' }).click();
  await page.getByRole('button', { name: '套餐令牌', exact: true }).click();
  await create().click();
  await delay(200);
  assert.equal(writes.at(-1).package_enabled, true);
  assert.equal(writes.at(-1).package_limit_quota, 5000000);
  await page.evaluate(() => window.qa.open());
  await ready();
  await name().fill('limited');
  await choose('Claude · 优质');
  await enabled(create());
  await page.locator('summary').filter({ hasText: '额度与有效期' }).click();
  await page.getByRole('switch', { name: '无限额度' }).click();
  await page
    .getByRole('spinbutton', { name: '总额度金额 (USD)' })
    .fill('12.34');
  await page.getByRole('button', { name: '一天', exact: true }).click();
  await page.locator('summary').filter({ hasText: '额度与有效期' }).click();
  const beforeWrites = writes.length;
  holdSave = true;
  await create().click();
  for (let i = 0; !releaseSave && i < 80; i++) await delay(25);
  assert(releaseSave);
  assert.equal(await create().isEnabled(), false);
  assert.equal(writes.length, beforeWrites + 1);
  releaseSave();
  await delay(200);
  assert.equal(writes.at(-1).remain_quota, 6170000);
  assert.equal(writes.at(-1).unlimited_quota, false);
  assert(writes.at(-1).expired_time > Date.now() / 1000);
  console.log(
    'PASS retained folded quota/expiry, package amount, duplicate-submit guard',
  );
  failModels = true;
  await page.evaluate(() => window.qa.open());
  await ready();
  await name().fill('failed models');
  await choose('Claude · 优质');
  await page.getByText('模型加载失败', { exact: false }).waitFor();
  assert.equal(await create().isEnabled(), false);
  failModels = false;
  await page.getByRole('button', { name: '重试', exact: true }).click();
  await enabled(create());
  await page.evaluate(() => window.qa.close());
  await delay(100);
  failGroups = true;
  await page.evaluate(() => window.qa.open());
  await page.getByText('分组加载失败', { exact: false }).waitFor();
  assert.equal(await page.getByRole('radio').count(), 0);
  assert.equal(await create().isEnabled(), false);
  failGroups = false;
  await page.getByRole('button', { name: '重试', exact: true }).click();
  await ready();
  await page.evaluate(() => window.qa.close());
  await delay(100);
  holdGroups = true;
  await page.evaluate(() => window.qa.open());
  for (let i = 0; !releaseGroups && i < 80; i++) await delay(25);
  assert(releaseGroups);
  groups = { 'Claude · 优质': { desc: '新账号', ratio: 0 } };
  await page.evaluate(() => window.qa.switchUser(902));
  await ready();
  releaseGroups();
  await delay(150);
  assert.equal(await page.getByRole('radio').count(), 1);
  assert.equal(await create().isEnabled(), false);
  console.log(
    'PASS permission failure and account-switch late response isolation',
  );
  await page.setViewportSize({ width: 390, height: 844 });
  await name().fill('mobile');
  await choose('Claude · 优质');
  await enabled(create());
  await delay(350);
  await page.screenshot({
    path: path.join(artifacts, 'mobile.png'),
    fullPage: true,
    animations: 'disabled',
    style: '.semi-toast-wrapper { visibility:hidden !important; }',
  });
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  );
  const box = await create().boundingBox();
  assert(
    box && box.x >= 0 && box.x + box.width <= 390 && box.y + box.height <= 844,
  );
  await create().focus();
  await page.keyboard.press('Enter');
  await delay(200);
  assert.equal(writes.at(-1).name, 'mobile');
  for (const variant of ['excluded-only', 'legacy']) {
    healthVariant = variant;
    await page.setViewportSize({ width: 320, height: 900 });
    await page.evaluate(() => {
      document.documentElement.classList.add('dark');
      document.body.setAttribute('theme-mode', 'dark');
      window.qa.open();
    });
    await ready();
    await name().fill(variant);
    await choose('Claude · 优质');
    await enabled(create());
    await page.getByText('查看详情', { exact: true }).click();
    await healthSummary.waitFor();
    const description = await healthSummary
      .locator('.token-group-health-metrics')
      .getAttribute('title');
    if (variant === 'excluded-only') {
      assert.match(
        description,
        /已观测 4 次 · 计入统计 0 次.*成功 0 · 服务失败 0 · 未归因失败 0.*已排除用户错误 4 次/,
      );
      assert(
        await healthSummary
          .getByText('无有效统计样本', { exact: true })
          .isVisible(),
      );
      assert(
        await healthSummary
          .getByText('已排除 4 次', { exact: true })
          .isVisible(),
      );
      assert.equal(
        await healthSummary.getByText('暂无调用', { exact: true }).count(),
        0,
      );
      assert.equal(
        await healthSummary.getByText('样本不足', { exact: true }).count(),
        0,
      );
      assert.equal(
        await healthSummary
          .locator('strong.group-health-text-empty')
          .textContent(),
        '—',
      );
    } else {
      assert.match(
        description,
        /已观测 10 次 · 计入统计 10 次.*成功 2 · 服务失败 0 · 未归因失败 8/,
      );
      assert.doesNotMatch(description, /客户端断开 \d+ 次|已排除用户错误/);
      assert.equal(
        await healthSummary
          .locator('strong.group-health-text-limited')
          .textContent(),
        '20.00%',
      );
      assert(
        await healthSummary
          .getByText('未归因 8 次', { exact: true })
          .isVisible(),
      );
    }
    await health.getByRole('button', { name: /趋势/ }).click();
    assert(
      await health
        .getByText('最近 24 个小时段成功率', { exact: true })
        .isVisible(),
    );
    await outcomeDetails.locator('summary').click();
    await health.locator('button.group-health-bar').first().click();
    const bucket = health.locator('.group-health-bucket');
    if (variant === 'excluded-only') {
      assert.equal(
        await health.locator('button.group-health-bar-empty').count(),
        24,
      );
      assert.match(await bucket.textContent(), /无有效统计样本.*已排除 4 次/);
      assert.match(await bucket.textContent(), /已观测 4 次 · 计入统计 0 次/);
      assert.equal(
        await health
          .getByText('该时段暂无调用记录，产生新请求后显示统计。', {
            exact: true,
          })
          .count(),
        0,
      );
    } else {
      assert.match(await bucket.textContent(), /20\.00%.*未归因 8 次/);
      assert.match(
        await bucket.textContent(),
        /成功 2 · 服务失败 0 · 未归因失败 8/,
      );
    }
    assert.equal(
      await page.getByRole('radio', { checked: true }).inputValue(),
      'Claude · 优质',
    );
    assert(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
      `${variant} health details must not overflow at 320px`,
    );
    await outcomeDetails.scrollIntoViewIfNeeded();
    await page.screenshot({
      path: path.join(artifacts, `token-health-${variant}-320.png`),
      animations: 'disabled',
      style: '.semi-toast-wrapper { visibility:hidden !important; }',
    });
    await health.locator('button.group-health-bar').last().click();
    assert.match(await bucket.textContent(), /当前小时尚未结束/);
    await page.evaluate(() => window.qa.close());
    await delay(100);
  }
  console.log(
    'PASS excluded-only and legacy classifications, hourly agreement, current-hour wording, 320px dark details and unchanged selection',
  );
  for (const count of [1, 29, 30]) {
    healthVariant = `boundary-${count}`;
    const tone = count < 30 ? 'limited' : 'healthy';
    const label = count < 30 ? '样本不足' : '运行稳定';
    await page.evaluate(() => window.qa.open());
    await ready();
    await name().fill(healthVariant);
    await choose('Claude · 优质');
    await enabled(create());
    await page.getByText('查看详情', { exact: true }).click();
    const summaryRate = healthSummary.locator(
      `strong.group-health-text-${tone}`,
    );
    assert.equal(await summaryRate.textContent(), '100.00%');
    assert.equal(
      await healthSummary.getByText('样本不足', { exact: true }).count(),
      count < 30 ? 1 : 0,
    );
    await health.getByRole('button', { name: /趋势/ }).click();
    const panel = health.locator('.group-health-panel');
    assert.equal(
      await panel
        .locator(`.group-health-number.group-health-text-${tone}`)
        .textContent(),
      '100.00%',
    );
    const hour = panel.locator('button.group-health-bar').first();
    assert.equal(
      await hour.getAttribute('class'),
      `group-health-bar group-health-bar-${tone}`,
    );
    if (count < 30) {
      assert.equal(
        await hour.evaluate((el) => getComputedStyle(el).backgroundColor),
        await panel
          .locator('button.group-health-bar-empty')
          .first()
          .evaluate((el) => getComputedStyle(el).backgroundColor),
      );
      assert.equal(
        await summaryRate.evaluate((el) => getComputedStyle(el).color),
        await panel
          .locator(`.group-health-number.group-health-text-${tone}`)
          .evaluate((el) => getComputedStyle(el).color),
      );
    }
    await hour.click();
    assert.equal(
      await panel.locator('.group-health-bucket-header span').textContent(),
      label,
    );
    assert.equal(
      await panel
        .locator(`.group-health-bucket strong.group-health-text-${tone}`)
        .textContent(),
      '100.00%',
    );
    assert.equal(
      await page.getByRole('radio', { checked: true }).inputValue(),
      'Claude · 优质',
    );
    await page.screenshot({
      path: path.join(artifacts, `token-health-sample-${count}-320.png`),
      animations: 'disabled',
      style: '.semi-toast-wrapper { visibility:hidden !important; }',
    });
    await page.evaluate(() => window.qa.close());
    await delay(100);
  }
  console.log(
    'PASS token 1/29/30 count boundaries agree across summary, trend and hourly details; actual rate stays 100%',
  );
  assert.deepEqual(errors, []);
  console.log(
    'PASS mobile width, visible footer, keyboard submit; ARTIFACTS=' +
      artifacts,
  );
} catch (e) {
  if (page)
    await page
      .screenshot({ path: path.join(artifacts, 'failure.png'), fullPage: true })
      .catch(() => {});
  console.error('ARTIFACTS=' + artifacts);
  throw e;
} finally {
  if (browser) await browser.close();
  if (server) await server.close();
  for (const file of files) if (fs.existsSync(file)) fs.unlinkSync(file);
  // Vite uses esbuild's shared service in this process.
  const esbuild = await import('esbuild');
  esbuild.stop();
  console.log(
    'Owned browser, Vite and esbuild stopped; temporary fixture entries removed.',
  );
}
