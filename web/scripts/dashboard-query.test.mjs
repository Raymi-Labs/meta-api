import assert from 'node:assert/strict';
import test from 'node:test';
import {
  isDashboardAdmin,
  dashboardRates,
  dashboardQuotaRows,
  validateDashboardQuery,
} from '../src/helpers/dashboardQuery.js';
const range = {
  start_timestamp: '2026-10-01T00:00:00Z',
  end_timestamp: '2026-10-02T00:00:00Z',
};
test('rejects blank, invalid, reversed and zero ranges before a request', () => {
  for (const end of [
    '',
    'invalid',
    range.start_timestamp,
    '2026-09-30T00:00:00Z',
  ])
    assert.equal(
      validateDashboardQuery({ ...range, end_timestamp: end }, 'hour', false),
      'invalid-range',
    );
  assert.equal(
    validateDashboardQuery(range, 'invalid', false),
    'invalid-grain',
  );
  assert.equal(validateDashboardQuery(range, 'hour', false), null);
  assert.equal(
    validateDashboardQuery(
      { ...range, end_timestamp: '2026-11-01T00:00:00Z' },
      'day',
      false,
    ),
    'range-too-long',
  );
  assert.equal(
    validateDashboardQuery(
      { ...range, end_timestamp: '2026-11-01T00:00:00Z' },
      'day',
      true,
    ),
    null,
  );
  assert.equal(
    validateDashboardQuery(
      { ...range, end_timestamp: '2026-10-31T00:00:00Z' },
      'day',
      false,
    ),
    null,
  );
});
test('rates use the applied interval; invalid intervals cannot display Infinity or false zero', () => {
  assert.equal(dashboardRates(1440, 2880, range).avgRPM, '1.000');
  assert.equal(dashboardRates(1440, 2880, range).avgTPM, '2.000');
  assert.equal(dashboardRates(0, 0, range).avgRPM, '0.000');
  assert.equal(
    dashboardRates(1, 1, { ...range, end_timestamp: range.start_timestamp })
      .avgRPM,
    '—',
  );
  assert.equal(
    dashboardRates(1, 1, { ...range, end_timestamp: 'invalid' }).avgTPM,
    '—',
  );
});
test('empty success yields the chart placeholder without mutating API arrays; bad envelopes fail', () => {
  const empty = Object.freeze([]);
  assert.equal(dashboardQuotaRows(empty, 1000)[0].is_empty_placeholder, true);
  assert.equal(empty.length, 0);
  const rows = Object.freeze([
    { created_at: 2, count: 2 },
    { created_at: 1, count: 1 },
  ]);
  assert.equal(dashboardQuotaRows(rows)[0].created_at, 1);
  assert.equal(rows[0].created_at, 2);
  assert.throws(() => dashboardQuotaRows(null));
  assert.throws(() => dashboardQuotaRows({}));
});

test('overview roles retain admin 10 and root 100 while ordinary users stay self-only', () => {
  assert.equal(isDashboardAdmin({ role: 1 }), false);
  assert.equal(isDashboardAdmin(null), false);
  assert.equal(isDashboardAdmin({ role: 10 }), true);
  assert.equal(isDashboardAdmin({ role: 100 }), true);
});

test('presets are explicit, bounded, and never mutate the supplied clock', async () => {
  const { dashboardPresetRange } = await import(
    '../src/helpers/dashboardQuery.js'
  );
  const now = new Date('2026-10-27T02:30:20.999Z');
  for (const [preset, days] of [
    ['24h', 1],
    ['7d', 7],
    ['30d', 30],
  ]) {
    const range = dashboardPresetRange(preset, now);
    assert.equal(
      Date.parse(range.end_timestamp) - Date.parse(range.start_timestamp),
      days * 86400000,
    );
    assert.equal(range.end_timestamp, '2026-10-27T02:30:20.000Z');
    assert.equal(validateDashboardQuery(range, range.grain, false), null);
  }
  const today = dashboardPresetRange('today', now);
  assert.equal(new Date(today.start_timestamp).getHours(), 0);
  assert.equal(new Date(today.start_timestamp).getDate(), now.getDate());
  assert.equal(now.toISOString(), '2026-10-27T02:30:20.999Z');
  assert.equal(dashboardPresetRange('custom', now), null);
  assert.equal(dashboardPresetRange('90d', now), null);
  assert.equal(dashboardPresetRange('today', new Date('invalid')), null);
  const midnight = new Date(now);
  midnight.setHours(0, 0, 0, 0);
  assert.equal(dashboardPresetRange('today', midnight), null);
});

test('admin quick actions require both role and existing section/module visibility', async () => {
  const { dashboardQuickActions } = await import(
    '../src/helpers/dashboardQuery.js'
  );
  const allow = () => true;
  assert.deepEqual(
    dashboardQuickActions(false, allow).map((x) => x[1]),
    ['token', 'log', 'topup'],
  );
  assert.deepEqual(
    dashboardQuickActions(true, allow).map((x) => x[1]),
    ['token', 'log', 'channel', 'user'],
  );
  assert.deepEqual(
    dashboardQuickActions(true, (section, module) => section !== 'admin').map(
      (x) => x[1],
    ),
    ['token', 'log'],
  );
  assert.deepEqual(
    dashboardQuickActions(true, (_section, module) => module !== 'channel').map(
      (x) => x[1],
    ),
    ['token', 'log', 'user'],
  );
});

test('rolling 30 days stays within API limit across daylight-saving boundaries', async () => {
  const { dashboardPresetRange } = await import(
    '../src/helpers/dashboardQuery.js'
  );
  for (const instant of ['2026-03-10T16:00:00Z', '2026-11-04T16:00:00Z']) {
    const range = dashboardPresetRange('30d', new Date(instant));
    assert.equal(
      (Date.parse(range.end_timestamp) - Date.parse(range.start_timestamp)) /
        1000,
      2592000,
    );
    assert.equal(validateDashboardQuery(range, range.grain, false), null);
  }
});
