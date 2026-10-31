import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { transformSync } from 'esbuild';
import { videoPriceItems } from '../src/helpers/videoPriceDisplay.js';
import { buildH3SecondsExpression } from '../src/helpers/h3VideoPricing.js';

// Exercise the actual table renderer without browser-only icon/lottie imports.
const source = fs.readFileSync(
  new URL(
    '../src/components/table/model-pricing/view/table/PricingTableColumns.jsx',
    import.meta.url,
  ),
  'utf8',
);
const code = transformSync(
  source
    .slice(source.indexOf('export const getPricingTableColumns'))
    .replace('export const', 'const'),
  { loader: 'jsx' },
).code;
const getPricingTableColumns = vm.runInNewContext(
  code + '\ngetPricingTableColumns',
  {
    React,
    getModelCatalogPricing: ({ record, displayPrice }) => ({
      data: { isDynamicPricing: true },
      items: videoPriceItems(record.billing_expr, 0.8, displayPrice),
    }),
  },
);

test('comparison table displays both resolutions in the price column and no token placeholders', () => {
  const model = {
    model_name: 'MiniMax-H3',
    billing_mode: 'tiered_expr',
    enable_groups: ['public'],
    quota_type: 0,
    billing_expr: buildH3SecondsExpression('MiniMax-H3', {
      '768P': 2.5,
      '2K': 4,
    }),
  };
  const columns = getPricingTableColumns({
    t: (s) => s,
    selectedGroup: 'public',
    groupRatio: { public: 0.8 },
    tokenUnit: 'K',
    currency: 'CNY',
    displayPrice: { currencySymbol: '¥', toAmount: (n) => n * 0.2 },
  });
  const price = renderToStaticMarkup(
    columns.find((c) => c.dataIndex === 'input_price').render(null, model),
  );
  for (const text of ['768P', '2K', '¥0.4000', '¥0.6400', ' / 秒'])
    assert.ok(price.includes(text));
  assert.ok(!price.includes('见价格详情'));
  for (const key of ['output_price', 'cache_price'])
    assert.equal(
      columns.find((c) => c.dataIndex === key).render(null, model),
      '—',
    );
});
