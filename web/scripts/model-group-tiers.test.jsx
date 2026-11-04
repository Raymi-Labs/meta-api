import assert from 'node:assert/strict';
import { test } from 'bun:test';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { visibleOfficialTiers } from '../src/helpers/modelOfficialPricing';
import { groupTierRows } from '../src/helpers/modelGroupTiers';
import ModelGroupTierTable from '../src/pages/ModelDetail/ModelGroupTierTable';
const t = (text) => text;
const original = {
  currency: 'USD',
  tiers: ['30m', '12h'].flatMap((ttl) => [
    {
      label: `Standard · ≤ 272K input tokens · cache ${ttl}`,
      unit: 'million_tokens',
      input: 1,
      output: 5,
      cache_write: ttl === '30m' ? 1.25 : 2.5,
    },
    {
      label: `Standard · > 272K input tokens · cache ${ttl}`,
      unit: 'million_tokens',
      input: 2,
      output: 7.5,
      cache_write: ttl === '30m' ? 2.5 : 5,
    },
  ]),
};
const model = {
  quota_type: 0,
  billing_mode: 'tiered_expr',
  official_pricing: original,
};
const tiers = [
  { label: 'standard', conditions: [{ var: 'len', op: '<=', value: 272000 }] },
  { label: 'long', conditions: [] },
];
const getPricing = (i) => ({
  data: { isDynamicPricing: true, unitPricesUSD: { 'tiered-p': i ? 1 : 0.5 } },
  items: [
    {
      key: 'tiered-p',
      label: '输入',
      value: i ? '$1' : '$0.5',
      suffix: ' / 1M Tokens',
    },
  ],
});
test('short retention projection handles persisted metadata without mutation', () => {
  const before = JSON.stringify(original);
  const rows = visibleOfficialTiers(original);
  assert.deepEqual(
    rows.map((row) => row.cache_write),
    [1.25, 2.5],
  );
  assert.ok(rows.every((row) => !/30m|12h/.test(row.label)));
  assert.equal(JSON.stringify(original), before);
  assert.equal(
    visibleOfficialTiers({ tiers: [{ label: 'cache 1h' }] }).length,
    1,
  );
});
test('actual site boundaries match independently of official row order and render all amounts', () => {
  const reversed = {
    ...model,
    official_pricing: { ...original, tiers: [...original.tiers].reverse() },
  };
  const rows = groupTierRows(reversed, tiers, getPricing, t);
  assert.deepEqual(
    rows.map((row) => row.officialTier.input),
    [1, 2],
  );
  assert.deepEqual(
    rows.map((row) => row.data.unitPricesUSD['tiered-p']),
    [0.5, 1],
  );
  const html = renderToStaticMarkup(
    <ModelGroupTierTable
      rows={rows}
      model={model}
      data={{
        t,
        currency: 'USD',
        tokenUnit: 'M',
        siteDisplayType: 'USD',
        displayPrice: { toAmount: (value) => value },
      }}
    />,
  );
  assert.equal((html.match(/scope="row"/g) || []).length, 2);
  assert.ok(html.includes('<strong>$0.5</strong>'));
  assert.ok(html.includes('<strong>$1</strong>'));
  assert.equal((html.match(/<del /g) || []).length, 2);
});
test('unmatched boundaries and ambiguous references do not produce discounts', () => {
  for (const condition of [
    { var: 'len', op: '<', value: 272000 },
    { var: 'p', op: '<=', value: 272000 },
    { var: 'len', op: '<=', value: 128000 },
  ]) {
    assert.ok(
      groupTierRows(
        model,
        [{ ...tiers[0], conditions: [condition] }, tiers[1]],
        getPricing,
        t,
      ).every((row) => !row.officialTier),
    );
  }
  const duplicate = {
    ...model,
    official_pricing: {
      ...original,
      tiers: [...original.tiers, ...original.tiers],
    },
  };
  assert.ok(
    groupTierRows(duplicate, tiers, getPricing, t).every(
      (row) => !row.officialTier,
    ),
  );
  assert.equal(
    groupTierRows(model, [tiers[0], tiers[0]], getPricing, t)[1].officialTier,
    undefined,
  );
});
test('flat pricing compares every official tier and missing metadata retains site tiers', () => {
  const flat = groupTierRows(
    model,
    [{ label: 'flat', conditions: [] }],
    getPricing,
    t,
  );
  assert.equal(flat.length, 2);
  assert.deepEqual(
    flat.map((row) => row.data.unitPricesUSD['tiered-p']),
    [0.5, 0.5],
  );
  const missing = groupTierRows(
    { ...model, official_pricing: {} },
    tiers,
    getPricing,
    t,
  );
  assert.equal(missing.length, 2);
  assert.ok(missing.every((row) => !row.officialTier));
});

