import test from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { normalizeUsageLogGroupOptions } from '../src/helpers/usageLogQuery.js';

test('Semi Select searches actual names without matching duplicate descriptions', () => {
  const require = createRequire(import.meta.url);
  const SelectFoundation =
    require('@douyinfe/semi-foundation/lib/cjs/select/foundation.js').default;
  const options = normalizeUsageLogGroupOptions({
    'Claude · 福利': { desc: '自测 福利' },
    'Gemini · 福利': { desc: '自测 福利' },
    'GPT · 鹈鹕王': { desc: '稳定' },
  });
  const filter = (input) =>
    SelectFoundation.prototype._filterOption
      .call(
        { getProp: () => true },
        options.map((option) => ({ ...option })),
        input,
      )
      .filter((option) => option._show)
      .map((option) => option.value);
  assert.deepEqual(filter('claude'), ['Claude · 福利']);
  assert.deepEqual(filter('GEMINI'), ['Gemini · 福利']);
  assert.deepEqual(filter('鹈鹕王'), ['GPT · 鹈鹕王']);
  assert.deepEqual(filter('福利'), ['Claude · 福利', 'Gemini · 福利']);
  assert.deepEqual(filter('不存在'), []);
  assert.equal(filter('').length, 3);
  assert.deepEqual(filter('自测'), []);
});
