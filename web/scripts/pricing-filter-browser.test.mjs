import assert from 'node:assert/strict';
import puppeteer from 'puppeteer-core';

// Run against a local Vite server; all API traffic uses synthetic fixtures.
const baseURL = process.env.PRICING_TEST_URL || 'http://127.0.0.1:5179';
const browser = await puppeteer.launch({
  executablePath:
    process.env.CHROME_PATH ||
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  headless: true,
});
const pricing = {
  success: true,
  vendors: [
    { id: 1, name: 'Anthropic' },
    { id: 2, name: 'OpenAI' },
  ],
  usable_group: {
    Claude: '',
    GPT: '',
    Shared: '',
    Empty: '',
    ...Object.fromEntries(
      Array.from({ length: 24 }, (_, i) => [`Unused-${i}`, '']),
    ),
  },
  group_ratio: { Claude: 1, GPT: 1, Shared: 0, Empty: 1, Forbidden: 1 },
  supported_endpoint: {},
  auto_groups: [],
  data: [
    {
      model_name: 'claude-fixture',
      vendor_id: 1,
      enable_groups: ['Claude', 'Shared', 'Forbidden'],
      tags: 'tools',
      quota_type: 0,
      supported_endpoint_types: ['anthropic'],
    },
    {
      model_name: 'gpt-fixture',
      vendor_id: 2,
      enable_groups: ['GPT', 'Shared'],
      tags: 'vision',
      quota_type: 1,
      supported_endpoint_types: ['openai'],
    },
    {
      model_name: 'unknown-fixture',
      enable_groups: ['Shared'],
      tags: '',
      quota_type: 0,
      supported_endpoint_types: ['openai'],
    },
  ].map((model) => ({
    model_ratio: 1,
    completion_ratio: 1,
    model_price: 1,
    ...model,
  })),
};

const fixtureUser = {
  id: 7,
  username: 'health-fixture',
  role: 1,
  group: 'user',
};
const colorSamples = [
  { request_count: 0, success_rate: null },
  { request_count: 99, success_rate: 0 },
  { request_count: 100, success_rate: 95 },
  { request_count: 100, success_rate: 94 },
  { request_count: 100, success_rate: 80 },
  { request_count: 100, success_rate: 79 },
];
const healthGroups = [
  'Claude',
  'GPT',
  'Shared',
  'Empty',
  'Forbidden',
  'auto',
].map((group) => ({
  group,
  request_count: 100,
  success_count: 97,
  failure_count: 3,
  success_rate: 97,
  avg_ttft_ms: 1500,
  ttft_count: 97,
  cache_hit_rate: 50,
  cache_sample_count: 97,
  series: Array.from({ length: 24 }, (_, i) => ({
    ts: 1792180800 + i * 3600,
    ...colorSamples[i % colorSamples.length],
  })),
  ...({
    Claude: { success_count: 95, failure_count: 5, success_rate: 95 },
    GPT: { success_count: 80, failure_count: 20, success_rate: 80 },
    Shared: { success_count: 79, failure_count: 21, success_rate: 79 },
    Empty: {
      request_count: 99,
      success_count: 0,
      failure_count: 99,
      success_rate: 0,
    },
  }[group] || {}),
}));