test('region-specific official labels do not masquerade as unconditional standard input ranges', () => {
  const regional = {
    ...model,
    official_pricing: {
      ...original,
      tiers: original.tiers.map((tier) => ({
        ...tier,
        label: tier.label.replace('Standard', 'EU'),
      })),
    },
  };
  assert.ok(
    groupTierRows(regional, tiers, getPricing, t).every(
      (row) => !row.officialTier,
    ),
  );
});

test('Claude short creation tier merges TTL rows, preserves conditions and renders discounts', () => {
  const claude = {
    quota_type: 0,
    supports_cache_creation_ttl: true,
    official_pricing: {
      currency: 'USD',
      tiers: ['1h', '5m'].flatMap((ttl) => [
        {
          label: `Standard · ≤ 200K input tokens · cache ${ttl}`,
          unit: 'million_tokens',
          input: 2,
          cache_write: ttl === '5m' ? 2.5 : 4,
        },
        {
          label: `Standard · > 200K input tokens · cache ${ttl}`,
          unit: 'million_tokens',
          input: 4,
          cache_write: ttl === '5m' ? 5 : 8,
        },
      ]),
    },
  };
  const rows = groupTierRows(
    claude,
    [],
    () => ({
      data: { unitPricesUSD: { 'create-cache': 0.5 } },
      items: [
        { key: 'create-cache', label: '缓存创建价格（5分钟）', value: '$0.5' },
      ],
    }),
    t,
  );
  assert.equal(rows.length, 2);
  assert.deepEqual(
    rows.map((row) => row.officialTier.cache_write),
    [2.5, 5],
  );
  const html = renderToStaticMarkup(
    <ModelGroupTierTable
      rows={rows}
      model={claude}
      data={{
        t: (text, values) => text.replace('{{percent}}', values?.percent ?? ''),
        currency: 'USD',
        tokenUnit: 'M',
        displayPrice: { toAmount: (v) => v },
      }}
    />,
  );
  assert.ok(html.includes('较官方低 80%'));
  assert.ok(html.includes('较官方低 90%'));
  assert.ok(!html.includes('暂不比较'));
  assert.ok(!html.includes('cache 1h'));
});

test('shortest retention is per region, unit and condition, independent of price and order', () => {
  const tiers = [
    { label: 'US · cache 1h', unit: 'million_tokens', cache_write: 1 },
    { label: 'US · cache 5m', unit: 'million_tokens', cache_write: 3 },
    { label: 'EU · cache 1h', unit: 'million_tokens', cache_write: 4 },
    { label: 'US · cache 1h', unit: 'request', price: 5 },
    { label: 'Unknown cache TTL', unit: 'million_tokens', cache_write: 6 },
    { label: 'Standard', unit: 'million_tokens', cache_write: 0 },
  ];
  assert.deepEqual(
    visibleOfficialTiers({ tiers }).map((row) => [
      row.label,
      row.cache_write ?? row.price,
    ]),
    [
      ['US', 3],
      ['EU', 4],
      ['US', 5],
      ['Unknown cache TTL', 6],
      ['Standard', 0],
    ],
  );
});
