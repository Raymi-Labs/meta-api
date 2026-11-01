/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

const count = (value) =>
  typeof value === 'number' && Number.isSafeInteger(value) && value >= 0;

// Keep model-wide public traffic separate from the site's group measurements.
export function modelUsageReference(reference, groups, now = Date.now()) {
  if (
    !groups.length ||
    reference?.source !== 'openrouter' ||
    reference.scope !== 'model_global' ||
    reference.bucket_seconds !== 86400 ||
    !(reference.fetched_at > 0 && reference.fetched_at * 1000 <= now) ||
    !(reference.expires_at * 1000 > now) ||
    !Array.isArray(reference.series) ||
    reference.series.length > 30 ||
    groups.some(
      (group) =>
        !reference.groups?.includes(group.group) ||
        group.observed_count > 0 ||
        group.request_count > 0 ||
        group.total_tokens != null ||
        (group.series || []).some(
          (p) =>
            p.observed_count > 0 ||
            p.request_count > 0 ||
            p.total_tokens != null,
        ),
    )
  )
    return null;
  const seen = new Set();
  const series = [];
  const today = Math.floor(now / 86400000) * 86400;
  for (const point of reference.series) {
    if (
      !Number.isInteger(point.ts) ||
      point.ts % 86400 !== 0 ||
      seen.has(point.ts) ||
      point.ts > today
    )
      return null;
    seen.add(point.ts);
    if (point.ts < today - 29 * 86400) continue;
    series.push({
      ts: point.ts,
      total_tokens: count(point.total_tokens) ? point.total_tokens : null,
      request_count: count(point.request_count) ? point.request_count : null,
      model_usage_reference: true,
    });
  }
  return series.some((p) => p.total_tokens != null || p.request_count != null)
    ? series.sort((a, b) => a.ts - b.ts)
    : null;
}

export function usageReferenceGroup(series, days, label, now = Date.now()) {
  const start = Math.floor(now / 86400000) * 86400 - (days - 1) * 86400;
  const points = series.filter((p) => p.ts >= start);
  const sum = (metric) => {
    const values = points.map((p) => p[metric]).filter(count);
    const total = values.reduce((a, b) => a + b, 0);
    return values.length && count(total) ? total : null;
  };
  return {
    group: label,
    series: points,
    model_usage_reference: true,
    total_tokens: sum('total_tokens'),
    request_count: sum('request_count'),
  };
}
