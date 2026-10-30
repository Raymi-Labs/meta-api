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

import { describe, test, expect, mock } from 'bun:test';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

import {
  chartDisplayValue,
  formatChartValue,
  formatChartDetailValue,
  formatCompactUsage,
  chartDomain,
  contiguousChartPoints,
} from '../src/helpers/modelChartDisplay';

// Semi UI imports browser-only animation code. Isolate its shell mocks so other
// suites still exercise their own real imports regardless of test order.
if (process.env.META_MODEL_CHART_TEST === import.meta.url) {
  mock.module('@douyinfe/semi-ui', () => ({
    Button: ({ children, icon, ...props }) => (
      <button {...props}>
        {icon}
        {children}
      </button>
    ),
    Modal: () => null,
  }));
  const { default: ModelMetricChart } = await import(
    '../src/pages/ModelDetail/ModelMetricChart'
  );
  describe('model chart display units and sample integrity', () => {
    test('usage abbreviations stay bounded while details retain full counts', () => {
      for (const [value, expected] of [
        [0, '0'],
        [999, '999'],
        [1000, '1K'],
        [999950, '1M'],
        [822800000, '822.8M'],
        [1050000000, '1.1B'],
        [1000000000000, '1T'],
        [1000000000000000, '1e15'],
      ]) {
        expect(formatCompactUsage(value)).toBe(expected);
        for (const metric of ['total_tokens', 'request_count']) {
          expect(formatChartValue(value, metric)).toBe(expected);
          expect(formatChartDetailValue(value, metric)).toBe(
            new Intl.NumberFormat(undefined, {
              maximumFractionDigits: 0,
            }).format(value),
          );
        }
      }
      for (const value of [null, undefined, -1, Infinity, NaN]) {
        expect(formatCompactUsage(value)).toBe('—');
        expect(formatChartDetailValue(value, 'total_tokens')).toBe('—');
      }
      for (const peak of [0, 1, 3, 9, 99]) {
        const domain = chartDomain([peak], 'request_count');
        expect(domain.max).toBeGreaterThanOrEqual(peak);
        expect(domain.ticks.every(Number.isInteger)).toBe(true);
        expect(new Set(domain.ticks).size).toBe(domain.ticks.length);
      }
    });

    test('usage chart abbreviates summary and axis, but exposes exact hourly and aggregate values', () => {
      const count = 1234567890123;
      const exact = formatChartDetailValue(count, 'total_tokens');
      const html = renderToStaticMarkup(
        <ModelMetricChart
          title='Tokens'
          metric='total_tokens'
          bar
          t={(key) => key}
          groups={[
            {
              group: '企业专属',
              total_tokens: count,
              series: [
                { ts: 0, total_tokens: count },
                { ts: 3600, total_tokens: null },
                { ts: 7200, total_tokens: 0 },
              ],
            },
          ]}
        />,
      );
      expect(html).toContain(`<strong title="${exact}">1.2T</strong>`);
      expect(html).toContain(`企业专属: ${exact}`);
      expect(html).toContain(`<td>${exact}</td>`);
      expect(html).toContain('<td>—</td>');
      expect(html).toContain('<td>0</td>');
      expect(html).toContain('>1.5T</text>');
    });
    test('both latency metrics convert milliseconds once and always show two decimals', () => {
      for (const metric of ['avg_ttft_ms', 'avg_completion_ms']) {
        for (const [raw, expected] of [
          [6672, '6.67 s'],
          [250, '0.25 s'],
          [1000, '1.00 s'],
          [0, '0.00 s'],
        ]) {
          const point = { [metric]: raw };
          expect(
            formatChartValue(chartDisplayValue(point, metric), metric),
          ).toBe(expected);
          expect(point[metric]).toBe(raw);
        }
        expect(formatChartValue(1, metric, false)).toBe('1.00');
        for (const value of [null, undefined, -1, Infinity, 'invalid']) {
          expect(chartDisplayValue({ [metric]: value }, metric)).toBeNull();
        }
        expect(formatChartValue(null, metric)).toBe('—');
      }
    });

    test('success scale labels close-to-100 changes without clipping outages; latency ticks stay distinct', () => {
      for (const values of [[100, 99.9], [98, 93], [0, 100], []]) {
        const domain = chartDomain(values, 'success_rate');
        expect(domain.min).toBeGreaterThanOrEqual(0);
        expect(domain.max).toBe(100);
        expect(domain.min).toBeLessThan(Math.min(100, ...values) || 1);
      }
      for (const values of [[0], [0.001], [0.01, 6.672], [120]]) {
        const domain = chartDomain(values, 'avg_ttft_ms');
        expect(domain.max).toBeGreaterThanOrEqual(Math.max(...values));
        expect(
          new Set(
            domain.ticks.map((n) => formatChartValue(n, 'avg_ttft_ms', false)),
          ).size,
        ).toBe(domain.ticks.length);
      }
      expect(contiguousChartPoints(0, 3600)).toBe(true);
      for (const [before, after] of [
        [null, 3600],
        [0, 7200],
        [3600, 0],
        [0, 0],
      ]) {
        expect(contiguousChartPoints(before, after)).toBe(false);
      }
    });

    test('rendered tooltip text, table and weighted summary use seconds, preserving gaps and zero', () => {
      const html = renderToStaticMarkup(
        <ModelMetricChart
          title='首字均值 · s'
          metric='avg_ttft_ms'
          t={(key) => key}
          groups={[
            {
              group: '标准',
              avg_ttft_ms: 2500,
              series: [
                { ts: 0, avg_ttft_ms: 0 },
                { ts: 3600, avg_ttft_ms: 1000 },
                { ts: 7200, avg_ttft_ms: null },
                { ts: 10800, avg_ttft_ms: 6672 },
              ],
            },
          ]}
        />,
      );
      expect(html).toContain('标准: 6.67 s');
      expect(html).toContain('<td>0.00 s</td>');
      expect(html).toContain('<td>—</td>');
      expect(html).toContain('<strong title="2.50 s">2.50 s</strong>');
      expect(html).not.toContain(' ms');
      // Only the first pair is adjacent; neither the null bucket nor later data bridge it.
      expect((html.match(/stroke-width="1.8"/g) || []).length).toBe(1);
      expect(html).toContain('aria-label="展开 · 首字均值 · s"');
    });

    test('low-sample success remains gray; no-data and usage do not invent observations', () => {
      const render = (metric, groups, bar = false) =>
        renderToStaticMarkup(
          <ModelMetricChart
            title='Metric'
            metric={metric}
            groups={groups}
            bar={bar}
            t={(key) => key}
          />,
        );
      const success = render('success_rate', [
        {
          group: '小样本',
          request_count: 2,
          success_rate: 100,
          series: [{ ts: 3600, request_count: 2, success_rate: 100 }],
        },
      ]);
      expect(success).toContain('fill="#a8aeb9"');
      expect(success).toContain('100%');
      const empty = render('avg_ttft_ms', [{ group: '无样本', series: [] }]);
      expect(empty).toContain('暂无数据');
      expect(empty).toContain('<strong title="—">—</strong>');
      expect(empty).not.toContain('<table>');
      const usage = render(
        'total_tokens',
        [
          {
            group: '用量',
            total_tokens: 0,
            series: [{ ts: 3600, total_tokens: 0, usage_sample_count: 1 }],
          },
        ],
        true,
      );
      expect(usage).toContain('<td>0</td>');
      expect(usage).not.toContain(' s</td>');
    });
  });
} else {
  test('model chart display contracts pass in an isolated process', () => {
    const result = spawnSync(
      process.execPath,
      ['test', fileURLToPath(import.meta.url)],
      {
        env: { ...process.env, META_MODEL_CHART_TEST: import.meta.url },
        encoding: 'utf8',
        timeout: 30000,
      },
    );
    expect(result.status, result.stdout + result.stderr).toBe(0);
  });
}
