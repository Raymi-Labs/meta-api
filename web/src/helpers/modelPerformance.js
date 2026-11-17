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
  value != null && Number.isFinite(Number(value)) && Number(value) >= 0;

// Display fallback only. Never feed references into historical buckets or aggregates.
export function modelPerformance(stats, metric = 'ttft', now = Date.now()) {
  const latency = getHealthLatency(stats, metric, true);
  const throughput = getHealthMeasurement(stats, 'output_tps', true);
  const reference = stats?.performance_reference;
  const fresh =
    isFreshModelReference(reference, now) && reference.percentile === 50;
  const externalLatency =
    metric === 'ttft' &&
    !valid(latency.value) &&
    fresh &&
    typeof reference.ttft_ms === 'number' &&
    valid(reference.ttft_ms);
  const externalThroughput =
    !valid(throughput.value) && fresh && valid(reference.output_tps);
  return {
    latency: externalLatency
      ? { value: Number(reference.ttft_ms), count: 0 }
      : latency,
    throughput: externalThroughput
      ? Number(reference.output_tps)
      : throughput.value,
    throughputReferenceTs: throughput.referenceTs,
    externalLatency: Boolean(externalLatency),
    externalThroughput: Boolean(externalThroughput),
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
