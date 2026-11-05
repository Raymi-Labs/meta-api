import assert from 'node:assert/strict';
import test from 'node:test';
import {
  getWalletCostReference,
  isStripeBelowMinimum,
  walletQuoteEndpoint,
} from '../src/helpers/walletPaymentCosts.js';

test('only fresh comparable USD reference is displayed; zero is a valid difference', () => {
  const now = Date.now();
  const ref = {
    method: 'stripe',
    baseline_method: 'alipay',
    sample_count: 3,
    sample_amount: 100,
    reference_usd: 3,
    reference_paid: 20.81,
    baseline_amount: 20,
    difference_percent: 4.05,
    observed_at: now / 1000 - 60,
    expires_at: now / 1000 + 60,
  };
  assert.equal(getWalletCostReference(ref, { currency: 'USD' }, now), ref);
  for (const patch of [
    { sample_count: 2 },
    { expires_at: now / 1000 - 1 },
    { observed_at: now / 1000 + 1 },
    { reference_paid: NaN },
    { sample_amount: 0 },
    { expires_at: undefined },
    { difference_percent: Infinity },
  ]) {
    assert.equal(
      getWalletCostReference({ ...ref, ...patch }, { currency: 'USD' }, now),
      null,
    );
  }
  assert.equal(getWalletCostReference(ref, { currency: 'EUR' }, now), null);
  assert.equal(getWalletCostReference(ref, undefined, now), null);
  assert.equal(
    getWalletCostReference(
      { ...ref, difference_percent: 0 },
      { currency: 'USD' },
      now,
    ).difference_percent,
    0,
  );
});

test('minimum uses actual quote currency and shared providers keep quote routes', () => {
  assert(isStripeBelowMinimum('stripe', { currency: 'USD', amount: 0.49 }));
  assert(!isStripeBelowMinimum('stripe', { currency: 'USD', amount: 0.5 }));
  assert(!isStripeBelowMinimum('stripe', { currency: 'CNY', amount: 0.49 }));
  assert(!isStripeBelowMinimum('alipay', { currency: 'USD', amount: 0.49 }));
  assert.equal(walletQuoteEndpoint('stripe'), '/api/user/stripe/amount');
  assert.equal(walletQuoteEndpoint('waffo:1'), '/api/user/waffo/amount');
  assert.equal(walletQuoteEndpoint('usdt.trc20'), '/api/user/amount');
});
