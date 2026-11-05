import assert from 'node:assert/strict';
import test from 'node:test';
import {
  OFFICIAL_PRICE_UNITS,
  formatOfficialPrice,
  officialPriceColumns,
} from '../src/helpers/modelOfficialPricing.js';
test('official prices preserve missing, zero and tiny prices without billing conversion', () => {
  assert.equal(formatOfficialPrice(null, 'USD'), '—');
  assert.equal(formatOfficialPrice(0, 'USD'), '$0');
  assert.equal(formatOfficialPrice(10, 'USD'), '$10');
  assert.equal(formatOfficialPrice(0.000000001, 'USD'), '$0.000000001');
  assert.equal(formatOfficialPrice(2, 'CNY'), '¥2');
  assert.deepEqual(
    officialPriceColumns([
      { input: 10, output: 50, cache_read: 0, cache_write: null },
    ]).map(([key]) => key),
    ['input', 'output', 'cache_read'],
  );
});
test('modality and character rates retain their units', () => {
  assert.equal(OFFICIAL_PRICE_UNITS.million_tokens, 'M tokens');
  assert.equal(OFFICIAL_PRICE_UNITS.million_characters, 'M characters');
  for (const mode of ['audio', 'image', 'video'])
    assert.equal(
      OFFICIAL_PRICE_UNITS[`million_${mode}_tokens`],
      `M ${mode} tokens`,
    );
});
