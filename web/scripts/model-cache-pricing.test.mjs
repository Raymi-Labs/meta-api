import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import vm from 'node:vm';
import { test } from 'node:test';
import { videoPriceItems } from '../src/helpers/videoPriceDisplay.js';
import { buildH3SecondsExpression } from '../src/helpers/h3VideoPricing.js';
import {
  BILLING_PRICING_VARS,
  BILLING_VAR_REGEX,
  BILLING_VAR_KEY_TO_FIELD,
} from '../src/constants/billing.constants.js';
import { getModelSortPrices } from '../src/helpers/modelPriceSorting.js';
import { buildFirstTierGroupPriceItems } from '../src/helpers/tieredBillingDisplay.js';
import { splitBillingExprAndRequestRules } from '../src/pages/Setting/Ratio/components/requestRuleExpr.js';
import { isVideoPriceExpression } from '../src/helpers/videoPricingTemplate.js';

import {
  parseImagePriceExpression,
  isImagePriceExpression,
} from '../src/helpers/imagePricing.js';
import { formatModelPricingUnitPrice } from '../src/helpers/modelPricingCurrency.js';

// Execute actual pricing code without unrelated browser UI, i18n and toast
// dependencies. AST extraction fails if an expected declaration disappears.
const { parse } = createRequire(import.meta.url)('@babel/parser');
const extract = (relativePath, names) => {
  const source = readFileSync(new URL(relativePath, import.meta.url), 'utf8');
  const body = parse(source, { sourceType: 'module', plugins: ['jsx'] }).program
    .body;
  const selected = [];
  const found = new Set();
  for (const node of body) {
    const declaration =
      node.type === 'ExportNamedDeclaration' ? node.declaration : node;
    if (!declaration || declaration.type === 'ImportDeclaration') continue;
    const name =
      declaration.id?.name || declaration.declarations?.[0]?.id?.name;
    if (names && !names.includes(name)) continue;
    selected.push(source.slice(declaration.start, declaration.end));
    found.add(name);
  }
  for (const name of names || []) assert.ok(found.has(name), `Missing ${name}`);
  return selected.join('\n');
};
const source = [
  extract('../src/helpers/utils.jsx', [
    'normalizePricingTimeRatioInfo',
    'formatTimeRatioValue',
    'shouldShowPricingTimeRatio',
    'getTimeRatioInfoFromMap',
    'appendTimeRatioItem',
    'calculateModelPrice',
    'getModelPriceItems',
  ]),
  extract('../src/helpers/render.jsx', [
    'stripExprVersion',
    'parseTierBody',
    'parseTiersFromExpr',
  ]),
  extract('../src/helpers/modelCatalogPricing.js'),
  '({ getModelCatalogPricing, isCachePriceItem })',
].join('\n');
const { getModelCatalogPricing, isCachePriceItem } = vm.runInNewContext(
  source,
  {
    getModelSortPrices,
    videoPriceItems,
    BILLING_PRICING_VARS,
    BILLING_VAR_REGEX,
    BILLING_VAR_KEY_TO_FIELD,
    buildFirstTierGroupPriceItems,
    splitBillingExprAndRequestRules,
    isVideoPriceExpression,
    parseImagePriceExpression,
    isImagePriceExpression,
    formatModelPricingUnitPrice,
  },
);
const t = (key) => key;
const money = (symbol = '$', rate = 1) =>
  Object.assign((value) => `${symbol}${value * rate}`, {
    currencySymbol: symbol,
    toAmount: (value) => value * rate,
  });
