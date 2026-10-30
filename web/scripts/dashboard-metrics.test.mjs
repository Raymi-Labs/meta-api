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
  dashboardCacheStatUrl,
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

test('cache queries retain time boundaries and exact administrator username semantics', () => {
  const inputs = {
    start_timestamp: '2026-10-29T15:18:00+08:00',
    end_timestamp: '2026-10-30T15:18:00+08:00',
    username: '123&other',
  };
  const admin = new URL(
    dashboardCacheStatUrl(inputs, true),
    'http://localhost',
  );
  assert.equal(admin.pathname, '/api/log/stat');
  assert.equal(admin.searchParams.get('username'), 'name:123&other');
  assert.equal(
    Number(admin.searchParams.get('end_timestamp')) -
      Number(admin.searchParams.get('start_timestamp')),
    86400,
  );
  const self = new URL(
    dashboardCacheStatUrl(inputs, false),
    'http://localhost',
  );
  assert.equal(self.pathname, '/api/log/self/stat');
  assert.equal(self.searchParams.has('username'), false);
});
