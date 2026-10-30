import assert from 'node:assert/strict';
import { test } from 'node:test';
import { requestModelCapabilitySync } from '../src/helpers/modelCapabilitySync.js';

test('sync uses the administrator capability endpoint and reports changed models', async () => {
  const calls = [];
  const result = await requestModelCapabilitySync({
    post: async (...args) => {
      calls.push(args);
      return {
        data: {
          success: true,
          data: { matched: 8, created: 2, updated: 3, skipped: 3 },
        },
      };
    },
  });
  assert.deepEqual(calls, [['/api/models/sync_capabilities']]);
  assert.deepEqual(result, { created: 2, updated: 3 });
});

test('business failure remains a failure with its server message', async () => {
  const response = {
    data: { success: false, message: 'Sync already running' },
  };
  await assert.rejects(
    requestModelCapabilitySync({ post: async () => response }),
    (error) => error.response === response,
  );
});

test('network failure is propagated and malformed responses cannot report success', async () => {
  const unavailable = new Error('Network unavailable');
  await assert.rejects(
    requestModelCapabilitySync({
      post: async () => {
        throw unavailable;
      },
    }),
    (error) => error === unavailable,
  );
  await assert.rejects(
    requestModelCapabilitySync({ post: async () => ({ data: {} }) }),
  );
  assert.deepEqual(
    await requestModelCapabilitySync({
      post: async () => ({
        data: { success: true, data: { created: -1, updated: '9' } },
      }),
    }),
    { created: 0, updated: 0 },
  );
});

test('partial source failures remain visible alongside applied counts', async () => {
  const result = await requestModelCapabilitySync({
    post: async () => ({
      data: {
        success: true,
        data: { created: 1, updated: 2, source_failures: 3 },
      },
    }),
  });
  assert.deepEqual(result, { created: 1, updated: 2, sourceFailures: 3 });
});