const base = {
  model_name: 'fixture-text',
  quota_type: 0,
  model_ratio: 2,
  completion_ratio: 5,
  supports_cache_read: true,
  cache_ratio: 0.1,
  supports_cache_creation: true,
  cache_creation_ratio: 1.25,
  supports_cache_creation_ttl: true,
  cache_creation_ratio_1h: 2,
  enable_groups: ['standard'],
};
const pricing = (record = {}, options = {}) =>
  getModelCatalogPricing(
    {
      record: { ...base, ...record },
      groupRatio: { standard: 1 },
      selectedGroup: 'standard',
      tokenUnit: 'M',
      currency: 'USD',
      displayPrice: money(),
      ...options,
    },
    t,
  );
const item = (result, key) => result.items.find((entry) => entry.key === key);
const cacheKeys = (result) =>
  Array.from(result.items.filter(isCachePriceItem), (entry) => entry.key);

test('ordinary pricing reads API cache fields and preserves 5m/1h labels', () => {
  const result = pricing();
  assert.equal(item(result, 'input').value, '$4.0000');
  assert.equal(item(result, 'completion').value, '$20.0000');
  assert.equal(item(result, 'cache').value, '$0.4000');
  assert.equal(item(result, 'create-cache').value, '$5.0000');
  assert.equal(item(result, 'create-cache-1h').value, '$8.0000');
  assert.match(item(result, 'create-cache').label, /5/);
  assert.match(item(result, 'create-cache-1h').label, /1/);
  assert.equal(item(result, 'cache').suffix, ' / 1M Tokens');
  assert.equal(
    item(pricing({ create_cache_ratio: 999 }), 'create-cache').value,
    '$5.0000',
  );
});

test('unconfigured API zero values are absent, not advertised as free', () => {
  for (const flag of [false, undefined, null, 'true', 1]) {
    assert.deepEqual(
      cacheKeys(
        pricing({
          supports_cache_read: flag,
          cache_ratio: 0,
          supports_cache_creation: flag,
          cache_creation_ratio: 0,
          cache_creation_ratio_1h: 0,
        }),
      ),
      [],
    );
  }
});

test('explicitly supported zero cache prices and ratios stay visible', () => {
  const record = {
    cache_ratio: 0,
    cache_creation_ratio: 0,
    cache_creation_ratio_1h: 0,
  };
  for (const key of ['cache', 'create-cache', 'create-cache-1h'])
    assert.equal(item(pricing(record), key).value, '$0.0000');
  const ratios = pricing(record, { quotaDisplayType: 'TOKENS' });
  for (const key of [
    'cache-ratio',
    'create-cache-ratio',
    'create-cache-1h-ratio',
  ]) {
    assert.equal(item(ratios, key).value, 0);
    assert.equal(item(ratios, key).suffix, 'x');
  }
});

test('missing, invalid and negative cache ratios never become prices', () => {
  for (const invalid of [undefined, null, '', 'invalid', NaN, Infinity, -1]) {
    const record = {
      cache_ratio: invalid,
      cache_creation_ratio: invalid,
      cache_creation_ratio_1h: invalid,
    };
    assert.deepEqual(cacheKeys(pricing(record)), [], String(invalid));
    assert.deepEqual(
      cacheKeys(pricing(record, { quotaDisplayType: 'TOKENS' })),
      [],
      String(invalid),
    );
  }
  assert.equal(item(pricing({ cache_ratio: '0.1' }), 'cache').value, '$0.4000');
});

test('TTL is explicit; generic creation does not invent a one-hour price', () => {
  for (const flag of [false, undefined, null, 'true', 1]) {
    const result = pricing({ supports_cache_creation_ttl: flag });
    assert.equal(item(result, 'create-cache').value, '$5.0000');
    assert.equal(item(result, 'create-cache').label, '缓存创建价格');
    assert.equal(item(result, 'create-cache-1h'), undefined);
    assert.equal(
      item(
        pricing(
          { supports_cache_creation_ttl: flag },
          { quotaDisplayType: 'TOKENS' },
        ),
        'create-cache-1h-ratio',
      ),
      undefined,
    );
  }
  assert.deepEqual(cacheKeys(pricing({ supports_cache_creation: false })), [
    'cache',
  ]);
});

