import { test, expect } from 'bun:test';
import {
  filterOpenRouterActivity,
  openRouterUSD,
} from '../src/pages/UpstreamAccounts/openRouterFinance.js';

test('USD preserves zero and negative balance but leaves missing values unknown', () => {
  expect(openRouterUSD(0)).toBe('$0.000000');
  expect(openRouterUSD(-1.25)).toBe('$-1.250000');
  for (const value of [null, undefined, NaN, Infinity])
    expect(openRouterUSD(value)).toBe('--');
});
test('date and model/provider filters intersect without adding BYOK external costs', () => {
  const data = [
    {
      date: '2026-11-01',
      model: 'A',
      provider_name: 'OpenAI',
      usage: 2,
      byok_usage_inference: 10,
    },
    { date: '2026-10-31', model: 'B', provider_name: 'OpenAI', usage: 3 },
    { date: '2026-11-01', model: 'B', provider_name: 'Other', usage: 1 },
  ];
  expect(filterOpenRouterActivity(data, ' openAI ', '2026-11-01')).toEqual([
    data[0],
  ]);
  expect(filterOpenRouterActivity(data, 'b')).toEqual([data[1], data[2]]);
  expect(filterOpenRouterActivity(data, 'missing')).toEqual([]);
  expect(filterOpenRouterActivity(null)).toEqual([]);
  expect(data[0].usage).toBe(2);
});
