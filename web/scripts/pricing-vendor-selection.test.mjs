import assert from 'node:assert/strict';
import {
  groupForPricingVendor,
  sortPricingVendors,
} from '../src/helpers/pricingVendorSelection.js';
import { sortPricingModels } from '../src/helpers/modelPriceSorting.js';

const models = [
  {
    vendor_name: 'OpenAI',
    enable_groups: ['gpt', 'shared', 'zero', 'forbidden'],
  },
  { vendor_name: 'DeepSeek', enable_groups: ['ds', 'shared', 'zero'] },
  { enable_groups: ['unknown'] },
];
const usableGroup = { gpt: '', ds: '', shared: '', zero: '', unknown: '' };
const resolve = (group, vendor) =>
  groupForPricingVendor({ models, usableGroup, group, vendor });
assert.equal(resolve('gpt', 'DeepSeek'), 'all');
assert.equal(resolve('shared', 'DeepSeek'), 'shared');
assert.equal(resolve('zero', 'DeepSeek'), 'zero');
assert.equal(resolve('gpt', 'all'), 'gpt');
assert.equal(resolve('gpt', 'unknown'), 'all');
assert.equal(resolve('unknown', 'unknown'), 'unknown');
assert.equal(resolve('forbidden', 'OpenAI'), 'all');
assert.equal(resolve('all', 'Missing'), 'all');
assert.deepEqual(
  sortPricingVendors([
    'xAI',
    'Moonshot',
    'Google',
    'DeepSeek',
    'OpenAI',
    'Anthropic',
    'Inception',
  ]),
  ['Anthropic', 'OpenAI', 'DeepSeek', 'Google', 'Moonshot', 'xAI', 'Inception'],
);
assert.deepEqual(sortPricingVendors(['Grok', 'Kimi', 'Gemini', 'Claude']), [
  'Claude',
  'Gemini',
  'Kimi',
  'Grok',
]);
const input = [
  'gpt-5.9',
  'gpt-6',
  'gpt-5.10',
  'gpt-6.1',
  'gpt-6.1-20261020',
  'gpt-6.1-20261021',
].map((model_name) => ({ model_name }));
const before = [...input];
assert.deepEqual(
  sortPricingModels(input, 'version-desc').map((m) => m.model_name),
  [
    'gpt-6.1-20261021',
    'gpt-6.1-20261020',
    'gpt-6.1',
    'gpt-6',
    'gpt-5.10',
    'gpt-5.9',
  ],
);
assert.deepEqual(input, before);
const equal = [
  { model_name: 'MODEL-2', id: 1 },
  { model_name: 'model-2', id: 2 },
];
assert.deepEqual(sortPricingModels(equal, 'version-desc'), equal);
const families = [
  'gpt-image-2',
  'gpt-5',
  'gpt-6',
  'gpt-image-2.5',
  'custom',
].map((model_name) => ({ model_name }));
assert.deepEqual(
  sortPricingModels(families, 'version-desc').map((m) => m.model_name),
  ['custom', 'gpt-6', 'gpt-5', 'gpt-image-2.5', 'gpt-image-2'],
);
console.log(
  'Vendor selection, permissions, stable vendor order and natural model versions passed.',
);
