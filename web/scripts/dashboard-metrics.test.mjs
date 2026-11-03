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

import assert from 'node:assert/strict';
import { test } from 'bun:test';
import {
  dashboardMetricValues,
  dashboardUsageSummary,
} from '../src/helpers/dashboardMetrics.js';

test('unit cost uses weighted totals and handles missing or zero usage', () => {
  const stats = dashboardMetricValues(18500000, 28000000, 2680, 0.628);
  assert.equal(stats.unitPrice, 18500000 / 28);
  assert.equal(stats.cacheRate, 0.628);
  assert.equal(dashboardMetricValues(0, 0, 0, 0).unitPrice, null);
  assert.equal(dashboardMetricValues(0, 10, 1, 0).unitPrice, 0);
  for (const invalid of [null, undefined, NaN, Infinity, -1, '42']) {
    const result = dashboardMetricValues(invalid, invalid, invalid, invalid);
    assert.deepEqual(result, {
      quota: null,
      tokens: null,
      requests: null,
      cacheRate: null,
      unitPrice: null,
    });
  }
  assert.equal(dashboardMetricValues(1, 1, 1, 1.2).cacheRate, null);
});

test('unified snapshot accepts negative net spend but rejects old, stale and partial responses', () => {
  const summary = {
    source: 'retained_settlement_logs_v1',
    start_timestamp: 100,
    end_timestamp: 200,
    quota: -30,
    token_quota: 10,
    other_quota: -40,
    requests: 2,
    tokens: 120,
    priced_tokens: 100,
    input_tokens: 100,
    cache_read_tokens: 80,
    unclassified_records: 0,
    unit_price: 100000,
    cache_rate: 0.8,
  };
  assert.equal(
    dashboardUsageSummary({ success: true, summary }, 100, 200),
    summary,
  );
  assert.throws(() =>
    dashboardUsageSummary({ success: true, data: [] }, 100, 200),
  );
  assert.throws(() =>
    dashboardUsageSummary({ success: false, summary }, 100, 200),
  );
  assert.throws(() =>
    dashboardUsageSummary({ success: true, summary }, 101, 200),
  );
  for (const patch of [
    { tokens: -1 },
    { tokens: Number.MAX_SAFE_INTEGER + 1 },
    { cache_rate: 1.1 },
    { unit_price: Infinity },
    { quota: '30' },
  ]) {
    assert.throws(() =>
      dashboardUsageSummary(
        { success: true, summary: { ...summary, ...patch } },
        100,
        200,
      ),
    );
  }
  const unknown = {
    ...summary,
    unclassified_records: 1,
    unit_price: null,
    cache_rate: null,
  };
  assert.equal(
    dashboardUsageSummary({ success: true, summary: unknown }, 100, 200),
    unknown,
  );
});
