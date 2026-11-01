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
} from '../components/group-health/healthUtils';

const valid = (value) =>
  value != null && Number.isFinite(Number(value)) && Number(value) >= 0;

// Display fallback only. Never feed references into historical buckets or aggregates.
export function modelPerformance(stats, metric = 'ttft', now = Date.now()) {
  const latency = getHealthLatency(stats, metric, true);
  const throughput = getHealthMeasurement(stats, 'output_tps', true);
  const reference = stats?.performance_reference;
  const fresh =
    reference?.source === 'openrouter' &&
    reference.percentile === 50 &&
    Number(reference.fetched_at) > 0 &&
    Number(reference.fetched_at) * 1000 <= now &&
    Number(reference.expires_at) * 1000 > now;
  const externalLatency =
    metric === 'ttft' &&
    !valid(latency.value) &&
    fresh &&
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
