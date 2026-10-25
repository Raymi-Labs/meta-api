import assert from 'node:assert/strict';
import test from 'node:test';
import {
  readReasoningAdaptation,
  buildReasoningAdaptation,
} from '../src/helpers/channelReasoningAdaptation.js';

test('old settings inherit; saved exact rules round trip without loss', () => {
  assert.equal(
    readReasoningAdaptation('{}').reasoning_adaptation_mode,
    'inherit',
  );
  const rule = {
    model: 'glm-5.3-flash',
    protocol: 'chat',
    mapping: { max: 'max', high: 'high' },
  };
  const saved = buildReasoningAdaptation('auto', JSON.stringify([rule]));
  const loaded = readReasoningAdaptation(
    JSON.stringify({ proxy: 'unchanged', reasoning_adaptation: saved }),
  );
  assert.equal(loaded.reasoning_adaptation_mode, 'auto');
  assert.deepEqual(
    buildReasoningAdaptation(
      loaded.reasoning_adaptation_mode,
      loaded.reasoning_adaptation_rules,
    ),
    saved,
  );
});
test('reject malformed, wildcard, duplicate, unsupported protocol and effort rules', () => {
  const rule = { model: 'glm-5.3', protocol: 'chat', mapping: { high: 'max' } };
  for (const raw of [
    '{}',
    'null',
    'bad',
    JSON.stringify([rule, rule]),
    JSON.stringify([{ ...rule, model: 'glm-*' }]),
    JSON.stringify([{ ...rule, protocol: 'responses' }]),
    JSON.stringify([{ ...rule, mapping: { high: 'unknown' } }]),
  ]) {
    assert.throws(() => buildReasoningAdaptation('auto', raw));
  }
  assert.throws(() => buildReasoningAdaptation('invalid', '[]'));
});
