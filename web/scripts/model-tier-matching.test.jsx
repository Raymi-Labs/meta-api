import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import { test } from 'bun:test';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import fixtures from './fixtures/official-tier-matching.json';
import {
  BILLING_VAR_REGEX,
  BILLING_VAR_KEY_TO_FIELD,
} from '../src/constants/billing.constants';
import {
  groupTierRows,
  groupComparisonUnavailable,
} from '../src/helpers/modelGroupTiers';
import { groupPriceComparison } from '../src/helpers/modelGroupDiscount';
import ModelGroupTierTable from '../src/pages/ModelDetail/ModelGroupTierTable';

// Exercise the real expression parser without importing its browser-only UI dependencies.
const source = fs.readFileSync(
  new URL('../src/helpers/render.jsx', import.meta.url),
  'utf8',
);
const parser = source
  .slice(
    source.indexOf('export function stripExprVersion'),
    source.indexOf('export function renderTieredModelPrice'),
  )
  .replaceAll('export function', 'function');
const parse = vm.runInNewContext(parser + ';parseTiersFromExpr', {
  BILLING_VAR_REGEX,
  BILLING_VAR_KEY_TO_FIELD,
});
const t = (text) => text;
const ds = fixtures.find((m) => m.model_name === 'deepseek-flash');
const getPricing = () => ({
  data: { unitPricesUSD: { 'tiered-p': 10 } },
  items: [
    { key: 'tiered-p', label: '输入', value: '$10', suffix: ' / 1M Tokens' },
  ],
});
const rows = (model) =>
  groupTierRows(model, parse(model.billing_expr), getPricing, t);

test('public snapshot: peak/off-peak and localized Standard length tiers match by meaning, not position', () => {
  for (const [name, expected] of [
    ['deepseek-flash', [0.3, 0.15]],
    ['gemini-2.5-pro', [1.25, 2.5]],
    ['MiniMax-M3', [0.3, 0.6]],
  ]) {
    const model = fixtures.find((m) => m.model_name === name);
    for (const tiers of [
      model.official_pricing.tiers,
      [...model.official_pricing.tiers].reverse(),
    ]) {
      const result = rows({
        ...model,
        official_pricing: { ...model.official_pricing, tiers },
      });
      assert.deepEqual(
        result.map((r) => r.officialTier?.input),
        expected,
        name,
      );
    }
  }
  const result = rows(ds);
  assert.deepEqual(
    result.map((r) => r.label),
    ['峰时', '谷时'],
  );
  assert.ok(
    result.every((r) => r.comparisonNote.includes('生效时段以各自规则为准')),
  );
});

test('period rates retain input/output/cache and group/currency/K/M calculations', () => {
  const options = {
    model: ds,
    quotaDisplayType: 'USD',
    exchangeQuote: { usd_cny: 7, date: new Date().toISOString().slice(0, 10) },
  };
  for (const [index, row] of rows(ds).entries()) {
    for (const [key, field, rate] of [
      ['tiered-p', 'input', 10],
      ['tiered-c', 'output', 40],
      ['tiered-cr', 'cache_read', 0.2],
    ]) {
      for (const groupRatio of [0, 0.2, 1])
        for (const tokenUnit of ['K', 'M'])
          for (const currency of ['USD', 'CNY']) {
            const divisor = tokenUnit === 'K' ? 1000 : 1;
            const amount = ((rate / (index ? 2 : 1)) * groupRatio) / divisor;
            const { comparison, reason } = groupPriceComparison({
              ...options,
              officialTier: row.officialTier,
              item: { key },
              pricing: { unitPricesUSD: { [key]: amount } },
              tokenUnit,
              currency,
              displayPrice: {
                toAmount: (v) => v * (currency === 'CNY' ? 0.2 : 1),
              },
            });
            assert.equal(reason, undefined);
            assert.equal(
              comparison.group,
              amount * (currency === 'CNY' ? 0.2 : 1),
            );
            assert.equal(
              comparison.official,
              (row.officialTier[field] * (currency === 'CNY' ? 7 : 1)) /
                divisor,
            );
          }
    }
  }
});

test('ambiguous period names and extra region/service conditions remain visibly unmatched', () => {
  const variants = [
    {
      ...ds,
      official_pricing: {
        ...ds.official_pricing,
        tiers: [...ds.official_pricing.tiers, ...ds.official_pricing.tiers],
      },
    },
    ...['US · ', 'Priority · '].map((prefix) => ({
      ...ds,
      official_pricing: {
        ...ds.official_pricing,
        tiers: ds.official_pricing.tiers.map((tier) => ({
          ...tier,
          label: prefix + tier.label,
        })),
      },
    })),
    { ...ds, billing_expr: ds.billing_expr.replace('谷时', '峰时') },
    { ...ds, billing_expr: ds.billing_expr.replace('峰时', '未知') },
  ];
  for (const model of variants) {
    const result = rows(model);
    assert.ok(result.every((r) => !r.officialTier && r.comparisonUnavailable));
    const html = renderToStaticMarkup(
      <ModelGroupTierTable rows={result} model={model} data={{ t }} />,
    );
    assert.ok(html.includes('尚未匹配到对应的官方档位'));
    assert.ok(html.includes('<strong>$10</strong>'));
    assert.ok(!html.includes('model-group-discount'));
  }
  assert.equal(groupComparisonUnavailable({}), '暂无官方参考价');
  assert.equal(
    groupComparisonUnavailable({
      quota_type: 1,
      official_pricing: { tiers: [{ unit: 'image' }] },
    }),
    '官方与本站的计价单位不同',
  );
});

test('unavailable comparison explains missing price, TTL, FX, and view without inventing a discount', () => {
  const base = {
    model: ds,
    item: { key: 'tiered-p' },
    pricing: { unitPricesUSD: { 'tiered-p': 10 } },
    officialTier: ds.official_pricing.tiers[1],
    currency: 'USD',
    tokenUnit: 'M',
    displayPrice: { toAmount: (v) => v },
  };
  for (const [patch, expected] of [
    [{ officialTier: { unit: 'million_tokens' } }, '官方未提供该项单价'],
    [
      { currency: 'CNY', exchangeQuote: null },
      '暂无有效汇率，暂不显示官方对比',
    ],
    [{ currency: 'EUR' }, '当前币种无法换算官方价格'],
    [{ quotaDisplayType: 'TOKENS' }, '倍率视图不比较金额'],
    [
      {
        item: { key: 'tiered-cc' },
        model: { ...ds, supports_cache_creation_ttl: true },
      },
      '缓存时长未对应，暂不比较',
    ],
  ]) {
    assert.deepEqual(groupPriceComparison({ ...base, ...patch }), {
      comparison: null,
      reason: expected,
    });
  }
});
