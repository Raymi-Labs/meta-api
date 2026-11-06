import { test } from 'bun:test';
import assert from 'node:assert/strict';
import { activityWeeks, validActivity } from '../src/helpers/usageCalendar';
const days = Array.from({ length: 365 }, (_, i) => ({
  date: new Date(Date.UTC(2023, 2, 14 + i)).toISOString().slice(0, 10),
  requests: i > 361 ? 1 : 0,
}));
const data = {
  source: 'retained_settlement_logs_v1',
  as_of: 1710268800,
  timezone: 'America/New_York',
  days,
  total: 3,
  longest_streak: 3,
  daily_average: 3 / 365,
  weekly_average: 21 / 365,
};
test('calendar preserves 365 dates across leap year and aligns weekdays', () => {
  assert(validActivity(data));
  const weeks = activityWeeks(days);
  assert.equal(weeks.flat().filter(Boolean).length, 365);
  assert.equal(weeks[0][2].date, '2023-03-14');
  assert.equal(weeks.flat().filter(Boolean).at(-1).date, '2024-03-12');
});
test('invalid, partial or inconsistent results never become zero usage', () => {
  for (const altered of [
    { days: days.slice(1) },
    { total: 4 },
    { longest_streak: 4 },
    { daily_average: 3 },
    { weekly_average: 8 },
    { source: 'unknown' },
    { as_of: null },
    { days: days.map((d, i) => (i === 10 ? { ...d, requests: -1 } : d)) },
    { days: days.map((d, i) => (i === 10 ? { ...d, date: days[9].date } : d)) },
  ])
    assert(!validActivity({ ...data, ...altered }));
});
test('a retained empty window is valid without invented streaks', () => {
  assert(
    validActivity({
      ...data,
      days: days.map((d) => ({ ...d, requests: 0 })),
      total: 0,
      longest_streak: 0,
      daily_average: 0,
      weekly_average: 0,
    }),
  );
});
