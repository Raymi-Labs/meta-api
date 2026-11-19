import assert from 'node:assert/strict';
import { test } from 'bun:test';
import {
  modelPerformance,
  modelUptime,
} from '../src/helpers/modelPerformance.js';
const now = 1800000000000;
const reference = {
  source: 'openrouter',
  percentile: 50,
  fetched_at: now / 1000 - 5,
  expires_at: now / 1000 + 295,
  ttft_ms: 1915,
  output_tps: 20,
};
const calculate = (stats = {}, metric = 'ttft') =>
  modelPerformance({ performance_reference: reference, ...stats }, metric, now);
test('P50 fills missing fields individually without changing measured statistics', () => {
  const stats = {
    avg_ttft_ms: 0,
    avg_output_tps: null,
    request_count: 0,
    performance_reference: reference,
  };
  const before = structuredClone(stats);
  const display = calculate(stats);
  assert.equal(display.latency.value, 0);
  assert.equal(display.externalLatency, false);
  assert.equal(display.throughput, 20);
  assert.equal(display.externalThroughput, true);
  assert.deepEqual(stats, before);
  assert.equal(calculate({ avg_output_tps: 0 }).throughput, 0);
  assert.equal(calculate().latency.value, 1915);
});
test('fresh OR TTFT precedes old site history; external TTFT cannot fill completion latency', () => {
  const display = calculate({
    last_valid: { ttft: { value: 3250, ts: now / 1000 - 3600 } },
  });
  assert.equal(display.latency.value, 1915);
  assert.equal(display.externalLatency, true);
  assert.equal(calculate({}, 'completion').latency.value, undefined);
});
test('expired, missing, malformed or non-P50 references remain unknown', () => {
  for (const ref of [
    null,
    { ...reference, expires_at: now / 1000 },
    { ...reference, percentile: 90 },
    { ...reference, source: 'wrong' },
    { ...reference, ttft_ms: -1, output_tps: 'bad' },
  ]) {
    const display = calculate({ performance_reference: ref });
    assert.equal(display.externalLatency, false);
    assert.equal(display.externalThroughput, false);
  }
});

test('historical throughput fills independently and current zero wins over history and P50', () => {
  const stats = {
    last_valid: {
      output_tps: { value: 73.69682444577592, ts: now / 1000 - 80000 },
    },
  };
  const before = structuredClone(stats);
  const display = calculate({
    ...stats,
    performance_reference: { ...reference, output_tps: null },
  });
  assert.equal(display.throughput, stats.last_valid.output_tps.value);
  assert.equal(display.throughputReferenceTs, stats.last_valid.output_tps.ts);
  assert.equal(display.externalThroughput, false);
  assert.equal(display.latency.value, 1915);
  assert.deepEqual(stats, before);
  assert.equal(calculate({ ...stats, avg_output_tps: 0 }).throughput, 0);
  assert.equal(
    calculate({ ...stats, avg_output_tps: 0 }).throughputReferenceTs,
    undefined,
  );
  for (const ref of [
    { value: -1, ts: 1 },
    { value: NaN, ts: 1 },
    { value: 100, ts: 0 },
    { value: 100, ts: 'bad' },
  ]) {
    assert.equal(calculate({ last_valid: { output_tps: ref } }).throughput, 20);
  }
});

test('daily uptime fills only missing site measurements without mutating statistics', () => {
  const stats = {
    uptime_sample_count: 0,
    uptime_rate: null,
    performance_reference: {
      ...reference,
      uptime_rate: 98.76,
      uptime_window_seconds: 86400,
    },
  };
  const before = structuredClone(stats);
  assert.equal(modelUptime(stats, now).value, 98.76);
  assert.equal(modelUptime(stats, now).external, true);
  assert.equal(
    modelUptime({ ...stats, uptime_rate: 0, uptime_sample_count: 5 }, now)
      .external,
    false,
  );
  assert.equal(
    modelUptime({ ...stats, uptime_rate: 0, uptime_sample_count: 5 }, now)
      .value,
    0,
  );
  assert.deepEqual(stats, before);
  assert.equal(
    modelUptime(
      { success_rate: 0, performance_reference: stats.performance_reference },
      now,
    ).external,
    false,
  );
});
test('uptime rejects wrong sources, periods, invalid numbers and stale references', () => {
  const ref = { ...reference, uptime_rate: 0, uptime_window_seconds: 86400 };
  assert.equal(modelUptime({ performance_reference: ref }, now).value, 0);
  for (const patch of [
    { uptime_rate: null },
    { uptime_rate: undefined },
    { uptime_rate: '100' },
    { uptime_rate: NaN },
    { uptime_rate: -1 },
    { uptime_rate: 101 },
    { uptime_window_seconds: 1800 },
    { source: 'other' },
    { fetched_at: now / 1000 + 1 },
    { expires_at: now / 1000 },
    { expires_at: now / 1000 + 600 },
    { fetched_at: 0 },
  ]) {
    assert.equal(
      modelUptime({ performance_reference: { ...ref, ...patch } }, now)
        .external,
      false,
      JSON.stringify(patch),
    );
  }
});

test('all external performance fields share the same bounded freshness contract', () => {
  for (const patch of [
    { expires_at: now / 1000 + 600 },
    { fetched_at: now / 1000 + 1 },
    { fetched_at: NaN },
    { expires_at: Infinity },
    { fetched_at: '1800000000' },
    { expires_at: now / 1000 },
  ]) {
    const result = calculate({
      performance_reference: { ...reference, ...patch },
    });
    assert.equal(result.externalLatency, false);
    assert.equal(result.externalThroughput, false);
  }
});

test('model-level OR fills a non-OR route independently per metric, never fabricating uptime', () => {
  const stats = {
    performance_reference: { ...reference, output_tps: null },
    market_speed_reference: {
      ...reference,
      ttft_ms: 9999,
      output_tps: 64,
      uptime_rate: 100,
      uptime_window_seconds: 86400,
    },
  };
  assert.equal(calculate(stats).latency.value, 1915);
  assert.equal(calculate(stats).throughput, 64);
  assert.equal(calculate({ ...stats, avg_output_tps: 0 }).throughput, 0);
  assert.equal(
    calculate({ ...stats, performance_reference: null }).latency.value,
    9999,
  );
  assert.equal(
    calculate({
      ...stats,
      market_speed_reference: { ...reference, expires_at: now / 1000 },
    }).throughput,
    undefined,
  );
  assert.equal(modelUptime(stats, now).external, false);
});
