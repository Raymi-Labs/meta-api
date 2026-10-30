import assert from 'node:assert/strict';
import { test } from 'bun:test';
import {
  getCompareModels,
  buildModelComparePath,
  buildModelPlaygroundPath,
  getCompareGroups,
  estimateModelMonthlyCost,
} from '../src/helpers/modelCompare';
const model = {
  model_name: 'vendor/a+b%2F',
  model_ratio: 2,
  completion_ratio: 3,
  quota_type: 0,
  enable_groups: ['Public', 'Free'],
  supports_cache_read: true,
  cache_ratio: 0,
};
const data = {
  usableGroup: { Public: 'public', Free: 'free', Other: 'other' },
  groupRatio: { Public: 0.5, Free: 0, Other: 10 },
  timeRatioMap: {},
};

test('comparison and playground links preserve exact model IDs and bound selections', () => {
  const names = ['vendor/a+b%2F', '中文 &?', 'c', 'd', 'extra', 'c'];
  const path = buildModelComparePath(names);
  assert.deepEqual(getCompareModels(path.split('?')[1]), names.slice(0, 4));
  const pg = new URL(
    buildModelPlaygroundPath(names[0], '优惠 +'),
    'https://local.test',
  );
  assert.equal(pg.searchParams.get('model'), names[0]);
  assert.equal(pg.searchParams.get('group'), '优惠 +');
  assert.deepEqual(getCompareModels('?model=&model=' + 'x'.repeat(256)), []);
});

test('common groups are preferred and auto/unavailable groups excluded', () => {
  assert.deepEqual(
    getCompareGroups([model, { enable_groups: ['Free'] }], {
      ...data.usableGroup,
      auto: 'auto',
    }),
    ['Free', 'Public'],
  );
});

test('monthly estimates distinguish ordinary tokens, zero, unknown and dynamic billing', () => {
  const price = { unitPricesUSD: { input: 2, completion: 6 } };
  const workload = { requests: 1000, input: 2000, output: 500 };
  assert.equal(estimateModelMonthlyCost(model, price, workload), 210);
  assert.equal(
    estimateModelMonthlyCost(model, price, { ...workload, requests: 0 }),
    0,
  );
  assert.equal(estimateModelMonthlyCost(model, null, workload), null);
  assert.equal(
    estimateModelMonthlyCost(model, price, { ...workload, input: '' }),
    null,
  );
  assert.equal(
    estimateModelMonthlyCost(model, price, { ...workload, input: Infinity }),
    null,
  );
  assert.equal(
    estimateModelMonthlyCost({ ...model, quota_type: 1 }, price, workload),
    null,
  );
  assert.equal(
    estimateModelMonthlyCost(
      model,
      { ...price, isDynamicPricing: true },
      workload,
    ),
    null,
  );
});
