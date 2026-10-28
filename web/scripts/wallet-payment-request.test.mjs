import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
const source = readFileSync(
  new URL('../src/components/topup/index.jsx', import.meta.url),
  'utf8',
);
const handler = source.slice(
  source.indexOf('  const onlineTopUp ='),
  source.indexOf('  const onlineCreemTopUp ='),
);
function harness() {
  let release;
  const calls = [],
    redirects = [];
  const env = {
    aliveRef: { current: true },
    paymentBusyRef: { current: false },
    paymentSessionRef: { current: 1 },
    confirmedPayment: { count: 10, way: 'stripe', quote: { amount: 2 } },
    open: true,
    topUpCount: 10,
    payWay: 'stripe',
    setConfirmLoading: () => {},
    setOpen: (v) => {
      env.open = v;
    },
    t: (s) => s,
    showError: () => {},
    redirectToPayment: (url) => {
      redirects.push(url);
      return true;
    },
    submitPaymentForm: () => true,
    API: {
      post: (url, payload) => {
        calls.push({ url, payload });
        return new Promise((resolve) => {
          release = resolve;
        });
      },
    },
  };
  const run = new Function('env', `with(env){${handler};return onlineTopUp;}`)(
    env,
  );
  return {
    env,
    run,
    calls,
    redirects,
    finish: () =>
      release({
        data: {
          message: 'success',
          data: { pay_link: 'https://example.invalid/fixture' },
        },
      }),
  };
}
test('payment double click creates one order for the confirmed amount and method', async () => {
  const h = harness();
  const first = h.run();
  await h.run();
  assert.equal(h.calls.length, 1);
  assert.deepEqual(h.calls[0].payload, {
    amount: 10,
    payment_method: 'stripe',
  });
  h.finish();
  await first;
  assert.equal(h.redirects.length, 1);
});
test('cancelled payment response cannot navigate or close a later confirmation', async () => {
  const h = harness();
  const first = h.run();
  h.env.paymentSessionRef.current++;
  h.env.open = true;
  h.finish();
  await first;
  assert.equal(h.redirects.length, 0);
  assert.equal(h.env.open, true);
});
test('changing amount or payment method invalidates the old confirmation', async () => {
  for (const change of [{ topUpCount: 30 }, { payWay: 'alipay' }]) {
    const h = harness();
    Object.assign(h.env, change);
    await h.run();
    assert.equal(h.calls.length, 0);
  }
});
test('old account response cannot navigate the new account', async () => {
  const h = harness();
  const first = h.run();
  h.env.aliveRef.current = false;
  h.finish();
  await first;
  assert.equal(h.redirects.length, 0);
});
