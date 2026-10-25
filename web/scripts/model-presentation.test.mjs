import { describe, test, expect } from 'bun:test';
import {
  formatModelContext,
  formatModelThroughput,
  shouldNavigateModelCard,
  selectComparisonGroups,
  chartMetricValue,
} from '../src/helpers/modelPresentation.js';

describe('model presentation interactions and missing metrics', () => {
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
