import assert from 'node:assert/strict';
import {
  getSelectableButtonItems,
  isSelectableItemDisabled,
} from '../src/components/common/ui/selectableButtonItems.js';

const items = Object.freeze([
  Object.freeze({ value: 'all', tagCount: 0, disabled: true }),
  Object.freeze({ value: 'shared', tagCount: 'x0', disabled: false }),
  Object.freeze({ value: 'other', tagCount: 'x1', disabled: true }),
  Object.freeze({ value: 0, tagCount: 0 }),
  Object.freeze({ value: 'available', tagCount: 2 }),
]);
const values = (result) => result.map((item) => item.value);

// Existing consumers keep the original disabled and zero-count behavior.
assert.equal(getSelectableButtonItems(items, 'all'), items);
assert.equal(isSelectableItemDisabled(items[0]), true);
assert.equal(isSelectableItemDisabled(items[1]), false);
assert.equal(isSelectableItemDisabled(items[2]), true);
assert.equal(isSelectableItemDisabled(items[3]), true);
assert.deepEqual(values(getSelectableButtonItems(items, 'all', true, 'all')), [
  'all',
  'shared',
  'available',
]);
// A selected zero-result option remains visible; numeric quota type 0 is valid.
assert.deepEqual(values(getSelectableButtonItems(items, 0, true, 'all')), [
  'all',
  'shared',
  0,
  'available',
]);
assert.deepEqual(
  values(getSelectableButtonItems(items, ['other', 0], true, 'all')),
  ['all', 'shared', 'other', 0, 'available'],
);
assert.deepEqual(getSelectableButtonItems([], 'all', true, 'all'), []);
assert.deepEqual(
  values(getSelectableButtonItems(items.slice(0, 1), 'missing', true, 'all')),
  ['all'],
);
console.log(
  'Pricing filter visibility: passed (legacy, zero results, selection, reset, ratio, numeric values).',
);
