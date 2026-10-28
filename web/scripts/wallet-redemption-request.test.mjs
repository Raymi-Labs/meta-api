import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
// Execute the actual async handler with a synthetic server and state setters.
const source = readFileSync(
  new URL(
    '../src/components/topup/modals/WalletRedemptionModal.jsx',
    import.meta.url,
  ),
  'utf8',
);
const handler = source.slice(
  source.indexOf('  const createRedemption ='),
  source.indexOf('  const copyCode ='),
);
function harness(post) {
  const calls = [],
    errors = [],
    balances = [];
  const env = {
    amount: 30,
    minimumQuota: 10,
    walletQuota: 100,
    transferableQuota: 30,
    isAdmin: false,
    pendingRequestRef: { current: null },
    createBusyRef: { current: false },
    aliveRef: { current: true },
    listRequestRef: { current: 0 },
    displayAmountToQuota: (v) => v,
    newRequestID: () => `fixture-${calls.length + 1}`,
    t: (s) => s,
    renderQuota: (v) => String(v),
    API: {
      post: async (url, payload) => {
        calls.push({ ...payload });
        return post(payload);
      },
    },
    setCreating: () => {},
    setUncertainRequest: () => {},
    setCreatedCode: (v) => {
      env.code = v;
    },
    setAmount: (v) => {
      env.amount = v;
    },
    setLoadingList: () => {},
    setListError: () => {},
    setRedemptions: () => {},
    onQuotaChanged: (...values) => balances.push(values),
    loadUsageSummary: async () => {},
    showSuccess: () => {},
    showError: (v) => errors.push(v),
  };
  const run = new Function(
    'env',
    `with(env){${handler};return createRedemption;}`,
  )(env);
  return { env, run, calls, errors, balances };
}
const success = {
  data: {
    success: true,
    data: {
      redemption: { id: 1, key: 'fixture-code' },
      remaining_quota: 70,
      remaining_transferable_quota: 0,
    },
  },
};
test('uncertain retry uses original quota and request ID after remaining balance and input shrink', async () => {
  let fail = true;
  const h = harness(async () => {
    if (fail) throw Error('lost response');
    return success;
  });
  await h.run();
  h.env.amount = 0;
  h.env.walletQuota = 0;
  h.env.transferableQuota = 0;
  fail = false;
  await h.run();
  assert.equal(h.calls.length, 2);
  assert.deepEqual(h.calls[0], h.calls[1]);
  assert.equal(h.calls[1].quota, 30);
  assert.equal(h.env.code, 'fixture-code');
  assert.deepEqual(h.balances, [[70, 0]]);
});
test('rapid duplicate clicks send only one request while pending', async () => {
  let release;
  const h = harness(
    () =>
      new Promise((resolve) => {
        release = resolve;
      }),
  );
  const first = h.run();
  await h.run();
  assert.equal(h.calls.length, 1);
  release(success);
  await first;
  assert.equal(h.balances.length, 1);
});
test('new creation validates transferable quota before sending', async () => {
  const h = harness(async () => success);
  h.env.amount = 40;
  await h.run();
  assert.equal(h.calls.length, 0);
  assert.match(h.errors[0], /可创建兑换码额度不足/);
});
test('unmounted account cannot receive old request balance or code', async () => {
  let release;
  const h = harness(
    () =>
      new Promise((resolve) => {
        release = resolve;
      }),
  );
  const first = h.run();
  h.env.aliveRef.current = false;
  release(success);
  await first;
  assert.equal(h.balances.length, 0);
  assert.equal(h.env.code, undefined);
});
