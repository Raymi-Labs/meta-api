import { test } from 'node:test';
import assert from 'node:assert/strict';
import { omitUnchangedModelMetadata } from '../src/helpers/modelMetadataPatch.js';

test('saving unrelated settings does not submit stale automatic metadata', () => {
  const initial = {
    description: 'Official',
    context_length: 1000,
    tags: ['text'],
    capabilities: { reasoning: true },
    vendor_id: 1,
  };
  const values = structuredClone(initial);
  const payload = { ...values, tags: 'text', icon: 'Custom', status: 1 };
  assert.deepEqual(omitUnchangedModelMetadata(payload, values, initial), {
    icon: 'Custom',
    status: 1,
  });
  assert.equal(payload.description, 'Official');
});
test('manual changes, false, and explicit clearing remain in the patch', () => {
  const initial = {
    description: 'Official',
    context_length: 1000,
    capabilities: { reasoning: true },
    tags: ['text'],
    vendor_id: 1,
  };
  const values = {
    description: '',
    context_length: 0,
    capabilities: { reasoning: false },
    tags: [],
    vendor_id: null,
  };
  const payload = { ...values, tags: '', vendor_id: 0 };
  const expected = { ...payload, capability_overrides: { reasoning: false } };
  delete expected.capabilities;
  assert.deepEqual(
    omitUnchangedModelMetadata(payload, values, initial),
    expected,
  );
});

test('changing one capability preserves other background updates and supports unknown', () => {
  const initial = {
    capabilities: { reasoning: true, streaming: true, memory: false },
  };
  const values = { capabilities: { reasoning: true, streaming: false } };
  assert.deepEqual(omitUnchangedModelMetadata({ ...values }, values, initial), {
    capability_overrides: { streaming: false, memory: null },
  });
});

test('changing to a matching rule keeps required exact-only field resets', () => {
  const initial = {
    name_rule: 0,
    release_date: '2026-10-31',
    official_pricing: { currency: 'USD', tiers: [{ input: 1 }] },
  };
  const values = { ...initial, name_rule: 1 };
  const payload = { ...values, release_date: '', official_pricing: {} };
  assert.deepEqual(
    omitUnchangedModelMetadata(payload, values, initial),
    payload,
  );
});
