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
      Array.from({ length: 24 }, (_, i) => [
        i === 0
          ? 'LongGroupName-用于检查非常长的分组名称是否能够完整换行显示而不挤掉倍率标签'
          : `Unused-${i}`,
        '',
      ]),
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
  { request_count: 1, success_rate: 0 },
  { request_count: 29, success_rate: 100 },
  { request_count: 30, success_rate: 0 },
  { request_count: 99, success_rate: 0 },
  { request_count: 100, success_rate: 0 },
  { request_count: 100, success_rate: 95 },
  { request_count: 100, success_rate: 94.99 },
  { request_count: 100, success_rate: 80 },
  { request_count: 100, success_rate: 79.99 },
  {
    request_count: 7,
    success_count: 1,
    failure_count: 6,
    success_rate: 100 / 7,
  },
  {
    request_count: 201,
    success_count: 65,
    failure_count: 136,
    success_rate: 6500 / 201,
  },
];
const reportedSeries = [
  [7, 1],
  [8, 4],
  [7, 1],
  [8, 2],
  [7, 4],
  [8, 4],
  [7, 3],
  [8, 3],
  [7, 2],
  [8, 2],
  [7, 1],
  [7, 2],
  [9, 2],
  [6, 1],
  [8, 3],
  [9, 2],
  [11, 3],
  [7, 1],
  [8, 2],
  [8, 3],
  [12, 3],
  [8, 3],
  [21, 9],
  [5, 4],
].map(([request_count, success_count], i) => ({
  ts: 1792180800 + i * 3600,
  request_count,
  success_count,
  failure_count: request_count - success_count,
  success_rate: (success_count / request_count) * 100,
}));
assert.equal(
  reportedSeries.reduce((total, bucket) => total + bucket.request_count, 0),
  201,
);
assert.equal(
  reportedSeries.reduce((total, bucket) => total + bucket.success_count, 0),
  65,
);
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
    let healthOverrides = {};
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
                    .map((stats) => ({
                      ...stats,
                      ...healthOverrides[stats.group],
                    }))
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
    const scope = '.pricing-filter-drawer .semi-sidesheet-body';
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
    await page.waitForSelector('.pricing-filter-section summary');
    // Collapsing a section must not clear or alter filters.
    const summary = await page.$('.pricing-filter-section summary');
    await summary.focus();
    await page.keyboard.press('Enter');
    assert.equal(
      await page.$eval('.pricing-filter-section', (el) => el.open),
      false,
    );
    await page.keyboard.press('Enter');
    assert.equal(
      await page.$eval('.pricing-filter-section', (el) => el.open),
      true,
    );
    await page.waitForFunction(() => {
      const r = document
        .querySelector('.pricing-filter-drawer .semi-sidesheet-inner')
        .getBoundingClientRect();
      return Math.abs(r.right - innerWidth) < 1;
    });
    assert.equal(
      await page.$eval(
        '.pricing-filter-drawer',
        (el) => el.scrollWidth > el.clientWidth,
      ),
      false,
    );
    assert(
      await page.$$eval('.pricing-filter-chip', (chips) =>
        chips.every((chip) => chip.scrollWidth <= chip.clientWidth + 1),
      ),
    );
    await page.screenshot({
      path: `/tmp/pricing-drawer-${mobile ? 'mobile-dark' : 'desktop'}.png`,
      fullPage: true,
    });
    await page.keyboard.press('Escape');
    await page.waitForSelector('.pricing-filter-drawer', { hidden: true });
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
            document.querySelectorAll(`${root} summary`),
          ).find((el) => el.textContent.trim() === heading);
          return Array.from(
            title.closest('.pricing-filter-section').querySelectorAll('button'),
          ).map((b) => ({
            text: b.querySelector('.pricing-filter-chip-label').textContent,
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
    await page.waitForFunction(() => {
      const r = document
        .querySelector('.pricing-filter-drawer .semi-sidesheet-inner')
        .getBoundingClientRect();
      return Math.abs(r.right - innerWidth) < 1;
    });
    await page.screenshot({
      path: `/tmp/pricing-drawer-selected-${mobile ? 'mobile-dark' : 'desktop'}.png`,
      fullPage: true,
    });
    assert.equal(
      await page.evaluate((root) => {
        const title = Array.from(
          document.querySelectorAll(`${root} summary`),
        ).find((el) => el.textContent.trim() === '可用令牌分组');
        return title
          .closest('.pricing-filter-section')
          .textContent.includes('展开更多');
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
    await clickText('重置', '.pricing-filter-drawer');
    assert((await groupLabels()).some((b) => b.text === 'Empty' && b.disabled));
    await clickText('Anthropic');
    await clickText('Claude');
    {
      await clickText('关闭', '.pricing-filter-drawer');
      await page.waitForSelector('.pricing-filter-drawer', { hidden: true });
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
    await clickText('重置', '.pricing-filter-drawer');
    assert((await groupLabels()).some((b) => b.text === 'GPT'));
    {
      await clickText('关闭', '.pricing-filter-drawer');
      await page.waitForSelector('.pricing-filter-drawer', { hidden: true });
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
        await clickText(
          label,
          label === '重置' ? '.pricing-filter-drawer' : scope,
        );
      {
        await clickText('关闭', '.pricing-filter-drawer');
        await page.waitForSelector('.pricing-filter-drawer', { hidden: true });
      }
    };
    const screenshotPanel = async (group, prefix) => {
      if (!process.env.PRICING_SCREENSHOT_DIR) return;
      await page.mouse.move(0, 0);
      await page.waitForFunction(
        () =>
          document.querySelectorAll('.semi-toast-content, .Toastify__toast')
            .length === 0,
      );
      const handle = await page.evaluateHandle(
        (group) =>
          Array.from(
            document.querySelectorAll(
              '.group-health-overview .group-health-panel',
            ),
          ).find(
            (panel) =>
              panel.querySelector('.group-health-name')?.textContent === group,
          ),
        group,
      );
      const panel = handle.asElement();
      assert(panel, `Missing screenshot panel: ${group}`);
      await panel.evaluate((el) => el.scrollIntoView({ block: 'center' }));
      await panel.screenshot({
        path: `${process.env.PRICING_SCREENSHOT_DIR}/${prefix}-${mobile ? 'mobile-dark' : 'desktop'}.png`,
      });
      await handle.dispose();
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
          label: panel.querySelector(
            '.group-health-badge:not(.group-health-badge-limited)',
          ).textContent,
          labelClass: panel.querySelector(
            '.group-health-badge:not(.group-health-badge-limited)',
          ).className,
          bars: Array.from(panel.querySelectorAll('.group-health-bar')).map(
            (bar) => bar.className,
          ),
          firstBarTitle: panel.querySelector('.group-health-bar').title,
        })),
    );
    const expectedColors = [
      ['Claude', '95.00%', '运行稳定', 'healthy'],
      ['GPT', '80.00%', '部分波动', 'warning'],
      ['Shared', '79.00%', '失败较多', 'degraded'],
      ['Empty', '0.00%', '失败较多', 'degraded'],
    ];
    const expectedBars = [
      'empty',
      'degraded',
      'healthy',
      'degraded',
      'degraded',
      'degraded',
      'healthy',
      'warning',
      'warning',
      'degraded',
      'degraded',
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
          (_, j) =>
            `group-health-bar group-health-bar-${expectedBars[j % expectedBars.length]}`,
        ),
      );
      assert(panel.firstBarTitle.includes('— · 暂无样本'));
    }
    const legend = await page.$eval(
      '.group-health-legend',
      (el) => el.textContent,
    );
    for (const label of [
      '成功率 ≥ 95%',
      '80% ≤ 成功率 < 95%',
      '成功率 < 80%',
      '暂无样本',
    ]) {
      assert(legend.includes(label), `Missing health legend: ${label}`);
    }
    assert(!legend.includes('99%'));
    assert(!legend.includes('90%'));
    assert(!legend.includes('不足 30 次'));
    assert(
      await page.$eval('.group-health-overview', (el) =>
        el.textContent.includes(
          '少于 30 次调用时提示样本不足，颜色仍按实际成功率显示。',
        ),
      ),
    );
    // Low-volume failures remain red and expose the sample warning independently.
    await page.click('.group-health-panel .group-health-bar:nth-child(2)');
    const bucketText = await page.$eval(
      '.group-health-bucket',
      (el) => el.textContent,
    );
    assert(bucketText.includes('0.00%'));
    assert(bucketText.includes('失败较多'));
    assert(bucketText.includes('样本不足'));
    assert(
      await page.$eval(
        '.group-health-bucket .group-health-inline strong',
        (el) => el.classList.contains('group-health-text-degraded'),
      ),
    );
    const lowSampleBar = await page.$eval(
      '.group-health-panel .group-health-bar:nth-child(2)',
      (el) => ({ title: el.title, label: el.getAttribute('aria-label') }),
    );
    for (const text of [lowSampleBar.title, lowSampleBar.label]) {
      assert(text.includes('失败较多'));
      assert(text.includes('样本不足'));
    }
    await page.click('.group-health-panel .group-health-bar:nth-child(2)');
    // Reproduce the screenshot: 65/201 daily success and 1/7 hourly success.
    healthOverrides = {
      Shared: {
        request_count: 201,
        success_count: 65,
        failure_count: 136,
        success_rate: 6500 / 201,
        avg_ttft_ms: 5200,
        ttft_count: 13,
        cache_hit_rate: 61.87,
        cache_sample_count: 65,
        series: reportedSeries,
      },
      Empty: {
        request_count: 0,
        success_count: 0,
        failure_count: 0,
        success_rate: null,
        series: reportedSeries.map(({ ts }) => ({
          ts,
          request_count: 0,
          success_count: 0,
          failure_count: 0,
          success_rate: null,
        })),
      },
    };
    await clickText('刷新', '.group-health-toolbar');
    await page.waitForFunction(() =>
      Array.from(document.querySelectorAll('.group-health-panel')).some(
        (panel) =>
          panel.querySelector('.group-health-name')?.textContent === 'Shared' &&
          panel.querySelector('.group-health-number')?.textContent === '32.34%',
      ),
    );
    const panelSnapshot = (group) =>
      page.$$eval(
        '.group-health-overview .group-health-panel',
        (panels, group) => {
          const panel = panels.find(
            (panel) =>
              panel.querySelector('.group-health-name').textContent === group,
          );
          return {
            rate: panel.querySelector('.group-health-number').textContent,
            rateClass: panel.querySelector('.group-health-number').className,
            badges: Array.from(
              panel.querySelectorAll('.group-health-badge'),
            ).map((el) => ({ text: el.textContent, className: el.className })),
            caption: panel.querySelector('.group-health-caption').textContent,
          };
        },
        group,
      );
    const dailySnapshot = await panelSnapshot('Shared');
    assert.equal(dailySnapshot.rate, '32.34%');
    assert(dailySnapshot.rateClass.includes('group-health-text-degraded'));
    assert.deepEqual(
      dailySnapshot.badges.map(({ text }) => text),
      ['失败较多'],
    );
    assert(dailySnapshot.caption.includes('201 次调用'));
    assert(!dailySnapshot.caption.includes('样本不足'));
    const emptySnapshot = await panelSnapshot('Empty');
    assert.equal(emptySnapshot.rate, '—');
    assert(emptySnapshot.rateClass.includes('group-health-text-empty'));
    assert.deepEqual(
      emptySnapshot.badges.map(({ text }) => text),
      ['暂无样本'],
    );
    assert.equal(emptySnapshot.caption, '暂无样本');
    await page.evaluate(() =>
      Array.from(document.querySelectorAll('.group-health-panel'))
        .find(
          (panel) =>
            panel.querySelector('.group-health-name').textContent === 'Shared',
        )
        .querySelector('.group-health-bar:nth-child(11)')
        .click(),
    );
    const screenshotBucket = await page.$eval(
      '.group-health-bucket',
      (el) => el.textContent,
    );
    for (const text of ['14.29%', '失败较多', '样本不足', '成功 1 · 失败 6'])
      assert(screenshotBucket.includes(text));
    assert(
      await page.$eval(
        '.group-health-bucket .group-health-inline strong',
        (el) => el.classList.contains('group-health-text-degraded'),
      ),
    );
    await screenshotPanel('Shared', 'reported');
    await page.evaluate(() =>
      Array.from(document.querySelectorAll('.group-health-panel'))
        .find(
          (panel) =>
            panel.querySelector('.group-health-name').textContent === 'Shared',
        )
        .querySelector('.group-health-bar:nth-child(11)')
        .click(),
    );
    // Apply the same low-sample semantics to the daily summary and its status badge.
    healthOverrides.Shared = {
      request_count: 7,
      success_count: 1,
      failure_count: 6,
      success_rate: 100 / 7,
      avg_ttft_ms: 5200,
      ttft_count: 1,
      cache_hit_rate: 61.87,
      cache_sample_count: 1,
      series: reportedSeries.map((bucket, i) =>
        i === 0
          ? bucket
          : {
              ts: bucket.ts,
              request_count: 0,
              success_count: 0,
              failure_count: 0,
              success_rate: null,
            },
      ),
    };
    await clickText('刷新', '.group-health-toolbar');
    await page.waitForFunction(() =>
      Array.from(document.querySelectorAll('.group-health-panel')).some(
        (panel) =>
          panel.querySelector('.group-health-name')?.textContent === 'Shared' &&
          panel.querySelector('.group-health-number')?.textContent === '14.29%',
      ),
    );
    const limitedSnapshot = await panelSnapshot('Shared');
    assert(limitedSnapshot.rateClass.includes('group-health-text-degraded'));
    assert.deepEqual(
      limitedSnapshot.badges.map(({ text }) => text).sort(),
      ['失败较多', '样本不足'].sort(),
    );
    assert(
      limitedSnapshot.badges.some(
        ({ text, className }) =>
          text === '失败较多' &&
          className.includes('group-health-badge-degraded'),
      ),
    );
    assert(
      limitedSnapshot.badges.some(
        ({ text, className }) =>
          text === '样本不足' &&
          className.includes('group-health-badge-limited'),
      ),
    );
    assert(limitedSnapshot.caption.includes('7 次调用'));
    assert(limitedSnapshot.caption.includes('样本不足'));
    await screenshotPanel('Shared', 'colors');
    healthOverrides = {};
    await clickText('刷新', '.group-health-toolbar');
    await page.waitForFunction(() =>
      Array.from(document.querySelectorAll('.group-health-panel')).some(
        (panel) =>
          panel.querySelector('.group-health-name')?.textContent === 'Shared' &&
          panel.querySelector('.group-health-number')?.textContent === '79.00%',
      ),
    );
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
    // The model drawer reuses the same tones in its compact pricing summary
    // and in the expanded performance tab.
    healthOverrides = {
      Shared: {
        request_count: 7,
        success_count: 1,
        failure_count: 6,
        success_rate: 100 / 7,
        avg_ttft_ms: 5200,
        ttft_count: 1,
        cache_sample_count: 1,
      },
    };
    await page.evaluate(() =>
      Array.from(document.querySelectorAll('[role=tab]'))
        .find((tab) => tab.textContent.trim() === '模型列表')
        .click(),
    );
    await page.waitForFunction(() =>
      Array.from(
        document.querySelectorAll('.semi-card h3, .semi-table-row span'),
      ).some((el) => el.textContent.trim() === 'gpt-fixture'),
    );
    await page.evaluate(() =>
      Array.from(
        document.querySelectorAll('.semi-card h3, .semi-table-row span'),
      )
        .find((el) => el.textContent.trim() === 'gpt-fixture')
        .click(),
    );
    await page.waitForFunction(() =>
      Array.from(
        document.querySelectorAll('.semi-sidesheet .group-health-price-row'),
      ).some(
        (row) =>
          row.firstElementChild.textContent === 'Shared' &&
          row.textContent.includes('14.29%'),
      ),
    );
    const compactRows = await page.$$eval(
      '.semi-sidesheet .group-health-price-row',
      (rows) =>
        rows.map((row) => ({
          group: row.firstElementChild.textContent,
          rate: row.querySelector('.group-health-inline strong')?.textContent,
          rateClass: row.querySelector('.group-health-inline strong')
            ?.className,
          text: row.textContent,
        })),
    );
    const compactShared = compactRows.find(({ group }) => group === 'Shared');
    assert.equal(compactShared.rate, '14.29%');
    assert(compactShared.rateClass.includes('group-health-text-degraded'));
    assert(compactShared.text.includes('样本不足'));
    const compactGPT = compactRows.find(({ group }) => group === 'GPT');
    assert.equal(compactGPT.rate, '80.00%');
    assert(compactGPT.rateClass.includes('group-health-text-warning'));
    assert(!compactGPT.text.includes('样本不足'));
    await clickText('查看时间趋势', '.semi-sidesheet');
    await page.waitForSelector('.semi-sidesheet .group-health-panel');
    const detailPanels = await page.$$eval(
      '.semi-sidesheet .group-health-panel',
      (panels) =>
        panels.map((panel) => ({
          group: panel.querySelector('.group-health-name').textContent,
          rateClass: panel.querySelector('.group-health-number').className,
          status: panel.querySelector(
            '.group-health-badge:not(.group-health-badge-limited)',
          ).textContent,
          limited: Boolean(panel.querySelector('.group-health-badge-limited')),
        })),
    );
    const detailShared = detailPanels.find(({ group }) => group === 'Shared');
    assert(detailShared.rateClass.includes('group-health-text-degraded'));
    assert.equal(detailShared.status, '失败较多');
    assert.equal(detailShared.limited, true);
    const detailGPT = detailPanels.find(({ group }) => group === 'GPT');
    assert(detailGPT.rateClass.includes('group-health-text-warning'));
    assert.equal(detailGPT.status, '部分波动');
    assert.equal(detailGPT.limited, false);
    const detailLegend = await page.$eval(
      '.semi-sidesheet .group-health-legend',
      (el) => el.textContent,
    );
    for (const label of [
      '成功率 ≥ 95%',
      '80% ≤ 成功率 < 95%',
      '成功率 < 80%',
      '暂无样本',
    ])
      assert(detailLegend.includes(label));
    assert(
      await page.$eval('.semi-sidesheet', (el) =>
        el.textContent.includes(
          '少于 30 次调用时提示样本不足，颜色仍按实际成功率显示。',
        ),
      ),
    );
    await page.evaluate(() =>
      document
        .querySelector('.group-health-price-summary')
        .closest('.semi-sidesheet')
        .querySelector('.semi-sidesheet-close')
        .click(),
    );
    await page.waitForSelector('.semi-sidesheet .group-health-panel', {
      hidden: true,
    });
    healthOverrides = {};
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
