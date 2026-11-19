import assert from 'node:assert/strict';
import { test } from 'bun:test';
import {
  modelUsageReference,
  usageReferenceGroup,
} from '../src/helpers/modelUsageReference';
import {
  chartDisplayValue,
  chartPerformanceReference,
  chartSummary,
  hasChartData,
} from '../src/helpers/modelChartDisplay';
const now = Date.UTC(2026, 10, 1, 12);
const day = Date.UTC(2026, 10, 1) / 1000;
const groups = [{ group: '企业级', request_count: 0, series: [] }];
const reference = {
  source: 'openrouter',
  scope: 'model_global',
  bucket_seconds: 86400,
  fetched_at: now / 1000 - 5,
  expires_at: now / 1000 + 895,
  groups: ['企业级'],
  series: [
    { ts: day, total_tokens: 0, request_count: 0 },
    { ts: day - 86400, total_tokens: 5000, request_count: 10 },
    { ts: day - 10 * 86400, total_tokens: 100, request_count: null },
  ],
};
test('public activity retains daily dates, true zeros and separate totals', () => {
  const before = structuredClone({ reference, groups });
  const series = modelUsageReference(reference, groups, now);
  const week = usageReferenceGroup(series, 7, '模型用量参考', now);
  assert.equal(week.total_tokens, 5000);
  assert.equal(week.request_count, 10);
  assert.equal(week.series.length, 2);
  assert.equal(chartDisplayValue(week.series[1], 'request_count'), 0);
  assert.equal(
    usageReferenceGroup(series, 30, '模型用量参考', now).total_tokens,
    5100,
  );
  assert.deepEqual({ reference, groups }, before);
});
test('local zero usage, real or excluded requests, and unauthorized groups prevent public fallback', () => {
  for (const g of [
    { total_tokens: 0 },
    { request_count: 1 },
    { observed_count: 1 },
    { group: '福利' },
    { series: [{ total_tokens: 0 }] },
    { series: [{ request_count: 1 }] },
  ]) {
    assert.equal(
      modelUsageReference(reference, [{ ...groups[0], ...g }], now),
      null,
    );
  }
  assert.equal(modelUsageReference(reference, [], now), null);
});
test('stale, invalid scope and conflicting date data stay unknown', () => {
  for (const patch of [
    { expires_at: now / 1000 },
    { source: 'other' },
    { scope: 'account' },
    { bucket_seconds: 3600 },
    { series: [...reference.series, reference.series[0]] },
    { series: [{ ts: day + 1, total_tokens: 1 }] },
    { series: [{ ts: day, total_tokens: -1, request_count: '10' }] },
  ]) {
    assert.equal(
      modelUsageReference({ ...reference, ...patch }, groups, now),
      null,
    );
  }
});
test('P50 is a current empty-plot reference, never a replacement for actual history or zero', () => {
  const stats = {
    performance_reference: {
      source: 'openrouter',
      percentile: 50,
      fetched_at: now / 1000 - 5,
      expires_at: now / 1000 + 295,
      ttft_ms: 1915,
      output_tps: 20,
    },
    series: [],
  };
  const before = structuredClone(stats);
  assert.equal(chartPerformanceReference(stats, 'avg_ttft_ms', now), 1.915);
  assert.equal(chartPerformanceReference(stats, 'avg_output_tps', now), 20);
  assert.equal(chartPerformanceReference(stats, 'request_count', now), null);
  assert.equal(
    chartPerformanceReference({ ...stats, avg_ttft_ms: 0 }, 'avg_ttft_ms', now),
    null,
  );
  assert.equal(
    chartPerformanceReference(
      { ...stats, series: [{ ts: day, avg_ttft_ms: 0 }] },
      'avg_ttft_ms',
      now,
    ),
    null,
  );
  assert.deepEqual(stats, before);
});

test('uptime snapshot extends the same reference rules and never becomes history', () => {
  const ref = {
    source: 'openrouter',
    percentile: 50,
    fetched_at: now / 1000 - 5,
    expires_at: now / 1000 + 295,
    uptime_rate: 99.9,
    uptime_window_seconds: 86400,
    ttft_ms: 2000,
    output_tps: 30,
  };
  const group = {
    uptime_sample_count: 0,
    uptime_rate: null,
    performance_reference: ref,
    series: [{ ts: day, uptime_sample_count: 0, uptime_rate: null }],
  };
  const before = structuredClone(group);
  assert.equal(chartPerformanceReference(group, 'uptime_rate', now), 99.9);
  assert.deepEqual(chartSummary(group, 'uptime_rate', now), {
    value: 99.9,
    kind: 'external',
  });
  assert.equal(chartDisplayValue(group.series[0], 'uptime_rate'), null);
  assert.equal(
    chartPerformanceReference(
      {
        ...group,
        series: [{ ts: day, uptime_rate: 0, uptime_sample_count: 1 }],
      },
      'uptime_rate',
      now,
    ),
    null,
  );
  assert.deepEqual(
    chartSummary(
      { ...group, uptime_rate: 0, uptime_sample_count: 1 },
      'uptime_rate',
      now,
    ),
    { value: 0, kind: 'measured' },
  );
  assert.deepEqual(group, before);
});
test('available site summaries and last valid cache/completion fill snapshots only', () => {
  for (const [metric, key, value, expected] of [
    ['avg_ttft_ms', 'ttft', 2500, 2.5],
    ['avg_output_tps', 'output_tps', 30, 30],
    ['avg_completion_ms', 'completion', 5000, 5],
    ['cache_hit_rate', 'cache', 0, 0],
  ]) {
    const g = {
      group: 'g',
      series: [],
      last_valid: { [key]: { value, ts: day - 3600 } },
    };
    assert.deepEqual(chartSummary(g, metric, now), {
      value: expected,
      kind: 'history',
    });
    assert.equal(chartDisplayValue(g, metric), null);
    assert.deepEqual(chartSummary({ ...g, [metric]: 0 }, metric, now), {
      value: 0,
      kind: 'measured',
    });
  }
  const unsupported = {
    performance_reference: {
      source: 'openrouter',
      percentile: 50,
      fetched_at: now / 1000 - 5,
      expires_at: now / 1000 + 295,
      ttft_ms: 10,
      output_tps: 100,
      cache_hit_rate: 80,
      avg_completion_ms: 50,
    },
    series: [],
  };
  for (const metric of [
    'cache_hit_rate',
    'avg_completion_ms',
    'total_tokens',
    'request_count',
  ])
    assert.equal(chartSummary(unsupported, metric, now).value, null);
});

test('comparison shows a positive request snapshot without treating missing tokens or availability as zero', () => {
  const groups = [{ request_count: 29269, series: [] }];
  assert.equal(chartSummary(groups[0], 'request_count').value, 29269);
  assert.equal(hasChartData(groups, 'request_count'), true);
  for (const metric of ['uptime_rate', 'total_tokens'])
    assert.equal(hasChartData(groups, metric), false);
  assert.equal(
    hasChartData([{ series: [{ ts: 1, total_tokens: 0 }] }], 'total_tokens'),
    true,
  );
});
