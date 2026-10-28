import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import http from 'node:http';
import puppeteer from 'puppeteer-core';

// Exercise the actual wallet handlers with mocked order creation. No production
// API or payment provider is contacted and no real order is created.
const source = await readFile(
  new URL('../src/components/topup/index.jsx', import.meta.url),
  'utf8',
);
const helper = await readFile(
  new URL('../src/helpers/paymentNavigation.js', import.meta.url),
  'utf8',
);
const extractHandler = (name, nextName) => {
  const start = source.indexOf(`  const ${name} =`);
  const end = source.indexOf(`  const ${nextName} =`, start);
  assert.ok(start >= 0 && end > start, `Missing handler: ${name}`);
  return source.slice(start, end);
};
const handlers =
  extractHandler('onlineTopUp', 'onlineCreemTopUp') +
  extractHandler('onlineCreemTopUp', 'getUserQuota');
const orders = [];
const visits = [];
let reply;
let browser;
let origin;
const server = http.createServer(async (req, res) => {
  if (req.url === '/paymentNavigation.js') {
    res.setHeader('Content-Type', 'text/javascript');
    res.end(helper);
  } else if (req.url === '/order') {
    let body = '';
    for await (const chunk of req) body += chunk;
    orders.push(JSON.parse(body));
    res.setHeader('Content-Type', 'application/json');
    res.end(JSON.stringify(reply));
  } else if (req.url.startsWith('/checkout')) {
    let body = '';
    for await (const chunk of req) body += chunk;
    visits.push({ method: req.method, url: req.url, body });
    res.end('Mock checkout');
  } else {
    res.setHeader('Content-Type', 'text/html; charset=utf-8');
    res.end(`<!doctype html><meta name="viewport" content="width=device-width">
      <button id="pay">Pay</button><button id="creem">Creem</button>
      <script type="module">
      import { redirectToPayment, submitPaymentForm } from '/paymentNavigation.js';
      window.paymentHelpers = { redirectToPayment, submitPaymentForm };
      window.errors = [];
      // Fail the test if any path attempts to create a popup.
      window.open = () => { throw new Error('Popup blocked'); };
      const t = value => value;
      const showError = message => window.errors.push(message);
      const setConfirmLoading = value => { window.loading = value; };
      const setOpen = () => {};
      const setCreemOpen = () => {};
      const topUpCount = '10';
      const selectedCreemProduct = { productId: 'mock-product' };
      const payWay = new URLSearchParams(location.search).get('method');
      const aliveRef = {current:true};
      const paymentBusyRef = {current:false};
      const paymentSessionRef = {current:1};
      const open = true;
      const confirmedPayment = {count:topUpCount,way:payWay,quote:{amount:2,currency:'CNY'}};
      const API = { post: async (endpoint, payload) => ({
        data: await (await fetch('/order', {
          method: 'POST',
          body: JSON.stringify({ endpoint, payload }),
        })).json(),
      }) };
      ${handlers}
      document.querySelector('#pay').onclick = onlineTopUp;
      document.querySelector('#creem').onclick = onlineCreemTopUp;
      window.ready = true;
      </script>`);
  }
});

