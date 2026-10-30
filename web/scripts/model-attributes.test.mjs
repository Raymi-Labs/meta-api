import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  formatModelTokenLimit,
  modelCapabilityStatus,
  modelAttributeSource,
} from '../src/helpers/modelAttributes.js';

test('limits retain 1.05M precision and unknowns are not unsupported', () => {
  assert.equal(formatModelTokenLimit(1050000), '1.05M tokens');
  assert.equal(formatModelTokenLimit(128000), '128K tokens');
  for (const value of [0, -1, Number.MAX_SAFE_INTEGER + 1, 'bad'])
    assert.equal(formatModelTokenLimit(value), '—');
  assert.equal(modelCapabilityStatus(false), '不支持');
  assert.equal(modelCapabilityStatus(undefined), '未知');
  assert.equal(modelCapabilityStatus(true), '支持');
});
test('attribution distinguishes manual values and rejects unsafe links', () => {
  assert.deepEqual(modelAttributeSource({}, 'context_length', true), {
    label: '人工配置',
  });
  assert.deepEqual(modelAttributeSource({}, 'context_length', false), {
    label: '未知',
  });
  assert.deepEqual(
    modelAttributeSource(
      {
        attribute_sources: {
          context_length: { kind: 'official', url: 'javascript:alert(1)' },
        },
      },
      'context_length',
      true,
    ),
    { label: '厂商官方', url: undefined },
  );
  assert.equal(
    modelAttributeSource(
      {
        attribute_sources: {
          context_length: { kind: 'community', url: 'https://example.com' },
        },
      },
      'context_length',
      true,
    ).label,
    '模型库补全',
  );
});