test('cache prices use the same group, time multiplier, currency and unit as input', () => {
  const options = {
    selectedGroup: 'all',
    groupRatio: { cheap: 1, discount: 2 },
    timeRatioMap: {
      'fixture-text': { cheap: { ratio: 4 }, discount: { ratio: 0.5 } },
    },
    currency: 'CNY',
    displayPrice: money('¥', 0.2),
  };
  const record = { enable_groups: ['cheap', 'discount'] };
  const result = pricing(record, options);
  assert.equal(result.data.usedGroup, 'discount');
  assert.equal(item(result, 'input').value, '¥0.8000');
  assert.equal(item(result, 'cache').value, '¥0.0800');
  assert.equal(item(result, 'create-cache').value, '¥1.0000');
  assert.equal(item(result, 'create-cache-1h').value, '¥1.6000');
  const perK = pricing(record, { ...options, tokenUnit: 'K' });
  assert.equal(item(perK, 'cache').value, '¥0.000080');
  assert.equal(item(perK, 'create-cache').value, '¥0.001000');
  assert.equal(item(perK, 'create-cache-1h').value, '¥0.001600');
  assert.equal(item(perK, 'cache').suffix, ' / 1K Tokens');
  const selected = pricing(record, { ...options, selectedGroup: 'cheap' });
  assert.equal(item(selected, 'cache').value, '¥0.3200');
  assert.equal(item(selected, 'create-cache').value, '¥4.0000');
});

test('ratio display uses configured ratios independently of currency conversion', () => {
  const result = pricing({}, { quotaDisplayType: 'TOKENS' });
  assert.equal(item(result, 'cache-ratio').value, 0.1);
  assert.equal(item(result, 'create-cache-ratio').value, 1.25);
  assert.equal(item(result, 'create-cache-1h-ratio').value, 2);
  assert.deepEqual(cacheKeys(result), [
    'cache-ratio',
    'create-cache-ratio',
    'create-cache-1h-ratio',
  ]);
});

test('dynamic cache prices come from first tier even with ordinary flags false', () => {
  const result = pricing({
    supports_cache_read: false,
    supports_cache_creation: false,
    billing_mode: 'tiered_expr',
    billing_expr:
      'len <= 10000 ? tier("base", p*4+c*20+cr*.4+cc*5+cc1h*8) : tier("long", p*8+c*40+cr*.8+cc*10+cc1h*16)',
  });
  assert.equal(result.data.isDynamicPricing, true);
  assert.equal(item(result, 'tiered-p').value, '$4.0000');
  assert.equal(item(result, 'tiered-cr').value, '$0.4000');
  assert.equal(item(result, 'tiered-cc').value, '$5.0000');
  assert.equal(item(result, 'tiered-cc1h').value, '$8.0000');
  assert.deepEqual(cacheKeys(result), [
    'tiered-cr',
    'tiered-cc',
    'tiered-cc1h',
  ]);
  assert.deepEqual(
    Array.from(
      result.items.filter((entry) => !isCachePriceItem(entry)),
      (entry) => entry.key,
    ),
    ['tiered-p', 'tiered-c'],
  );
});

test('per-request models never receive token cache prices', () => {
  const result = pricing({ quota_type: 1, model_price: 0.09 });
  assert.deepEqual(cacheKeys(result), []);
  assert.equal(result.items.length, 1);
  assert.equal(item(result, 'fixed').value, '$0.09');
  assert.equal(item(result, 'fixed').suffix, ' / 次');
});

test('shared cache classification excludes other metrics and handles absent items', () => {
  for (const key of [
    'cache',
    'cache-ratio',
    'create-cache',
    'create-cache-ratio',
    'create-cache-1h',
    'create-cache-1h-ratio',
    'tiered-cr',
    'tiered-cc',
    'tiered-cc1h',
  ])
    assert.equal(isCachePriceItem({ key }), true, key);
  for (const key of [
    'input',
    'completion',
    'tiered-p',
    'tiered-img',
    'time-ratio',
    'fixed',
  ])
    assert.equal(isCachePriceItem({ key }), false, key);
  assert.equal(isCachePriceItem(undefined), false);
  assert.equal(isCachePriceItem({}), false);
});