try {
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  origin = `http://127.0.0.1:${server.address().port}`;
  browser = await puppeteer.launch({
    executablePath:
      process.env.CHROME_PATH ||
      '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    headless: true,
  });
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const openWallet = async (method) => {
    await page.goto(`${origin}/wallet?method=${encodeURIComponent(method)}`);
    await page.waitForFunction(() => window.ready === true);
  };
  const initialPages = (await browser.pages()).length;
  const cases = [
    ['stripe', '/api/user/stripe/pay', 'pay_link'],
    ['waffo', '/api/user/waffo/pay', 'payment_url'],
    ['waffo:2', '/api/user/waffo/pay', 'payment_url'],
    ['creem', '/api/user/creem/pay', 'checkout_url'],
  ];

  for (const mobile of [true, false]) {
    await page.setViewport({
      width: mobile ? 390 : 1280,
      height: 844,
      isMobile: mobile,
      hasTouch: mobile,
    });
    for (const [method, endpoint, field] of cases) {
      const checkout = `${origin}/checkout?provider=${encodeURIComponent(method)}`;
      reply = { message: 'success', data: { [field]: checkout } };
      await openWallet(method);
      await Promise.all([
        page.waitForNavigation(),
        page.click(method === 'creem' ? '#creem' : '#pay'),
      ]);
      assert.equal(page.url(), checkout);
      assert.equal(orders.at(-1).endpoint, endpoint);
      assert.equal(visits.at(-1).method, 'GET');
      assert.equal((await browser.pages()).length, initialPages);
      if (method === 'waffo:2') {
        assert.equal(orders.at(-1).payload.pay_method_index, 2);
      }
      console.log(
        `PASS ${mobile ? 'mobile' : 'desktop'} ${method} same-tab GET`,
      );
    }
    const params = {
      amount: '10.00',
      sign: 'mock&+签名',
      submit: 'reserved-field',
      target: '_blank',
      action: 'must-remain-a-field',
    };
    reply = { message: 'success', url: `${origin}/checkout`, data: params };
    await openWallet('alipay');
    await Promise.all([page.waitForNavigation(), page.click('#pay')]);
    assert.equal(orders.at(-1).endpoint, '/api/user/pay');
    assert.equal(visits.at(-1).method, 'POST');
    assert.deepEqual(
      Object.fromEntries(new URLSearchParams(visits.at(-1).body)),
      params,
    );
    assert.equal((await browser.pages()).length, initialPages);
    console.log(
      `PASS ${mobile ? 'mobile' : 'desktop'} gateway same-tab POST, signed fields preserved`,
    );
  }

  for (const [method, response] of [
    ['stripe', { message: 'success', data: {} }],
    ['waffo', { message: 'success' }],
    [
      'creem',
      { message: 'success', data: { checkout_url: 'javascript:alert(1)' } },
    ],
    ['alipay', { message: 'success', url: `${origin}/checkout` }],
    ['alipay', { message: 'success', url: `${origin}/checkout`, data: {} }],
    ['stripe', { message: 'Provider unavailable' }],
  ]) {
    reply = response;
    const count = visits.length;
    await openWallet(method);
    await page.click(method === 'creem' ? '#creem' : '#pay');
    await page.waitForFunction(
      () => window.errors.length === 1 && !window.loading,
    );
    assert.ok(page.url().includes('/wallet?'));
    assert.equal(visits.length, count);
    assert.equal(await page.$('form'), null);
    assert.equal((await browser.pages()).length, initialPages);
  }
  console.log(
    'PASS malformed responses and provider failures stay on wallet and show error',
  );

  await openWallet('stripe');
  const validation = await page.evaluate(() => {
    const invalid = [
      undefined,
      null,
      '',
      ' ',
      '/relative',
      {},
      'javascript:alert(1)',
      'data:text/html,test',
    ];
    const results = invalid.map((url) => [
      window.paymentHelpers.redirectToPayment(url),
      window.paymentHelpers.submitPaymentForm(url, { sign: 'mock' }),
    ]);
    const native = HTMLFormElement.prototype.submit;
    HTMLFormElement.prototype.submit = () => {
      throw new Error('Submission failed');
    };
    let threw = false;
    try {
      window.paymentHelpers.submitPaymentForm(`${location.origin}/checkout`, {
        sign: 'mock',
      });
    } catch {
      threw = true;
    } finally {
      HTMLFormElement.prototype.submit = native;
    }
    return { results, threw, forms: document.querySelectorAll('form').length };
  });
  assert.ok(
    validation.results.every((pair) =>
      pair.every((result) => result === false),
    ),
  );
  assert.equal(validation.threw, true);
  assert.equal(validation.forms, 0);
  assert.deepEqual(errors, []);
  console.log(
    'PASS unsafe URLs rejected and failed POST cleans up temporary form',
  );
} finally {
  try {
    if (browser) await browser.close();
  } finally {
    server.closeAllConnections();
    await new Promise((resolve) => server.close(resolve));
  }
}
console.log('Payment navigation tests passed; test browser and server closed.');
