import { readFile } from 'node:fs/promises';
import assert from 'node:assert/strict';
import {
  getModelSortPrices,
  sortPricingModels,
} from '../src/helpers/modelPriceSorting.js';
import {
  buildUsageLogQuery,
  readLogExportError,
} from '../src/helpers/usageLogExport.js';

const models = [
  {
    model_name: 'expensive',
    quota_type: 0,
    model_ratio: 4,
    completion_ratio: 2,
  },
  { model_name: 'free', quota_type: 0, model_ratio: 0, completion_ratio: 1 },
  { model_name: 'missing', quota_type: 0 },
  { model_name: 'request', quota_type: 1, model_price: 0.2 },
  {
    model_name: 'dynamic',
    quota_type: 0,
    model_ratio: 0,
    billing_mode: 'tiered_expr',
  },
  { model_name: 'cheap', quota_type: 0, model_ratio: 1, completion_ratio: 10 },
];
const prices = (record) => getModelSortPrices(record, 0.5);
const names = (order) =>
  sortPricingModels(models, order, prices).map((model) => model.model_name);
assert.deepEqual(names('input-asc'), [
  'free',
  'cheap',
  'expensive',
  'missing',
  'request',
  'dynamic',
]);
assert.deepEqual(names('input-desc'), [
  'expensive',
  'cheap',
  'free',
  'missing',
  'request',
  'dynamic',
]);
assert.deepEqual(names('output-desc').slice(0, 3), [
  'cheap',
  'expensive',
  'free',
]);
assert.equal(names('request-desc')[0], 'request');
assert.equal(sortPricingModels(models, 'default', prices), models);
assert.equal(models[0].model_name, 'expensive');
assert.equal(getModelSortPrices(models[0], 0).input, 0);
assert.equal(
  getModelSortPrices({ quota_type: 0, model_ratio: '' }, 1).input,
  null,
);
assert.equal(
  getModelSortPrices({ quota_type: 1, model_price: -1 }, 1).request,
  null,
);
assert.equal(getModelSortPrices(models[0], Infinity).input, null);

const values = {
  logType: 2,
  token_name: 'a&b',
  model_name: 'gpt%',
  startTimestamp: 10,
  endTimestamp: 20,
  group: '企业',
  request_id: 'req',
  username: '1',
  channel_ids: '2,3',
  channel_keyword: 'team',
  group_vendor: 'OpenAI',
};
const self = buildUsageLogQuery(values, false);
assert.equal(self.request_id, 'req');
assert.equal(self.channel_ids, undefined);
assert.equal(self.username, undefined);
assert.equal(buildUsageLogQuery(values, true).channel_ids, '2,3');
assert.equal(
  await readLogExportError(
    { response: { data: new Blob(['{"message":"limit"}']) } },
    'fallback',
  ),
  'limit',
);
assert.equal(
  await readLogExportError(
    { response: { data: new Blob(['bad']) } },
    'fallback',
  ),
  'fallback',
);
console.log('Pricing sorting and export query tests passed');

// Exercise the same group/time resolution used by cards, tables and sorting.
const utils = await readFile(
  new URL('../src/helpers/utils.jsx', import.meta.url),
  'utf8',
);
const normalize = utils.slice(
  utils.indexOf('export const normalizePricingTimeRatioInfo'),
  utils.indexOf('const appendTimeRatioItem'),
);
const calculate = utils.slice(
  utils.indexOf('export const calculateModelPrice'),
  utils.indexOf('export const getModelPriceItems'),
);
const moduleSource = `import { getModelSortPrices } from '${new URL('../src/helpers/modelPriceSorting.js', import.meta.url).href}';\n${normalize}\n${calculate}`;
const { calculateModelPrice } = await import(
  `data:text/javascript;base64,${Buffer.from(moduleSource).toString('base64')}`
);
const record = {
  model_name: 'grouped',
  quota_type: 0,
  model_ratio: 2,
  completion_ratio: 3,
  enable_groups: ['cheap', 'expensive'],
};
const options = {
  record,
  groupRatio: { cheap: 1, expensive: 2 },
  timeRatioMap: { grouped: { cheap: { ratio: 4 }, expensive: { ratio: 0.5 } } },
  tokenUnit: 'M',
  currency: 'USD',
  displayPrice: (value) => `$${value}`,
};
const all = calculateModelPrice({ ...options, selectedGroup: 'all' });
assert.equal(all.usedGroup, 'expensive');
assert.equal(all.sortPrices.input, 4);
assert.equal(
  calculateModelPrice({ ...options, selectedGroup: 'cheap' }).sortPrices.input,
  16,
);
assert.equal(
  calculateModelPrice({ ...options, selectedGroup: 'cheap', tokenUnit: 'K' })
    .sortPrices.input,
  16,
);
console.log('Shared group and time pricing calculations passed');