test('unrounded comparison amounts share the actual pricing calculation', () => {
  const result = pricing(
    {},
    {
      groupRatio: { standard: 0.4 },
      timeRatioMap: { 'fixture-text': { standard: { ratio: 0.5 } } },
    },
  );
  assert.equal(result.data.unitPricesUSD.input, 0.8);
  assert.equal(result.data.unitPricesUSD.completion, 4);
  assert.equal(result.data.unitPricesUSD.cache, 0.08000000000000002);
  const perK = pricing({}, { tokenUnit: 'K' });
  assert.equal(perK.data.unitPricesUSD.input, 0.004);
  assert.equal(perK.data.unitPricesUSD['create-cache'], 0.005);
  const dynamic = pricing(
    {
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("base", p*4+c*20+cr*.4)',
    },
    { groupRatio: { standard: 0.5 } },
  );
  assert.equal(dynamic.data.firstTierLabel, 'base');
  assert.equal(dynamic.data.unitPricesUSD['tiered-p'], 2);
  assert.equal(dynamic.data.unitPricesUSD['tiered-cr'], 0.2);
  assert.equal(
    pricing({ quota_type: 1, model_price: 0.09 }).data.unitPricesUSD.fixed,
    0.09,
  );
  assert.equal(
    pricing({ quota_type: 2, audio_duration_price: 3 }).data.unitPricesUSD[
      'audio-duration'
    ],
    3,
  );
});

test('video resolution prices share actual recharge/group/time conversion across M and K', () => {
  const record = {
    model_name: 'MiniMax-H3',
    billing_mode: 'tiered_expr',
    billing_expr: buildH3SecondsExpression('MiniMax-H3', {
      '768P': 2.5,
      '2K': 4,
    }),
    enable_groups: ['standard', 'discount'],
  };
  for (const tokenUnit of ['M', 'K']) {
    const result = pricing(record, {
      tokenUnit,
      selectedGroup: 'all',
      groupRatio: { standard: 1, discount: 0.8 },
      currency: 'CNY',
      displayPrice: money('¥', 0.2),
    });
    assert.deepEqual(
      Array.from(result.items, (i) => [i.label, i.value, i.suffix]),
      [
        ['768P', '¥0.4000', ' / 秒'],
        ['2K', '¥0.6400', ' / 秒'],
      ],
    );
    assert.equal(result.data.usedGroup, 'discount');
    assert.equal(result.data.videoPricing, true);
    assert.equal(result.data.unitPricesUSD['video-resolution-1'], 3.2);
  }
  const timed = pricing(record, {
    groupRatio: { standard: 0.8 },
    timeRatioMap: { 'MiniMax-H3': { standard: { ratio: 1.5, matched: true } } },
    displayPrice: money('¥', 0.2),
  });
  assert.equal(timed.items[0].value, '¥0.6000');
  const free = pricing(record, {
    groupRatio: { standard: 0 },
    displayPrice: money('¥', 0.2),
  });
  assert.equal(free.items[0].value, '¥0.0000');
  const base = pricing(record, { displayPrice: money('⚡', 1) });
  assert.equal(base.items[0].value, '⚡2.5000');
  const subscription = pricing(record, { displayPrice: money('¥', 0.1) });
  assert.equal(subscription.items[0].value, '¥0.2500');
  const dynamic = pricing({
    ...record,
    billing_expr: '(' + record.billing_expr + ') * 2',
  });
  assert.equal(dynamic.items[0].value, '动态计费');
  assert.ok(!dynamic.data.unitPricesUSD);
});