try {
  for (const mobile of [false, true]) {
    const page = await browser.newPage();
    const errors = [];
    let failHealth = false;
    const healthQueries = [];
    page.on('pageerror', (err) => errors.push(err.message));
    await page.setViewport({ width: mobile ? 390 : 1440, height: 1100 });
    const initialStorage = await page.evaluateOnNewDocument(
      (user, dark) => {
        localStorage.setItem('i18nextLng', 'zh');
        localStorage.setItem('user', JSON.stringify(user));
        localStorage.setItem('theme-mode', dark ? 'dark' : 'light');
      },
      fixtureUser,
      mobile,
    );
    await page.setRequestInterception(true);
    page.on('request', async (request) => {
      const url = new URL(request.url());
      if (url.origin !== new URL(baseURL).origin) {
        await request.abort();
      } else if (url.pathname.startsWith('/api/')) {
        let body = { success: true, data: [] };
        if (url.pathname === '/api/pricing') body = pricing;
        if (url.pathname === '/api/user/self') {
          body = { success: true, data: fixtureUser };
        }
        if (url.pathname === '/api/health/groups') {
          healthQueries.push(url.search);
          body = failHealth
            ? { success: false, message: 'fixture health unavailable' }
            : {
                success: true,
                data: {
                  enabled: true,
                  updated_at: 1792267200,
                  groups: healthGroups
                    .filter(
                      (stats) =>
                        !url.searchParams.get('group') ||
                        stats.group === url.searchParams.get('group'),
                    )
                    .map((stats) =>
                      url.searchParams.get('model') &&
                      !['GPT', 'Shared', 'Empty'].includes(stats.group)
                        ? { ...stats, request_count: 0, success_rate: null }
                        : stats,
                    ),
                },
              };
        }
        if (url.pathname === '/api/status') {
          body = {
            success: true,
            data: {
              system_name: 'New API',
              logo: '/logo.png',
              price: 1,
              usd_exchange_rate: 1,
              quota_per_unit: 500000,
              quota_display_type: 'USD',
            },
          };
        }
        await request.respond({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(body),
        });
      } else {
        await request.continue();
      }
    });
    await page.goto(`${baseURL}/pricing`, { waitUntil: 'networkidle0' });
    const scope = '.semi-modal-body';
    const clickText = async (text, root = scope) => {
      await page.waitForFunction(
        (root, text) =>
          Array.from(document.querySelectorAll(`${root} button`)).some(
            (b) => b.textContent.trim().startsWith(text) && !b.disabled,
          ),
        {},
        root,
        text,
      );
      await page.evaluate(
        (root, text) => {
          Array.from(document.querySelectorAll(`${root} button`))
            .find((b) => b.textContent.trim().startsWith(text) && !b.disabled)
            .click();
        },
        root,
        text,
      );
    };
    // Top vendor buttons are never hidden behind an overflow menu.
    await page.waitForSelector('.pricing-vendors button');
    if (!mobile) {
      await page.waitForSelector('.pricing-group-list button');
      await clickText('Anthropic', '.pricing-vendors');
      await clickText('Claude', '.pricing-group-list');
      await clickText('OpenAI', '.pricing-vendors');
      assert(
        await page.$eval(
          '.pricing-group-list',
          (el) => !el.textContent.includes('Claude'),
        ),
      );
      assert(
        await page.$eval('.pricing-price-context', (el) =>
          el.textContent.includes('全部分组'),
        ),
      );
      await clickText('GPT', '.pricing-group-list');
      assert(
        await page.$eval('.pricing-price-context', (el) =>
          el.textContent.includes('GPT'),
        ),
      );
      assert(
        !(await page.$eval('.pricing-sidebar', (el) =>
          el.textContent.includes('Forbidden'),
        )),
      );
      await clickText('全部分组', '.pricing-group-list');
      await clickText('全部供应商', '.pricing-vendors');
    }
    await clickText('更多筛选', 'body');
    await page.waitForFunction(
      (root) => document.querySelector(root)?.textContent.includes('Shared'),
      {},
      scope,
    );
    const filterLabels = (heading) =>
      page.evaluate(
        (root, heading) => {
          const title = Array.from(
            document.querySelectorAll(`${root} .semi-divider_inner-text`),
          ).find((el) => el.textContent.trim() === heading);
          return Array.from(
            title.closest('.mb-8').querySelectorAll('button'),
          ).map((b) => ({
            text: b.querySelector('.sbg-ellipsis').textContent,
            disabled: b.disabled,
          }));
        },
        scope,
        heading,
      );
    const groupLabels = () => filterLabels('可用令牌分组');
    assert((await groupLabels()).some((b) => b.text === 'Empty' && b.disabled));
    await clickText('Anthropic');
    assert.deepEqual(
      (await groupLabels()).map((b) => b.text),
      ['全部分组', 'Claude', 'Shared'],
    );
    assert(!(await groupLabels()).some((b) => b.disabled));
    assert.deepEqual(
      (await filterLabels('标签')).map((b) => b.text),
      ['全部标签', 'tools'],
    );
    assert.deepEqual(
      (await filterLabels('计费类型')).map((b) => b.text),
      ['全部类型', '按量计费'],
    );
    assert.deepEqual(
      (await filterLabels('端点类型')).map((b) => b.text),
      ['全部端点', 'anthropic'],
    );
    assert.equal(
      await page.evaluate((root) => {
        const title = Array.from(
          document.querySelectorAll(`${root} .semi-divider_inner-text`),
        ).find((el) => el.textContent.trim() === '可用令牌分组');
        return title.closest('.mb-8').textContent.includes('展开更多');
      }, scope),
      false,
    );
    if (process.env.PRICING_SCREENSHOT_DIR) {
      await page.screenshot({
        path: `${process.env.PRICING_SCREENSHOT_DIR}/${mobile ? 'mobile' : 'desktop'}.png`,
        fullPage: true,
      });
    }
    // A shared zero-ratio group remains available and does not lock vendor switching.
    await clickText('Shared');
    await clickText('OpenAI');
    assert.deepEqual(
      (await groupLabels()).map((b) => b.text),
      ['全部分组', 'GPT', 'Shared'],
    );
    await clickText('未知供应商');
    assert.deepEqual(
      (await groupLabels()).map((b) => b.text),
      ['全部分组', 'Shared'],
    );
    await clickText('重置', '.semi-modal');
    assert((await groupLabels()).some((b) => b.text === 'Empty' && b.disabled));
    await clickText('Anthropic');
    await clickText('Claude');
    {
      await clickText('确定', '.semi-modal');
      await page.waitForSelector('.semi-modal', { hidden: true });
    }
    await page.$eval('input[placeholder*="搜索模型"]', (input) => {
      const setter = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        'value',
      ).set;
      setter.call(input, 'no-such-model-fixture');
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    // Wait on real debounced filter state, not a fixed delay.
    await page.waitForFunction(
      () => !document.body.textContent.includes('claude-fixture'),
    );
    await clickText('更多筛选', 'body');
    assert.deepEqual(
      (await groupLabels()).map((b) => b.text),
      ['全部分组', 'Claude'],
    );
    assert(!(await groupLabels()).some((b) => b.disabled));
    await clickText('全部分组');
    assert.deepEqual(
      (await groupLabels()).map((b) => b.text),
      ['全部分组'],
    );
    await clickText('重置', '.semi-modal');
    assert((await groupLabels()).some((b) => b.text === 'GPT'));
    {
      await clickText('确定', '.semi-modal');
      await page.waitForSelector('.semi-modal', { hidden: true });
    }
    const healthTab = async () => {
      await page.waitForSelector('[role=tab]');
      await page.evaluate(() => {
        Array.from(document.querySelectorAll('[role=tab]'))
          .find((tab) => tab.textContent.trim() === '分组健康')
          .click();
      });
    };
    const expectGroups = async (expected) => {
      await page.waitForFunction(
        (expected) => {
          const actual = Array.from(
            document.querySelectorAll(
              '.group-health-overview .group-health-name',
            ),
          ).map((el) => el.textContent.trim());
          return JSON.stringify(actual) === JSON.stringify(expected);
        },
        {},
        expected,
      );
    };
    const chooseFilters = async (...labels) => {
      await clickText('更多筛选', 'body');
      for (const label of labels)
        await clickText(label, label === '重置' ? '.semi-modal' : scope);
      {
        await clickText('确定', '.semi-modal');
        await page.waitForSelector('.semi-modal', { hidden: true });
      }
    };
    await healthTab();
    await expectGroups(['Claude', 'GPT', 'Shared', 'Empty']);
    const colors = await page.$$eval(
      '.group-health-overview .group-health-panel',
      (panels) =>
        panels.map((panel) => ({
          name: panel.querySelector('.group-health-name').textContent,
          rate: panel.querySelector('.group-health-number').textContent,
          rateClass: panel.querySelector('.group-health-number').className,
          label: panel.querySelector('.group-health-badge').textContent,
          labelClass: panel.querySelector('.group-health-badge').className,
          bars: Array.from(panel.querySelectorAll('.group-health-bar')).map(
            (bar) => bar.className,
          ),
          firstBarTitle: panel.querySelector('.group-health-bar').title,
        })),
    );
    const expectedColors = [
      ['Claude', '95.00%', '部分波动', 'healthy'],
      ['GPT', '80.00%', '失败较多', 'warning'],
      ['Shared', '79.00%', '失败较多', 'degraded'],
      ['Empty', '0.00%', '失败较多', 'healthy'],
    ];
    const expectedBars = [
      'healthy',
      'healthy',
      'healthy',
      'warning',
      'warning',
      'degraded',
    ];
    for (const [i, panel] of colors.entries()) {
      const [name, rate, label, tone] = expectedColors[i];
      assert.deepEqual(
        [panel.name, panel.rate, panel.label],
        [name, rate, label],
      );
      assert(panel.rateClass.includes(`group-health-text-${tone}`));
      assert(panel.labelClass.includes(`group-health-badge-${tone}`));
      assert.deepEqual(
        panel.bars,
        Array.from(
          { length: 24 },
          (_, j) => `group-health-bar group-health-bar-${expectedBars[j % 6]}`,
        ),
      );
      assert(panel.firstBarTitle.includes('— · 暂无样本'));
    }
    // Clicking a green low-volume bucket still shows the measured 0% result.
    await page.click('.group-health-panel .group-health-bar:nth-child(2)');
    const bucketText = await page.$eval(
      '.group-health-bucket',
      (el) => el.textContent,
    );
    assert(bucketText.includes('0.00%'));
    assert(bucketText.includes('失败较多'));
    await page.click('.group-health-panel .group-health-bar:nth-child(2)');
    if (process.env.PRICING_SCREENSHOT_DIR) {
      await page.screenshot({
        path: `${process.env.PRICING_SCREENSHOT_DIR}/colors-${mobile ? 'mobile-dark' : 'desktop'}.png`,
        fullPage: true,
      });
    }
    await chooseFilters('OpenAI');
    await expectGroups(['GPT', 'Shared']);
    assert(
      await page.$eval('.group-health-overview', (el) =>
        el.textContent.includes('分组汇总包含该组全部模型'),
      ),
    );
    if (process.env.PRICING_SCREENSHOT_DIR) {
      if (mobile) await page.setViewport({ width: 320, height: 1000 });
      await page.screenshot({
        path: `${process.env.PRICING_SCREENSHOT_DIR}/health-${mobile ? 'mobile-dark' : 'desktop'}.png`,
        fullPage: true,
      });
    }
    // Exact-model API samples preserve a disabled group's outage.
    await page.click('.group-health-filter-select');
    await page.waitForSelector('.semi-select-option');
    const optionLabels = await page.$$eval('.semi-select-option', (options) =>
      options.map((option) => option.textContent.trim()),
    );
    assert.deepEqual(optionLabels, ['全部模型 · 分组汇总', 'gpt-fixture']);
    await page.evaluate(() =>
      Array.from(document.querySelectorAll('.semi-select-option'))
        .find((option) => option.textContent.trim() === 'gpt-fixture')
        .click(),
    );
    await expectGroups(['GPT', 'Shared', 'Empty']);
    assert(healthQueries.some((query) => query.includes('model=gpt-fixture')));
    await chooseFilters('Anthropic');
    await expectGroups(['Claude', 'Shared']);
    await chooseFilters('未知供应商');
    await expectGroups(['Shared']);
    await chooseFilters('重置');
    await expectGroups(['GPT', 'Shared', 'Empty']);
    await page.waitForFunction(() => {
      const el = document.querySelector('.group-health-filter-select');
      const r = el.getBoundingClientRect();
      return el.contains(
        document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2),
      );
    });
    await page.click('.group-health-filter-select');
    await page.waitForSelector('.semi-select-option');
    await page.evaluate(() =>
      Array.from(document.querySelectorAll('.semi-select-option'))
        .find((option) => option.textContent.trim() === '全部模型 · 分组汇总')
        .click(),
    );
    await chooseFilters('vision');
    await expectGroups(['GPT', 'Shared']);
    await chooseFilters('重置', '按次计费');
    await expectGroups(['GPT', 'Shared']);
    await chooseFilters('重置', 'anthropic');
    await expectGroups(['Claude', 'Shared']);
    await chooseFilters('重置', 'OpenAI', 'GPT');
    await expectGroups(['GPT']);
    await page.$eval('input[placeholder*="搜索模型"]', (input) => {
      const setter = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        'value',
      ).set;
      setter.call(input, 'no-such-model-fixture');
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await expectGroups([]);
    await page.waitForFunction(() =>
      document
        .querySelector('.group-health-overview')
        ?.textContent.includes('当前筛选下没有可用分组'),
    );
    await chooseFilters('重置');
    // The local exact-model choice can become valid again after reset.
    await page.waitForFunction(() => {
      const el = document.querySelector('.group-health-filter-select');
      const r = el.getBoundingClientRect();
      return el.contains(
        document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2),
      );
    });
    await page.click('.group-health-filter-select');
    await page.waitForSelector('.semi-select-option');
    await page.evaluate(() =>
      Array.from(document.querySelectorAll('.semi-select-option'))
        .find((option) => option.textContent.trim() === '全部模型 · 分组汇总')
        .click(),
    );
    await expectGroups(['Claude', 'GPT', 'Shared', 'Empty']);
    failHealth = true;
    await clickText('刷新', '.group-health-toolbar');
    await page.waitForSelector('.group-health-overview [role=alert]');
    await expectGroups([]);
    failHealth = false;
    await clickText('重试', '.group-health-overview');
    await expectGroups(['Claude', 'GPT', 'Shared', 'Empty']);
    await page.reload({ waitUntil: 'networkidle0' });
    await healthTab();
    await chooseFilters('OpenAI');
    await expectGroups(['GPT', 'Shared']);
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth > window.innerWidth,
      ),
      false,
    );
    const authenticatedHealthRequests = healthQueries.length;
    await page.removeScriptToEvaluateOnNewDocument(initialStorage.identifier);
    await page.evaluate(() => localStorage.removeItem('user'));
    await page.reload({ waitUntil: 'networkidle0' });
    await healthTab();
    await page.waitForFunction(() =>
      document
        .querySelector('.group-health-overview')
        ?.textContent.includes('登录后查看可用分组的调用表现'),
    );
    assert.equal(healthQueries.length, authenticatedHealthRequests);
    await expectGroups([]);
    assert.deepEqual(errors, []);
    console.log(`${mobile ? 'Mobile' : 'Desktop'} pricing workflow passed.`);
    await page.close();
  }
} catch (error) {
  const pages = await browser.pages();
  await pages
    .at(-1)
    ?.screenshot({ path: '/tmp/pricing-layout-failure.png', fullPage: true });
  throw error;
} finally {
  await browser.close();
}
