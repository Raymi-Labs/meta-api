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

export const isUsageMetric = (metric) =>
  metric === 'total_tokens' || metric === 'request_count';

// Locale compact notation can leave long numbers unabridged (for example de).
// Keep count labels bounded across languages; details retain the full count.
export function formatCompactUsage(value) {
  if (value == null || !Number.isFinite(value) || value < 0) return '—';
  const suffixes = ['', 'K', 'M', 'B', 'T'];
  let index = 0;
  while (value >= 1000 ** (index + 1) && index < suffixes.length - 1) index++;
  let rounded = Math.round((value / 1000 ** index) * 10) / 10;
  if (rounded >= 1000 && index < suffixes.length - 1) {
    index++;
    rounded = Math.round((value / 1000 ** index) * 10) / 10;
  }
  if (index === 4 && rounded >= 1000) {
    return value.toExponential(1).replace('.0e', 'e').replace('e+', 'e');
  }
  return String(rounded) + suffixes[index];
}

export function formatChartDetailValue(value, metric) {
  if (isUsageMetric(metric)) {
    return value != null && Number.isFinite(value) && value >= 0
      ? new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(
          value,
        )
      : '—';
  }
  return formatChartValue(value, metric);
}

export function chartDisplayValue(point, metric) {
  const value = chartMetricValue(point, metric);
  return value == null ? null : isLatencyMetric(metric) ? value / 1000 : value;
}

export function formatChartValue(value, metric, withUnit = true) {
  if (value == null || !Number.isFinite(value)) return '—';
  if (isLatencyMetric(metric)) return value.toFixed(2) + (withUnit ? ' s' : '');
  if (isUsageMetric(metric)) return formatCompactUsage(value);
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
  const rawStep = Math.max(
    (peak || 1) / 4,
    isLatencyMetric(metric) ? 0.01 : isUsageMetric(metric) ? 1 : 0,
  );
  const magnitude = 10 ** Math.floor(Math.log10(rawStep));
  const niceStep =
    [1, 2, 2.5, 5, 10].find((n) => n * magnitude >= rawStep) * magnitude;
  const step = isUsageMetric(metric) ? Math.ceil(niceStep) : niceStep;
  return { min: 0, max: step * 4, ticks: [0, 1, 2, 3, 4].map((n) => n * step) };
}

// Hourly observations must not be joined across missing buckets.
export function contiguousChartPoints(previous, current) {
  return previous != null && current > previous && current - previous <= 3600;
}
