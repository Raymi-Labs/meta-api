import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import {
  isVideoConsumption,
  videoLogSegments,
} from '../src/helpers/videoPriceDisplay.js';
import { transformSync } from 'esbuild';

// Execute the real formatter so summary-column changes cannot silently remove
// the only way to inspect a record's content. No API or database is involved.
const source = readFileSync(
  new URL('../src/hooks/usage-logs/useUsageLogsData.jsx', import.meta.url),
  'utf8',
);
const start = source.indexOf('  const setLogsFormat =');
const end = source.indexOf('  // Load logs function', start);
const compiled = transformSync(source.slice(start, end), {
  loader: 'jsx',
}).code;
function format(rows, admin = false) {
  let details;
  const env = {
    t: (value) => value,
    isAdminUser: admin,
    isVideoConsumption,
    videoLogSegments,
    renderQuota: (value) => `Q${value}`,
    billingDisplayMode: 'price',
    requestConversionDisplayValue: () => 'native',
    getResponseModelInfo: () => null,
    timestamp2string: (value) => String(value),
    getLogOther: (value) => JSON.parse(value || '{}'),
    promptTokensExcludeCache: () => false,
    getCacheCreationBreakdown: () => ({ total: 0 }),
    formatTimeRatioLogDetail: () => '',
    setExpandData: (value) => {
      details = value;
    },
    setLogs: () => {},
  };
  new Function(
    'env',
    'rows',
    `with (env) { ${compiled}; setLogsFormat(rows); }`,
  )(env, rows);
  return details;
}
for (const type of [1, 3, 4]) {
  test(`type ${type} retains its only text in expandable details without request ID`, () => {
    const content = '额度变动说明：10 <script>fixture</script> & 原因';
    const result = format([
      { id: type, type, content, other: '{}', created_at: 1 },
    ]);
    assert.deepEqual(result[type], [{ key: '详情', value: content }]);
  });
}
test('Pulse reward content is retained exactly once', () => {
  const result = format([
    {
      id: 1,
      type: 4,
      content: 'reward',
      other: '{"pulse_reward":true}',
      created_at: 1,
    },
  ]);
  assert.deepEqual(result[1], [{ key: '详情', value: 'reward' }]);
});
test('empty non-consumption records do not gain a meaningless expandable item', () => {
  for (const type of [1, 3, 4])
    assert.deepEqual(
      format([{ id: 1, type, content: '', other: '{}', created_at: 1 }])[1],
      [],
    );
});
test('administrator record retains content and existing diagnostic details', () => {
  const result = format(
    [{ id: 1, type: 1, content: 'topup', other: '{}', created_at: 1 }],
    true,
  );
  assert.equal(result[1].filter((item) => item.key === '详情').length, 1);
  assert.ok(result[1].some((item) => item.key === '计费模式'));
});

test('historical charged video omits invented zero price and upstream attribution', () => {
  const result = format(
    [
      {
        id: 99,
        type: 2,
        quota: 120,
        content: 'generate',
        created_at: 1,
        other: JSON.stringify({ request_path: '/v1/videos', model_price: 0 }),
      },
    ],
    true,
  );
  const rendered = JSON.stringify(result[99]);
  assert.ok(rendered.includes('Q120'));
  assert.ok(!rendered.includes('上游返回'));
  assert.ok(!rendered.includes('模型价格'));
});
