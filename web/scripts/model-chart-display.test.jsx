import { describe, test, expect, mock } from 'bun:test';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

import {
  chartDisplayValue,
  formatChartValue,
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
      expect(html).toContain('<strong>2.50 s</strong>');
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
      expect(empty).toContain('<strong>—</strong>');
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
