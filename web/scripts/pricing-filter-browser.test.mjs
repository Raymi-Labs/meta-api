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

try {
  for (const mobile of [false, true]) {
    const page = await browser.newPage();
    const errors = [];
    page.on('pageerror', (err) => errors.push(err.message));
    await page.setViewport({ width: mobile ? 390 : 1440, height: 1100 });
    await page.evaluateOnNewDocument(() => {
      localStorage.setItem('i18nextLng', 'zh');
    });
    await page.setRequestInterception(true);
    page.on('request', async (request) => {
      const url = new URL(request.url());
      if (url.origin !== new URL(baseURL).origin) {
        await request.abort();
      } else if (url.pathname.startsWith('/api/')) {
        let body = { success: true, data: [] };
        if (url.pathname === '/api/pricing') body = pricing;
        if (url.pathname === '/api/status') {
          body = {
            success: true,
            data: {
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
    const scope = mobile ? '.semi-modal-body' : '.pricing-sidebar';
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
    if (mobile) await clickText('筛选', 'body');
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
    await clickText('重置', mobile ? '.semi-modal' : scope);
    assert((await groupLabels()).some((b) => b.text === 'Empty' && b.disabled));
    await clickText('Anthropic');
    await clickText('Claude');
    if (mobile) await clickText('确定', '.semi-modal');
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
    if (mobile) await clickText('筛选', 'body');
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
    await clickText('重置', mobile ? '.semi-modal' : scope);
    assert((await groupLabels()).some((b) => b.text === 'GPT'));
    assert.deepEqual(errors, []);
    console.log(`${mobile ? 'Mobile' : 'Desktop'} pricing workflow passed.`);
    await page.close();
  }
} finally {
  await browser.close();
}
