import { describe, test, expect } from 'bun:test';
import {
  formatModelContext,
  formatModelContextExact,
  formatModelThroughput,
  formatModelCardPrice,
  shouldNavigateModelCard,
  selectComparisonGroups,
  chartMetricValue,
} from '../src/helpers/modelPresentation.js';

describe('model presentation interactions and missing metrics', () => {
  test('card money uses two decimals without double rounding or hiding a nonzero rate', () => {
    expect(formatModelCardPrice(0.3, '¥')).toBe('¥0.30');
    expect(formatModelCardPrice(1.8, '¥')).toBe('¥1.80');
    expect(formatModelCardPrice(1.23496, '$')).toBe('$1.23');
    expect(formatModelCardPrice(1.235, '$')).toBe('$1.24');
    expect(formatModelCardPrice(1.005, '€')).toBe('€1.01');
    expect(formatModelCardPrice(0, '¥')).toBe('¥0.00');
    expect(formatModelCardPrice(0.0003, '¥')).toBe('<¥0.01');
    expect(formatModelCardPrice(null)).toBeNull();
    expect(formatModelCardPrice(NaN)).toBeNull();
    expect(formatModelCardPrice(-1)).toBeNull();
  });
  test('context is compact without inflating capacity and retains an exact tooltip', () => {
    expect(formatModelContext(1_050_000)).toBe('1M');
    expect(formatModelContext(1_500_000)).toBe('1.5M');
    expect(formatModelContext(200_000)).toBe('200K');
    expect(formatModelContext(262_144)).toBe('262K');
    expect(formatModelContext(32_768)).toBe('32K');
    expect(formatModelContext(999)).toBe('999 tokens');
    expect(formatModelContextExact(1_050_000)).toBe(
      (1_050_000).toLocaleString() + ' tokens',
    );
    expect(formatModelContextExact(0)).toBeUndefined();
  });
  test('card background navigates; copy, selection, nested links and text selection do not', () => {
    const event = (nested = false) => ({
      button: 0,
      defaultPrevented: false,
      target: { closest: () => nested },
    });
    expect(shouldNavigateModelCard(event())).toBe(true);
    expect(shouldNavigateModelCard(event(true))).toBe(false);
    expect(shouldNavigateModelCard(event(), 'selected model description')).toBe(
      false,
    );
    expect(shouldNavigateModelCard({ ...event(), button: 2 })).toBe(false);
    expect(
      shouldNavigateModelCard({ ...event(), defaultPrevented: true }),
    ).toBe(false);
  });
  test('comparison removes unknown/duplicate groups and caps plotted series', () => {
    expect(
      selectComparisonGroups(
        ['a', 'b', 'c', 'd', 'e'],
        ['private', 'a', 'a', 'b', 'c', 'd', 'e'],
      ),
    ).toEqual(['a', 'b', 'c', 'd']);
    expect(selectComparisonGroups([], ['a'])).toEqual([]);
  });
  test('unknown historic metrics stay absent; explicit zero tokens is valid', () => {
    expect(chartMetricValue({ total_tokens: null }, 'total_tokens')).toBe(null);
    expect(
      chartMetricValue(
        { total_tokens: 0, usage_sample_count: 2 },
        'total_tokens',
      ),
    ).toBe(0);
    expect(
      chartMetricValue(
        { request_count: 0, observed_count: 0 },
        'request_count',
      ),
    ).toBe(null);
    expect(
      chartMetricValue(
        { request_count: 0, observed_count: 2 },
        'request_count',
      ),
    ).toBe(0);
    expect(chartMetricValue({ avg_output_tps: -1 }, 'avg_output_tps')).toBe(
      null,
    );
    expect(formatModelThroughput(null)).toBe('—');
    expect(formatModelThroughput(0)).toBe('0.0 tok/s');
    expect(formatModelContext(0)).toBe('—');
    expect(formatModelContext(1.5)).toBe('—');
    expect(formatModelContext(262144)).toContain('262');
  });
});
