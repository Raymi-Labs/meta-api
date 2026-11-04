import assert from 'node:assert/strict';
import test from 'node:test';
import {
  normalizeVMCapacitySettings,
  readChannelCapacityUsage,
} from '../src/helpers/channelCapacity.js';

test('legacy settings remain disabled; VM settings round trip', () => {
  assert.deepEqual(normalizeVMCapacitySettings(), {
    enabled: false,
    overflow_policy: 'wait',
    max_wait_ms: 30000,
  });
  const saved = {
    enabled: true,
    overflow_policy: 'reroute',
    max_wait_ms: 40000,
  };
  assert.deepEqual(
    normalizeVMCapacitySettings(JSON.parse(JSON.stringify(saved))),
    saved,
  );
});

test('dynamic zero is not static inherit; unknown capacity does not become default100', () => {
  assert.equal(
    readChannelCapacityUsage({ max_concurrency: 0 }, 100).maxConcurrency,
    100,
  );
  for (const state of ['known', 'unknown', 'stale', 'busy']) {
    const usage = readChannelCapacityUsage(
      { max_concurrency: 0, inflight: 2, vm_capacity: { state } },
      100,
    );
    assert.equal(usage.maxConcurrency, 0);
    assert.equal(usage.inflight, 2);
  }
});
