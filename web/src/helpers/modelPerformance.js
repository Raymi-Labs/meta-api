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

import {
  getHealthLatency,
  getHealthMeasurement,
  getUptimeRate,
} from '../components/group-health/healthUtils';

const valid = (value) =>
  value != null &&
  value !== '' &&
  Number.isFinite(Number(value)) &&
  Number(value) >= 0;

// Display fallback only. Never feed references into historical buckets or aggregates.
export function modelPerformance(stats, metric = 'ttft', now = Date.now()) {
  const latency = getHealthLatency(stats, metric, true);
  const throughput = getHealthMeasurement(stats, 'output_tps', true);
  // Site window, then a fresh OR sample for this exact model, then site history.
  // Resolve per field: a route can provide TTFT while only the market has speed.
  const refs = [
    stats?.performance_reference,
    stats?.market_speed_reference,
  ].filter((ref) => isFreshModelReference(ref, now) && ref.percentile === 50);
  const orValue = (field) =>
    refs.find((r) => typeof r[field] === 'number' && valid(r[field]));
  const ttftRef =
    metric === 'ttft' && (!valid(latency.value) || latency.referenceTs)
      ? orValue('ttft_ms')
      : undefined;
  const speedRef =
    !valid(throughput.value) || throughput.referenceTs
      ? orValue('output_tps')
      : undefined;
  const ttft = ttftRef?.ttft_ms,
    speed = speedRef?.output_tps;
  return {
    latencyWindow: ttftRef?.latency_window_minutes ?? 30,
    speedWindow: speedRef?.throughput_window_minutes ?? 30,
    latency: ttft != null ? { value: ttft, count: 0 } : latency,
    throughput: speed ?? throughput.value,
    throughputReferenceTs: speed != null ? undefined : throughput.referenceTs,
    externalLatency: ttft != null,
    externalThroughput: speed != null,
  };
}

// External availability is a current reference, never a site history sample.
export function modelUptime(stats, now = Date.now()) {
  const measured = getUptimeRate(stats);
  const validRate = (v) =>
    typeof v === 'number' && Number.isFinite(v) && v >= 0 && v <= 100;
  if (validRate(measured)) return { value: measured, external: false };
  const ref = stats?.performance_reference;
  if (
    isFreshModelReference(ref, now) &&
    ref.uptime_window_seconds === 86400 &&
    validRate(ref.uptime_rate)
  ) {
    return {
      value: ref.uptime_rate,
      external: true,
      fetchedAt: ref.fetched_at,
      windowSeconds: 86400,
    };
  }
  return { value: undefined, external: false };
}

// A reference snapshot is usable for at most the service's five-minute TTL.
export function isFreshModelReference(ref, now = Date.now()) {
  return (
    ref?.source === 'openrouter' &&
    typeof ref.fetched_at === 'number' &&
    typeof ref.expires_at === 'number' &&
    Number.isFinite(ref.fetched_at) &&
    Number.isFinite(ref.expires_at) &&
    ref.fetched_at > 0 &&
    ref.fetched_at * 1000 <= now &&
    ref.expires_at * 1000 > now &&
    ref.expires_at - ref.fetched_at <= 300
  );
}
