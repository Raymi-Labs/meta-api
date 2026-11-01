import assert from 'node:assert/strict';
import { test } from 'bun:test';
import { modelPerformance } from '../src/helpers/modelPerformance.js';
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
test('same-scope historical TTFT wins; external TTFT cannot fill completion latency', () => {
  const display = calculate({
    last_valid: { ttft: { value: 3250, ts: now / 1000 - 3600 } },
  });
  assert.equal(display.latency.value, 3250);
  assert.equal(display.externalLatency, false);
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
  const display = calculate(stats);
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
