import assert from 'node:assert/strict';
import http from 'node:http';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import puppeteer from 'puppeteer-core';

const root = fileURLToPath(
  new URL('../public/image-playground/', import.meta.url),
);
const mime = {
  '.html': 'text/html',
  '.js': 'text/javascript',
  '.css': 'text/css',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.json': 'application/json',
};
const server = http.createServer(async (req, res) => {
  try {
    const pathname = decodeURIComponent(
      new URL(req.url, 'http://localhost').pathname,
    );
    const file = path.resolve(
      root,
      '.' + (pathname === '/' ? '/index.html' : pathname),
    );
    if (!file.startsWith(root)) {
      res.writeHead(403);
      res.end();
      return;
    }
    const data = await readFile(file);
    res.writeHead(200, {
      'Content-Type': mime[path.extname(file)] || 'application/octet-stream',
    });
    res.end(data);
  } catch {
    res.writeHead(404);
    res.end();
  }
});
let browser;
try {
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const origin = `http://127.0.0.1:${server.address().port}`;
  browser = await puppeteer.launch({
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    headless: true,
  });
  const page = await browser.newPage();
  page.setDefaultTimeout(15000);
  await page.setViewport({ width: 1440, height: 1000 });
  await page.evaluateOnNewDocument(() => {
    localStorage.setItem('image-playground-language', 'zh-CN');
  });
  const errors = [],
    calls = [];
  let held = null,
    holdImage = false;
  page.on('pageerror', (error) => errors.push(error.message));
  await page.setRequestInterception(true);
  const png =
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLbtAAAAABJRU5ErkJggg==';
  page.on('request', async (req) => {
    const url = new URL(req.url());
    if (url.protocol === 'data:' || url.protocol === 'blob:')
      return req.continue();
    if (url.origin !== origin) return req.abort();
    if (!url.pathname.startsWith('/v1/')) return req.continue();
    const body = req.postData()?.startsWith('{')
      ? JSON.parse(req.postData())
      : req.postData();
    calls.push({ path: url.pathname, body });
    let response;
    if (url.pathname === '/v1/responses') {
      assert.equal(body.model, 'gpt-6-sol');
      if (body.max_output_tokens === 32)
        response = {
          output: [
            {
              type: 'message',
              content: [{ type: 'output_text', text: '<title>测试猫</title>' }],
            },
          ],
        };
      else {
        assert(body.tools.every((tool) => tool.type === 'function'));
        const completed = JSON.stringify(body.input).includes(
          'function_call_output',
        );
        response = {
          output: completed
            ? [
                {
                  type: 'message',
                  content: [{ type: 'output_text', text: '图片已完成' }],
                },
              ]
            : [
                {
                  type: 'function_call',
                  name: 'generate_image_batch',
                  call_id: 'browser-image',
                  arguments: '{"images":[{"id":"one","prompt":"cat"}]}',
                },
              ],
        };
      }
    } else if (url.pathname === '/v1/images/generations') {
      assert.equal(body.model, 'gpt-image-2');
      if (holdImage) {
        held = req;
        return;
      }
      response = { data: [{ b64_json: png }] };
    } else {
      errors.push(`Unexpected API: ${url.pathname}`);
      return req.abort();
    }
    await req
      .respond({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(response),
      })
      .catch(() => {});
  });
  const profile = {
    id: 'newapi-image-playground',
    name: '默认',
    provider: 'openai',
    baseUrl: `${origin}/v1`,
    apiKey: 'synthetic-key',
    model: 'gpt-image-2',
    apiMode: 'images',
    agentTextModel: 'gpt-6-sol',
    streamImages: false,
  };
  const settings = {
    profiles: [profile],
    supportsEcommerce: true,
    defaultImageModel: 'gpt-image-2',
    defaultPlanModel: 'gpt-6-sol',
  };
  await page.goto(
    `${origin}/?appMode=gallery&settings=${encodeURIComponent(JSON.stringify(settings))}`,
    { waitUntil: 'networkidle0' },
  );
  const clickText = async (text) =>
    page.evaluate((value) => {
      const button = [...document.querySelectorAll('button')].find(
        (b) =>
          b.textContent.trim() === value && b.getBoundingClientRect().width > 0,
      );
      if (!button) throw new Error(`Missing visible button: ${value}`);
      button.click();
    }, text);
  await clickText('Agent');
  await page.waitForFunction(
    () => !document.body.innerText.includes('需要 Responses API 配置'),
  );
  const input = '[contenteditable="true"]';
  await page.waitForSelector(input);
  await page.type(input, '画一只猫');
  await page.evaluate(() =>
    [...document.querySelectorAll('button[aria-label="生成图像"]')]
      .find((b) => b.getBoundingClientRect().width > 0)
      .click(),
  );
  await page.waitForFunction(() =>
    document.body.innerText.includes('图片已完成'),
  );
  assert.equal(
    calls.filter((call) => call.path === '/v1/images/generations').length,
    1,
  );
  assert(
    calls.some(
      (call) =>
        call.path === '/v1/responses' &&
        JSON.stringify(call.body.input).includes('function_call_output'),
    ),
  );
  await clickText('画廊');
  await page.waitForFunction(() => document.querySelectorAll('img').length > 0);
  await clickText('电商套图');
  await page.waitForSelector('input[placeholder="例如：便携榨汁杯"]');
  await clickText('Agent');
  await page.setViewport({ width: 390, height: 844 });
  await page.screenshot({
    path:
      process.env.WORKSPACE_SCREENSHOT ||
      '/tmp/image-workspace-agent-mobile.png',
    fullPage: true,
  });
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth + 1,
    ),
  );
  holdImage = true;
  await page.click(input);
  await page.keyboard.down('Meta');
  await page.keyboard.press('KeyA');
  await page.keyboard.up('Meta');
  await page.keyboard.type('再画一只狗');
  // Start a fresh conversation so the fixture emits a new tool call.
  await page.evaluate(() =>
    document.querySelector('button[title="新对话"]').click(),
  );
  await page.type(input, '画一只狗');
  await page.evaluate(() =>
    [...document.querySelectorAll('button[aria-label="生成图像"]')]
      .find((b) => b.getBoundingClientRect().width > 0)
      .click(),
  );
  const deadline = Date.now() + 15000;
  while (!held && Date.now() < deadline)
    await new Promise((resolve) => setTimeout(resolve, 25));
  assert(held, 'image request should be in flight');
  await page.evaluate(() =>
    [...document.querySelectorAll('button[aria-label="停止生成"]')]
      .find((b) => b.getBoundingClientRect().width > 0)
      .click(),
  );
  await page.waitForSelector('button[aria-label="生成图像"]');
  await held.abort().catch(() => {});
  held = null;
  assert.equal(
    calls.filter((call) => call.path === '/v1/images/generations').length,
    2,
  );
  assert.deepEqual(errors, []);
  console.log(
    'PASS: Agent opens without settings, text→image→result loop, gallery return, ecommerce entry, mobile layout and stop; only synthetic local requests.',
  );
} finally {
  if (browser) await browser.close();
  await new Promise((resolve) => server.close(resolve));
  server.closeAllConnections();
}
