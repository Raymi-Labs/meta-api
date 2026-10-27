import assert from 'node:assert/strict';
import test from 'node:test';
import { getModelHealthView } from '../src/helpers/modelHealthView.js';
const user = { id: 7 };
const names = '["atlas"]';
const ready = {
  user,
  names,
  attempt: 0,
  state: 'ready',
  models: [{ model: 'atlas', avg_ttft_ms: 0, avg_output_tps: 42 }],
};
const view = (overrides = {}) =>
  getModelHealthView({
    user,
    names,
    enabled: true,
    attempt: 0,
    result: ready,
    ...overrides,
  });
test('logout and identity replacement never expose earlier performance results', () => {
  assert.equal(view({ user: null }).state, 'anonymous');
  assert.equal(view({ user: null }).summaries.size, 0);
  for (const next of [{ id: 8 }, { id: 7 }]) {
    assert.equal(view({ user: next }).state, 'loading');
    assert.equal(view({ user: next }).summaries.size, 0);
  }
});
test('page changes and retries hide stale responses until their own result arrives', () => {
  for (const change of [{ names: '["other"]' }, { attempt: 1 }]) {
    assert.equal(view(change).state, 'loading');
    assert.equal(view(change).summaries.size, 0);
  }
  assert.equal(view({ enabled: false }).state, 'idle');
  assert.equal(view({ enabled: false }).summaries.size, 0);
});
test('loading, failure, empty success and explicit zero measurements remain distinct', () => {
  assert.equal(view({ result: null }).state, 'loading');
  assert.equal(
    view({ result: { user, names, attempt: 0, state: 'error' } }).state,
    'error',
  );
  const empty = view({ result: { ...ready, models: [] } });
  assert.equal(empty.state, 'ready');
  assert.equal(empty.summaries.size, 0);
  assert.equal(view().summaries.get('atlas').avg_ttft_ms, 0);
});
