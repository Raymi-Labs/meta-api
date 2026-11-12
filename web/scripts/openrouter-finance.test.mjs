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

import { summarizeOpenRouterConsumption } from '../src/pages/UpstreamAccounts/openRouterFinance.js';
test('model comparison unions sources, preserves unknown sides, and applies shared UTC filters', () => {
  const data = {
    activity: [
      {
        date: '2026-11-01',
        model: 'a',
        usage: 2,
        requests: 2,
        byok_usage_inference: 10,
      },
      { date: '2026-11-01', model: 'a', usage: 1, requests: 1 },
      { date: '2026-11-02', model: 'b', usage: 0, requests: 1 },
    ],
    site_activity: [
      { date: '2026-11-01', model: 'a', quota: 50, requests: 2 },
      { date: '2026-11-02', model: 'local-only', quota: 20, requests: 1 },
    ],
  };
  const rows = summarizeOpenRouterConsumption(data);
  expect(rows.map((r) => [r.model, r.usage, r.quota])).toEqual([
    ['a', 3, 50],
    ['b', 0, null],
    ['local-only', null, 20],
  ]);
  expect(rows[0].byok).toBe(10);
  const daily = summarizeOpenRouterConsumption(data, { daily: true });
  expect(daily.reduce((sum, r) => sum + r.quota, 0)).toBe(70);
  expect(daily.reduce((sum, r) => sum + r.usage, 0)).toBe(3);
  expect(
    summarizeOpenRouterConsumption(data, { end: '2026-11-01', search: ' A ' }),
  ).toHaveLength(1);
  expect(
    summarizeOpenRouterConsumption({ site_activity: data.site_activity })[0]
      .usage,
  ).toBeNull();
});
