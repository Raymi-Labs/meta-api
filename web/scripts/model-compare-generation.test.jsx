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
import React from 'react';
import { StaticRouter } from 'react-router-dom/server';
import { renderToStaticMarkup } from 'react-dom/server';
import Metric from '../src/pages/ModelCompare/ComparisonGenerationMetric';
const render = (props = {}) =>
  renderToStaticMarkup(
    <StaticRouter>
      <Metric
        model={{ model_name: 'm', enable_groups: ['g'] }}
        health={{ isAuthenticated: true }}
        group='g'
        t={(s) => s}
        latency
        {...props}
      />
    </StaticRouter>,
  );
test('comparison only displays valid current or labeled same-group fallback measurements', () => {
  const now = Math.floor(Date.now() / 1000);
  const ref = {
    source: 'openrouter',
    percentile: 50,
    fetched_at: now - 5,
    expires_at: now + 200,
    ttft_ms: 1250,
    output_tps: 20,
  };
  const market = { ...ref, throughput_window_minutes: 1440 };
  assert.match(
    render({
      latency: false,
      stats: { market_speed_reference: market },
      t: (k, o) => (o?.hours === 24 ? '24h' : k),
    }),
    /20.0 tok\/s · 24h 参考 P50/,
  );
  assert.match(
    render({ stats: { avg_ttft_ms: 0, performance_reference: ref } }),
    /0 ms</,
  );
  assert.match(
    render({ stats: { performance_reference: ref } }),
    /1.25 s · 近30分钟 参考 P50/,
  );
  assert.match(
    render({ latency: false, stats: { performance_reference: ref } }),
    /20.0 tok\/s · 近30分钟 参考 P50/,
  );
  assert.match(
    render({
      stats: {
        last_valid: { ttft: { value: 400, ts: now - 60 } },
        performance_reference: ref,
      },
    }),
    /1.25 s · 近30分钟 参考 P50/,
  );
  assert.match(
    render({
      stats: { performance_reference: { ...ref, expires_at: now - 1 } },
    }),
    /暂无统计数据/,
  );
});
test('missing, unsupported and restricted metrics are not empty or invented zeroes', () => {
  for (const [props, expected] of [
    [{}, '暂无统计数据'],
    [{ stats: { request_count: 4 } }, '样本不足'],
    [{ group: 'other' }, '当前分组不可用'],
    [{ model: { output_modalities: ['image'] } }, '不支持'],
    [
      { health: { isAuthenticated: false }, stats: { avg_ttft_ms: 123 } },
      '登录后查看',
    ],
  ]) {
    const out = render(props);
    assert.ok(out.includes(expected));
    assert.doesNotMatch(out, /—|123 ms|0 ms/);
  }
});
