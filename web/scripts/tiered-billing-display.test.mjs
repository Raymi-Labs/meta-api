import assert from 'node:assert/strict';
import {
  buildFirstTierGroupPriceItems,
  buildTieredBillingBreakdown,
  resolveTieredLogParams,
} from '../src/helpers/tieredBillingDisplay.js';

const tier = {
  inputPrice: 1,
  outputPrice: 6,
  cacheReadPrice: 0.1,
  cacheCreatePrice: 1.25,
  cacheCreate1hPrice: 2,
};

const exactParams = resolveTieredLogParams(
  {
    tiered_params: {
      p: 1000,
      c: 200,
      len: 3000,
      cr: 1792,
      cc: 8,
      cc1h: 4,
    },
  },
  tier,
);
assert.deepEqual(exactParams, {
  p: 1000,
  c: 200,
  len: 3000,
  cr: 1792,
  cc: 8,
  cc1h: 4,
  img: 0,
  img_o: 0,
  ai: 0,
  ao: 0,
});

const exactBreakdown = buildTieredBillingBreakdown(tier, exactParams);
assert.equal(exactBreakdown.items.length, 5);
assert.equal(Number(exactBreakdown.subtotalUSD.toFixed(7)), 0.0023972);

const legacyParams = resolveTieredLogParams(
  {
    prompt_tokens: 3000,
    completion_tokens: 200,
    cache_tokens: 1792,
    cache_creation_tokens: 12,
    cache_creation_tokens_5m: 8,
    cache_creation_tokens_1h: 4,
  },
  tier,
);
assert.equal(legacyParams.p, 1196);
assert.equal(legacyParams.c, 200);
assert.equal(legacyParams.cr, 1792);
assert.equal(legacyParams.cc, 8);
assert.equal(legacyParams.cc1h, 4);
assert.equal(legacyParams.len, 3000);

const claudeParams = resolveTieredLogParams(
  {
    claude: true,
    prompt_tokens: 1200,
    completion_tokens: 100,
    cache_tokens: 800,
    cache_creation_tokens_5m: 20,
    cache_creation_tokens_1h: 10,
  },
  tier,
);
assert.equal(claudeParams.p, 1200);
assert.equal(claudeParams.len, 2030);

const displayPrice = () => '';
displayPrice.toAmount = (usdAmount) => usdAmount;
displayPrice.currencySymbol = '¥';

const firstTierPricing = buildFirstTierGroupPriceItems({
  tiers: [
    {
      label: 'standard',
      inputPrice: 2.5,
      outputPrice: 15,
      cacheReadPrice: 0.25,
    },
    {
      label: 'long_context',
      inputPrice: 5,
      outputPrice: 22.5,
      cacheReadPrice: 0.5,
    },
  ],
  effectiveBillingRatio: 0.45,
  displayPrice,
});
assert.equal(firstTierPricing.firstTier.label, 'standard');
assert.deepEqual(firstTierPricing.items, [
  {
    key: 'tiered-p',
    label: '输入价格',
    value: '¥1.1250',
    suffix: ' / 1M Tokens',
  },
  {
    key: 'tiered-c',
    label: '补全价格',
    value: '¥6.7500',
    suffix: ' / 1M Tokens',
  },
  {
    key: 'tiered-cr',
    label: '缓存读取价格',
    value: '¥0.1125',
    suffix: ' / 1M Tokens',
  },
]);

const freeGroupPricing = buildFirstTierGroupPriceItems({
  tiers: [{ label: 'base', inputPrice: 2.5 }],
  effectiveBillingRatio: 0,
  displayPrice,
});
assert.equal(freeGroupPricing.items[0].value, '¥0.0000');

assert.deepEqual(
  buildFirstTierGroupPriceItems({
    tiers: [],
    effectiveBillingRatio: 1,
    displayPrice,
  }),
  { firstTier: null, items: [] },
);

console.log('tiered billing display tests passed');

// Catalog K/M switching changes the reference price and its unit together.
const thousandTokenPricing = buildFirstTierGroupPriceItems({
  tiers: [tier],
  effectiveBillingRatio: 0.5,
  displayPrice,
  tokenUnit: 'K',
  precision: 6,
});
assert.equal(thousandTokenPricing.items[0].value, '¥0.000500');
assert.equal(thousandTokenPricing.items[1].value, '¥0.003000');
assert.equal(thousandTokenPricing.items[2].value, '¥0.000050');
assert.ok(
  thousandTokenPricing.items.every((item) => item.suffix === ' / 1K Tokens'),
);
const freeTierPricing = buildFirstTierGroupPriceItems({
  tiers: [tier],
  effectiveBillingRatio: 0,
  displayPrice,
  tokenUnit: 'K',
});
assert.ok(freeTierPricing.items.every((item) => item.value === '¥0.0000'));
assert.deepEqual(
  buildFirstTierGroupPriceItems({ tiers: [], displayPrice, tokenUnit: 'K' })
    .items,
  [],
);
