import assert from 'node:assert/strict';
import test from 'node:test';
import {
  modelReleaseDateSortKey,
  pricingSortOptions,
  sortPricingModels,
} from '../src/helpers/modelPriceSorting.js';
import { sanitizeModelCatalogSnapshot } from '../src/helpers/modelDetailNavigation.js';

test('release date parsing preserves month precision and rejects malformed dates', () => {
  assert.equal(modelReleaseDateSortKey('2026-10'), 20261001);
  assert.equal(modelReleaseDateSortKey('2026-10-26'), 20261026);
  assert.equal(modelReleaseDateSortKey('2024-02-29'), 20240229);
  assert.equal(modelReleaseDateSortKey('2000-02-29'), 20000229);
  for (const value of [
    undefined,
    null,
    '',
    20261026,
    '2026-10-26T00:00:00Z',
    '2026-1-01',
    '2026-00',
    '2026-13',
    '2026-02-29',
    '2100-02-29',
    '2026-04-31',
    '2026-10-00',
    '2026-10-32',
    '1969-12-31',
    ' 2026-10-26 ',
  ])
    assert.equal(modelReleaseDateSortKey(value), null, String(value));
});

test('latest releases cross vendor and version boundaries, unknown dates stay last', () => {
  const models = [
    {
      model_name: 'claude-99',
      release_date: '2025-02-01',
      updated_time: 9999999999,
    },
    { model_name: 'gpt-2', release_date: '2026-10-26' },
    { model_name: 'qwen-4', release_date: '2026-10' },
    { model_name: 'kimi-3', release_date: '2026-10-25' },
    { model_name: 'unknown-2', created_time: 9999999999 },
    { model_name: 'bad-date', release_date: '2026-02-30' },
    { model_name: 'unknown-10', release_date: '' },
  ];
  const original = structuredClone(models);
  const names = (input) =>
    sortPricingModels(input, 'release-desc').map((m) => m.model_name);
  const expected = [
    'gpt-2',
    'kimi-3',
    'qwen-4',
    'claude-99',
    'bad-date',
    'unknown-2',
    'unknown-10',
  ];
  assert.deepEqual(names(models), expected);
  assert.deepEqual(names([...models].reverse()), expected);
  assert.deepEqual(models, original);
  assert.deepEqual(names(models).slice(0, 2), ['gpt-2', 'kimi-3']);
});

test('equal dates have deterministic model-name ordering independent of API order', () => {
  const models = ['model-10', 'model-2', 'MODEL-2'].map((model_name) => ({
    model_name,
    release_date: '2026-10-01',
  }));
  const sorted = sortPricingModels(models, 'release-desc');
  assert.deepEqual(
    sorted.map((m) => m.model_name),
    ['MODEL-2', 'model-2', 'model-10'],
  );
  assert.deepEqual(
    sortPricingModels([...models].reverse(), 'release-desc'),
    sorted,
  );
});

test('new visits default to release date; explicit history and price choices remain', () => {
  assert.equal(sanitizeModelCatalogSnapshot().sortOrder, 'release-desc');
  assert.equal(
    sanitizeModelCatalogSnapshot({ sortOrder: 'invalid' }).sortOrder,
    'release-desc',
  );
  for (const sortOrder of [
    'release-desc',
    'version-desc',
    'default',
    'input-asc',
    'output-desc',
  ]) {
    assert.equal(
      sanitizeModelCatalogSnapshot({ sortOrder }).sortOrder,
      sortOrder,
    );
  }
  const options = pricingSortOptions((key) => key);
  assert.deepEqual(options[0], { value: 'release-desc', label: '最新发布' });
  assert.equal(
    options.find((item) => item.value === 'default').label,
    '原始顺序',
  );
});
