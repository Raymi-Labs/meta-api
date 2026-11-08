export function activityWeeks(days) {
  const cells = Array.from(
    { length: new Date(`${days[0].date}T00:00:00Z`).getUTCDay() },
    () => null,
  );
  cells.push(...days);
  while (cells.length % 7) cells.push(null);
  return Array.from({ length: cells.length / 7 }, (_, i) =>
    cells.slice(i * 7, i * 7 + 7),
  );
}

// Raw settlement logs, or settled hourly aggregates plus raw logs for today;
// both count consume settlements without task adjustments.
const ACTIVITY_SOURCES = [
  'retained_settlement_logs_v1',
  'settlement_hourly_v1',
];

export function validActivity(data) {
  if (
    !Number.isSafeInteger(data?.as_of) ||
    typeof data?.timezone !== 'string' ||
    !ACTIVITY_SOURCES.includes(data?.source) ||
    !Array.isArray(data.days) ||
    data.days.length !== 365
  )
    return false;
  let total = 0,
    streak = 0,
    longest = 0,
    previous = 0;
  for (const day of data.days) {
    if (
      !day ||
      typeof day.date !== 'string' ||
      !/^\d{4}-\d{2}-\d{2}$/.test(day.date) ||
      !Number.isSafeInteger(day.requests) ||
      day.requests < 0
    )
      return false;
    const date = Date.parse(`${day.date}T00:00:00Z`);
    if (
      !Number.isFinite(date) ||
      new Date(date).toISOString().slice(0, 10) !== day.date ||
      (previous && date - previous !== 86400000)
    )
      return false;
    previous = date;
    total += day.requests;
    streak = day.requests ? streak + 1 : 0;
    longest = Math.max(longest, streak);
  }
  return (
    Number.isSafeInteger(total) &&
    total === data.total &&
    longest === data.longest_streak &&
    Math.abs(data.daily_average - total / 365) < 1e-8 &&
    Math.abs(data.weekly_average - (total * 7) / 365) < 1e-8
  );
}
