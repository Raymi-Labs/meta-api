import test from 'node:test';
import assert from 'node:assert/strict';
import { compareGroupPrice } from '../src/helpers/modelGroupDiscount.js';
const input = {
  item: { key: 'input' },
  pricing: { unitPricesUSD: { input: 2 } },
  model: { quota_type: 0, official_pricing: { currency: 'USD' } },
  officialTier: { label: 'base', unit: 'million_tokens', input: 10 },
  currency: 'USD',
  displayPrice: { toAmount: (v) => v },
  usdExchangeRate: 7,
  tokenUnit: 'M',
  quotaDisplayType: 'USD',
};
const compare = (patch) => compareGroupPrice({ ...input, ...patch });
test('uses actual unit prices, lower / equal / higher / zero / unknown', () => {
  assert.deepEqual(compare(), {
    official: 10,
    group: 2,
    kind: 'lower',
    percent: '80',
  });
  assert.equal(
    compare({ pricing: { unitPricesUSD: { input: 10 } } }).kind,
    'equal',
  );
  assert.equal(
    compare({ pricing: { unitPricesUSD: { input: 12 } } }).percent,
    '20',
  );
  assert.equal(
    compare({ pricing: { unitPricesUSD: { input: 12 } } }).kind,
    'higher',
  );
  assert.equal(
    compare({ pricing: { unitPricesUSD: { input: 0 } } }).percent,
    '100',
  );
  assert.equal(
    compare({ officialTier: { unit: 'million_tokens', input: 0 } }).percent,
    null,
  );
  assert.equal(
    compare({
      officialTier: { unit: 'million_tokens', input: 0 },
      pricing: { unitPricesUSD: { input: 0 } },
    }).kind,
    'equal',
  );
  for (const price of [null, undefined, NaN, Infinity, -1, '10'])
    assert.equal(
      compare({ officialTier: { unit: 'million_tokens', input: price } }),
      null,
    );
  for (const price of [null, undefined, NaN, Infinity, -1])
    assert.equal(
      compare({ pricing: { unitPricesUSD: { input: price } } }),
      null,
    );
});
test('K/M and tiny amounts do not round before percentage comparison', () => {
  assert.equal(
    compare({ tokenUnit: 'K', pricing: { unitPricesUSD: { input: 0.002 } } })
      .percent,
    '80',
  );
  assert.equal(
    compare({ pricing: { unitPricesUSD: { input: 1e-9 } } }).percent,
    '99.9',
  );
  assert.equal(
    compare({ pricing: { unitPricesUSD: { input: 9.999 } } }).percent,
    '<0.1',
  );
  assert.equal(
    compare({
      pricing: { unitPricesUSD: { input: 0.1 + 0.2 } },
      officialTier: { unit: 'million_tokens', input: 0.3 },
    }).kind,
    'equal',
  );
});
test('official FX and recharge/package credit conversion are independent', () => {
  const recharge = compare({
    currency: 'CNY',
    displayPrice: { toAmount: (v) => v * 2 },
  });
  assert.equal(recharge.official, 70);
  assert.equal(recharge.group, 4);
  assert.equal(recharge.percent, '94.3');
  const pkg = compare({
    currency: 'CNY',
    displayPrice: { toAmount: (v) => v * 0.5 },
  });
  assert.equal(pkg.official, 70);
  assert.equal(pkg.group, 1);
  assert.equal(pkg.percent, '98.6');
  const cny = compare({
    model: { quota_type: 0, official_pricing: { currency: 'CNY' } },
    officialTier: { unit: 'million_tokens', input: 70 },
  });
  assert.equal(cny.official, 10);
  assert.equal(cny.percent, '80');
  for (const fx of [undefined, 0, NaN, Infinity, -1])
    assert.equal(compare({ currency: 'CNY', usdExchangeRate: fx }), null);
  assert.equal(compare({ currency: 'CUSTOM' }), null);
  assert.equal(
    compare({
      model: { quota_type: 0, official_pricing: { currency: 'EUR' } },
    }),
    null,
  );
});
test('selected official tier changes only reference; units and TTL cannot be guessed', () => {
  assert.equal(
    compare({ officialTier: { unit: 'million_tokens', input: 20 } }).percent,
    '90',
  );
  assert.equal(compare({ officialTier: { unit: 'request', input: 10 } }), null);
  assert.equal(compare({ quotaDisplayType: 'TOKENS' }), null);
  assert.equal(compare({ item: { key: 'time-ratio' } }), null);
  assert.equal(compare({ item: { key: 'create-cache-1h' } }), null);
  const cache = {
    item: { key: 'create-cache' },
    pricing: { unitPricesUSD: { 'create-cache': 2 } },
    officialTier: { unit: 'million_tokens', cache_write: 10 },
  };
  assert.equal(compare(cache).percent, '80');
  assert.equal(
    compare({
      ...cache,
      model: { ...input.model, supports_cache_creation_ttl: true },
    }),
    null,
  );
  assert.equal(
    compare({
      ...cache,
      item: { key: 'tiered-cc' },
      pricing: { isDynamicPricing: true, unitPricesUSD: { 'tiered-cc': 2 } },
      model: {
        ...input.model,
        billing_mode: 'tiered_expr',
        supported_endpoint_types: ['anthropic'],
      },
    }),
    null,
  );
});
test('dynamic first tier and fixed request/hour compare their respective rate only', () => {
  assert.equal(
    compare({
      item: { key: 'tiered-p' },
      model: { ...input.model, billing_mode: 'tiered_expr' },
      pricing: { isDynamicPricing: true, unitPricesUSD: { 'tiered-p': 2 } },
    }).percent,
    '80',
  );
  for (const [quota_type, key, unit] of [
    [1, 'fixed', 'request'],
    [2, 'audio-duration', 'hour'],
  ]) {
    assert.equal(
      compare({
        model: { ...input.model, quota_type },
        item: { key },
        pricing: { unitPricesUSD: { [key]: 2 } },
        officialTier: { unit, price: 10 },
      }).percent,
      '80',
    );
  }
});
