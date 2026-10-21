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
export const HEALTH_HEALTHY_RATE = 95;
export const HEALTH_WARNING_RATE = 80;

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
  const requestCount = Number(stats?.request_count);
  if (
    stats?.request_count == null ||
    !Number.isFinite(requestCount) ||
    requestCount <= 0
  )
    return 'empty';
  const successRate = Number(stats?.success_rate);
  if (
    stats?.success_rate == null ||
    !Number.isFinite(successRate) ||
    successRate < 0 ||
    successRate > 100
  )
    return 'empty';
  if (successRate >= HEALTH_HEALTHY_RATE) return 'healthy';
  if (successRate >= HEALTH_WARNING_RATE) return 'warning';
  return 'degraded';
}

export function isHealthSampleLimited(stats) {
  const count = Number(stats?.request_count);
  return healthTone(stats) !== 'empty' && count < HEALTH_MIN_SAMPLES;
}

export function getHealthLatency(stats, metric = 'ttft') {
  return metric === 'completion'
    ? { value: stats?.avg_completion_ms, count: stats?.completion_count || 0 }
    : { value: stats?.avg_ttft_ms, count: stats?.ttft_count || 0 };
}
