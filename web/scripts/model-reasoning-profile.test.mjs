import test from 'node:test';
import assert from 'node:assert/strict';
import { omitUnchangedModelMetadata } from '../src/helpers/modelMetadataPatch.js';

test('exact model declarations change only when the administrator edits them', () => {
  const initial = {
    name_rule: 0,
    reasoning_profile: {
      supportedLevels: ['low', 'ultra'],
      defaultLevel: 'low',
    },
  };
  const patch = (next, original) =>
    omitUnchangedModelMetadata(next, next, original);
  assert.deepEqual(patch(initial).reasoning_profile, initial.reasoning_profile);
  assert.ok(!('reasoning_profile' in patch(initial, structuredClone(initial))));
  assert.equal(
    patch({ ...initial, reasoning_profile: { supportedLevels: [] } }, initial)
      .reasoning_profile,
    null,
  );
  assert.equal(
    patch({ ...initial, name_rule: 1 }, initial).reasoning_profile,
    null,
  );
  assert.ok(!('reasoning_profile' in patch({ name_rule: 0 })));
});
