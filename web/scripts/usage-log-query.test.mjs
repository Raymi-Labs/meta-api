import test from 'node:test';
import assert from 'node:assert/strict';
import { normalizeUsageLogFilters } from '../src/helpers/usageLogQuery.js';
import { buildUsageLogQuery } from '../src/helpers/usageLogExport.js';

test('applied filters retain channels and timestamps after the form draft changes', () => {
  const draft = {
    dateRange: [
      new Date('2026-10-28T00:00:00Z'),
      new Date('2026-10-28T01:00:00Z'),
    ],
    channel_ids: [2, 3],
    logType: '5',
    model_name: 'model+a/b',
    username: 'name:123',
    group: 'vip',
    request_id: 'request-fixture',
  };
  const applied = normalizeUsageLogFilters(draft);
  draft.channel_ids.push(4);
  draft.model_name = 'other';
  draft.dateRange[0].setHours(12);
  const query = buildUsageLogQuery(applied, true);
  assert.equal(query.channel_ids, '2,3');
  assert.equal(query.model_name, 'model+a/b');
  assert.equal(query.end_timestamp - query.start_timestamp, 3600);
  assert.equal(query.type, 5);
  assert.equal(query.username, 'name:123');
  assert.equal(query.request_id, 'request-fixture');
});
test('self queries cannot include administrator filter fields', () => {
  const query = buildUsageLogQuery(
    normalizeUsageLogFilters({
      dateRange: ['2026-10-28 00:00:00', '2026-10-28 01:00:00'],
      username: 'other',
      channel: '4',
      channel_ids: [5],
      channel_keyword: 'secret',
      group_vendor: 'vendor',
      group: 'allowed',
    }),
    false,
  );
  for (const key of [
    'username',
    'channel',
    'channel_ids',
    'channel_keyword',
    'group_vendor',
  ])
    assert.equal(Object.hasOwn(query, key), false);
  assert.equal(query.group, 'allowed');
});
test('cleared or invalid dates remain invalid instead of silently widening the query', () => {
  for (const dateRange of [null, [], ['bad', 'bad'], ['', ''], [null, null]]) {
    const result = normalizeUsageLogFilters({ dateRange });
    assert.ok(Number.isNaN(result.startTimestamp));
    assert.ok(Number.isNaN(result.endTimestamp));
  }
});
test('channel usage end minus one stays an inclusive second, with no rounding expansion', () => {
  const result = normalizeUsageLogFilters({
    dateRange: [new Date(1793145600 * 1000), new Date((1793149200 - 1) * 1000)],
  });
  assert.equal(result.startTimestamp, 1793145600);
  assert.equal(result.endTimestamp, 1793149199);
});
