import assert from 'node:assert/strict';
import test from 'node:test';
import {
  getEffectiveVisibleColumns,
  readUsageLogColumnMode,
  USAGE_LOG_COLUMN_MODE_STORAGE_KEY,
} from '../src/helpers/usageLogView.js';

const COLUMN_KEYS = {
  TIME: 'time',
  MODEL: 'model',
  TYPE: 'type',
  COST: 'cost',
  USE_TIME: 'use_time',
  PROMPT: 'prompt',
  COMPLETION: 'completion',
  TOKEN: 'token',
  GROUP: 'group',
  DETAILS: 'details',
};

const storageWith = (value) => ({
  getItem(key) {
    assert.equal(key, USAGE_LOG_COLUMN_MODE_STORAGE_KEY);
    return value;
  },
});

test('ordinary users default to summary and only full is a stored alternative', () => {
  assert.equal(readUsageLogColumnMode(false, storageWith(null)), 'summary');
  assert.equal(
    readUsageLogColumnMode(false, storageWith('invalid')),
    'summary',
  );
  assert.equal(readUsageLogColumnMode(false, storageWith('full')), 'full');
});

test('storage failures fall back to summary', () => {
  const storage = {
    getItem() {
      throw new Error('blocked');
    },
  };
  assert.equal(readUsageLogColumnMode(false, storage), 'summary');
  assert.equal(readUsageLogColumnMode(false, null), 'summary');
});

test('administrators always keep full view', () => {
  assert.equal(readUsageLogColumnMode(true, storageWith('summary')), 'full');
  const original = { time: true, details: true };
  assert.deepEqual(
    getEffectiveVisibleColumns(original, 'summary', COLUMN_KEYS, true),
    original,
  );
});

test('summary filters only the derived view and never mutates preferences', () => {
  const original = Object.fromEntries(
    Object.values(COLUMN_KEYS).map((key) => [key, true]),
  );
  original.group = false;
  const snapshot = structuredClone(original);
  const effective = getEffectiveVisibleColumns(
    original,
    'summary',
    COLUMN_KEYS,
    false,
  );

  assert.deepEqual(original, snapshot);
  assert.notEqual(effective, original);
  assert.deepEqual(effective, {
    time: true,
    model: true,
    type: true,
    cost: true,
    use_time: true,
    prompt: true,
    completion: true,
    token: false,
    group: false,
    details: false,
  });
});

test('full view restores every saved choice and does not alias state', () => {
  const original = { time: false, details: true, group: false };
  const effective = getEffectiveVisibleColumns(
    original,
    'full',
    COLUMN_KEYS,
    false,
  );
  assert.deepEqual(effective, original);
  assert.notEqual(effective, original);
});

test('invalid preferences produce an empty safe derived view', () => {
  assert.deepEqual(
    getEffectiveVisibleColumns(null, 'summary', COLUMN_KEYS, false),
    {},
  );
});
