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

export const HEALTH_MIN_SAMPLES = 30;

export function formatHealthLatency(value) {
  if (value == null || !Number.isFinite(Number(value)) || Number(value) < 0)
    return '—';
  return Number(value) < 1000
    ? `${Math.round(Number(value))} ms`
    : `${(Number(value) / 1000).toFixed(2)} s`;
}

export function formatHealthRate(value) {
  return value == null || !Number.isFinite(Number(value))
    ? '—'
    : `${Number(value).toFixed(2)}%`;
}

export function resolveGroupHealthMetric(stats, group = '') {
  if (/(?:image|生图|图像)/i.test(group)) return 'completion';
  if (Number(stats?.ttft_count) > 0) return 'ttft';
  if (Number(stats?.completion_count) > 0) return 'completion';
  return 'ttft';
}

/** Preserve outages in unfiltered/exact-model data, never in unrelated aggregates. */
export function selectHealthGroups({
  groups = [],
  models = [],
  usableGroup = {},
  filterGroup = 'all',
  exactModel = false,
  filterByModels = false,
}) {
  const enabledGroups = new Set(
    models.flatMap((model) => model.enable_groups || []),
  );
  return groups.filter(
    ({ group, request_count: requestCount }) =>
      group !== 'auto' &&
      Object.prototype.hasOwnProperty.call(usableGroup || {}, group) &&
      ((!exactModel && !filterByModels) ||
        enabledGroups.has(group) ||
        (exactModel && Number(requestCount) > 0)) &&
      (filterGroup === 'all' || group === filterGroup),
  );
}

export function healthTone(stats) {
  if (!stats?.request_count || stats?.success_rate == null) return 'empty';
  if (stats.request_count < HEALTH_MIN_SAMPLES) return 'limited';
  if (stats.success_rate >= 99) return 'healthy';
  if (stats.success_rate >= 90) return 'warning';
  return 'degraded';
}

// Colors have their own policy; keep measured rates and status wording intact.
export function healthColorTone(stats) {
  const requestCount = Number(stats?.request_count);
  if (
    stats?.request_count == null ||
    !Number.isFinite(requestCount) ||
    requestCount < 0
  )
    return 'empty';
  if (requestCount < 100) return 'healthy';
  const successRate = Number(stats?.success_rate);
  if (stats?.success_rate == null || !Number.isFinite(successRate))
    return 'empty';
  if (successRate >= 95) return 'healthy';
  if (successRate >= 80) return 'warning';
  return 'degraded';
}

export function getHealthLatency(stats, metric = 'ttft') {
  return metric === 'completion'
    ? { value: stats?.avg_completion_ms, count: stats?.completion_count || 0 }
    : { value: stats?.avg_ttft_ms, count: stats?.ttft_count || 0 };
}
