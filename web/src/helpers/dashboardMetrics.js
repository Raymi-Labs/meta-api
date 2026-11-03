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

export function dashboardMetricValues(quota, tokens, requests, cacheRate) {
  const valid = (value) =>
    typeof value === 'number' && Number.isFinite(value) && value >= 0;
  const unitPrice =
    valid(quota) && valid(tokens) && tokens > 0
      ? (quota / tokens) * 1000000
      : null;
  return {
    quota: valid(quota) ? quota : null,
    tokens: valid(tokens) ? tokens : null,
    requests: valid(requests) ? requests : null,
    cacheRate: valid(cacheRate) && cacheRate <= 1 ? cacheRate : null,
    unitPrice: valid(unitPrice) ? unitPrice : null,
  };
}

// Do not fall back to the legacy hourly response: that silently restores the
// missing-cache denominator and combines metrics from different snapshots.
export function dashboardUsageSummary(response, start, end) {
  const s = response?.summary;
  const nonnegative = (v) => Number.isSafeInteger(v) && v >= 0;
  const finiteOrNull = (v) =>
    v === null || (typeof v === 'number' && Number.isFinite(v));
  if (
    !response?.success ||
    !s ||
    s.source !== 'retained_settlement_logs_v1' ||
    s.start_timestamp !== start ||
    s.end_timestamp !== end ||
    !Number.isSafeInteger(s.quota) ||
    !Number.isSafeInteger(s.token_quota) ||
    !Number.isSafeInteger(s.other_quota) ||
    ![
      'requests',
      'tokens',
      'priced_tokens',
      'cache_read_tokens',
      'input_tokens',
      'unclassified_records',
    ].every((k) => nonnegative(s[k])) ||
    !finiteOrNull(s.unit_price) ||
    !finiteOrNull(s.cache_rate) ||
    (s.cache_rate !== null && (s.cache_rate < 0 || s.cache_rate > 1))
  ) {
    throw new Error('Invalid unified usage summary');
  }
  return s;
}
