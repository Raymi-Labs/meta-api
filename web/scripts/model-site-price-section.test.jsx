import assert from 'node:assert/strict';
import { test } from 'bun:test';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import ModelOfficialPricing from '../src/pages/ModelDetail/ModelOfficialPricing';
test('missing official reference renders no placeholder while explicit official zero remains', () => {
  const t = (s) => s;
  for (const pricing of [undefined, {}, { currency: 'USD', tiers: [] }])
    assert.equal(
      renderToStaticMarkup(<ModelOfficialPricing pricing={pricing} t={t} />),
      '',
    );
  const html = renderToStaticMarkup(
    <ModelOfficialPricing
      pricing={{
        currency: 'USD',
        tiers: [
          { label: 'Standard', unit: 'million_tokens', input: 0, output: 0 },
        ],
      }}
      t={t}
    />,
  );
  assert.ok(html.includes('$0'));
});
