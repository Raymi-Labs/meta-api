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

export function resolveGroupHealthMetric(
  stats,
  group = '',
  useReference = false,
) {
  if (/(?:image|生图|图像)/i.test(group)) return 'completion';
  if (Number(stats?.ttft_count) > 0) return 'ttft';
  if (Number(stats?.completion_count) > 0) return 'completion';
  if (useReference && stats?.last_valid?.ttft) return 'ttft';
  if (useReference && stats?.last_valid?.completion) return 'completion';
  return 'ttft';
}

const healthCount = (value) => {
  const count = Number(value);
  return Number.isFinite(count) && count > 0 ? count : 0;
};

export function getHealthCounts(stats) {
  const request = healthCount(stats?.request_count);
  const success = healthCount(stats?.success_count);
  const failure =
    stats?.failure_count == null
      ? Math.max(0, request - success)
      : healthCount(stats.failure_count);
  const service = healthCount(stats?.service_failure_count);
  const unknown =
    stats?.unknown_failure_count == null
      ? Math.max(0, failure - service)
      : healthCount(stats.unknown_failure_count);
  const disconnected = healthCount(stats?.client_canceled_count);
  const excludedUser = healthCount(stats?.excluded_user_error_count);
  const excluded =
    stats?.excluded_count == null
      ? excludedUser
      : healthCount(stats.excluded_count);
  const observed =
    stats?.observed_count == null
      ? request + excluded
      : healthCount(stats.observed_count);
  return {
    request,
    success,
    failure,
    service,
    unknown,
    disconnected,
    excludedUser,
    excluded,
    observed,
  };
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
    (stats) =>
      stats.group !== 'auto' &&
      Object.prototype.hasOwnProperty.call(usableGroup || {}, stats.group) &&
      ((!exactModel && !filterByModels) ||
        enabledGroups.has(stats.group) ||
        (exactModel &&
          (getHealthCounts(stats).observed > 0 ||
            healthCount(stats.probe_count) > 0))) &&
      (filterGroup === 'all' || stats.group === filterGroup),
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
  const tone = healthRateTone(stats?.success_rate);
  if (tone === 'empty') return tone;
  return requestCount < HEALTH_MIN_SAMPLES ? 'limited' : tone;
}

function healthRateTone(rate) {
  const value = Number(rate);
  if (rate == null || !Number.isFinite(value) || value < 0 || value > 100)
    return 'empty';
  if (value >= HEALTH_HEALTHY_RATE) return 'healthy';
  if (value >= HEALTH_WARNING_RATE) return 'warning';
  return 'degraded';
}

export function isHealthSampleLimited(stats) {
  return healthTone(stats) === 'limited';
}

export function getHealthMeasurement(stats, metric, useReference = false) {
  const fields = {
    ttft: ['avg_ttft_ms', 'ttft_count'],
    completion: ['avg_completion_ms', 'completion_count'],
    cache: ['cache_hit_rate', 'cache_sample_count'],
    output_tps: ['avg_output_tps', 'throughput_sample_count'],
  };
  const [valueKey, countKey] = fields[metric];
  const value = stats?.[valueKey];
  const count = stats?.[countKey] || 0;
  const valid = (candidate) =>
    candidate != null &&
    Number.isFinite(Number(candidate)) &&
    Number(candidate) >= 0 &&
    (metric !== 'cache' || Number(candidate) <= 100);
  if (!useReference || valid(value)) return { value, count };
  const reference = stats?.last_valid?.[metric];
  if (
    valid(reference?.value) &&
    Number.isFinite(reference?.ts) &&
    reference.ts > 0 &&
    (stats?.ts == null || reference.ts <= stats.ts)
  ) {
    return { value: reference.value, count, referenceTs: reference.ts };
  }
  return { value, count };
}

export function getHealthLatency(stats, metric = 'ttft', useReference = false) {
  return getHealthMeasurement(
    stats,
    metric === 'completion' ? 'completion' : 'ttft',
    useReference,
  );
}

export function formatHealthReference(measurement, t) {
  if (!measurement?.referenceTs) return '';
  return t('沿用 {{time}} 的有效值', {
    time: new Date(measurement.referenceTs * 1000).toLocaleString(undefined, {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    }),
  });
}

// Keep request health intact; Uptime consumers explicitly select probe fallback.
export function getUptimeRate(stats) {
  return stats?.uptime_sample_count == null
    ? stats?.success_rate
    : stats.uptime_rate;
}

export function uptimeTone(stats) {
  const probes = healthCount(stats?.probe_bucket_count);
  if (!probes) return healthTone(stats);
  return healthRateTone(getUptimeRate(stats));
}
