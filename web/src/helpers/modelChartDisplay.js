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

import { chartMetricValue } from './modelPresentation';

export const isLatencyMetric = (metric) =>
  metric === 'avg_ttft_ms' || metric === 'avg_completion_ms';

export function chartDisplayValue(point, metric) {
  const value = chartMetricValue(point, metric);
  return value == null ? null : isLatencyMetric(metric) ? value / 1000 : value;
}

export function formatChartValue(value, metric, withUnit = true) {
  if (value == null || !Number.isFinite(value)) return '—';
  if (isLatencyMetric(metric)) return value.toFixed(2) + (withUnit ? ' s' : '');
  const number = new Intl.NumberFormat(undefined, {
    maximumFractionDigits: metric === 'success_rate' ? 2 : 1,
    notation: Math.abs(value) >= 10000 ? 'compact' : 'standard',
  }).format(value);
  const unit =
    metric === 'success_rate'
      ? '%'
      : metric === 'avg_output_tps'
        ? ' tok/s'
        : '';
  return number + (withUnit ? unit : '');
}

export function chartDomain(values, metric) {
  if (metric === 'success_rate') {
    const minimum = Math.min(100, ...values);
    // Show small changes near 100%, with explicit ticks and a bounded scale.
    const step = minimum >= 95 ? 1 : minimum >= 80 ? 5 : 25;
    const min = Math.max(0, Math.floor((minimum - step) / step) * step);
    return {
      min,
      max: 100,
      ticks: Array.from({ length: 5 }, (_, i) => min + ((100 - min) * i) / 4),
    };
  }
  const peak = Math.max(0, ...values);
  const rawStep = Math.max((peak || 1) / 4, isLatencyMetric(metric) ? 0.01 : 0);
  const magnitude = 10 ** Math.floor(Math.log10(rawStep));
  const step =
    [1, 2, 2.5, 5, 10].find((n) => n * magnitude >= rawStep) * magnitude;
  return { min: 0, max: step * 4, ticks: [0, 1, 2, 3, 4].map((n) => n * step) };
}

// Hourly observations must not be joined across missing buckets.
export function contiguousChartPoints(previous, current) {
  return previous != null && current > previous && current - previous <= 3600;
}
