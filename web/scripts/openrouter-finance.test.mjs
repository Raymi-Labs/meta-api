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

import { summarizeOpenRouterChannels } from '../src/pages/UpstreamAccounts/openRouterFinance.js';
test('channel groups keep identical names separate and match date totals', () => {
  const data = {
    channel_activity: [
      {
        date: '2026-11-01',
        channel_id: 1,
        channel_name: 'same',
        quota: 10,
        requests: 1,
      },
      {
        date: '2026-11-02',
        channel_id: 1,
        channel_name: 'same',
        quota: 20,
        requests: 2,
      },
      {
        date: '2026-11-01',
        channel_id: 2,
        channel_name: 'same',
        quota: 30,
        requests: 3,
      },
    ],
  };
  const rows = summarizeOpenRouterChannels(data);
  expect(rows.map((r) => [r.key, r.quota, r.site_requests])).toEqual([
    [1, 30, 3],
    [2, 30, 3],
  ]);
  expect(
    summarizeOpenRouterChannels(data, { end: '2026-11-01' }).reduce(
      (s, r) => s + r.quota,
      0,
    ),
  ).toBe(40);
  expect(summarizeOpenRouterChannels(data, { search: '#2' })).toHaveLength(1);
  expect(summarizeOpenRouterChannels(null)).toEqual([]);
});

import {
  openRouterUsageCNY,
  openRouterQuotaCNY,
} from '../src/pages/UpstreamAccounts/openRouterFinance.js';
import { cashCSV } from '../src/pages/UpstreamAccounts/cashFinance.js';
test('comparison CNY uses confirmed fixed rates and rounds only final values', () => {
  expect(openRouterUsageCNY(261.387777)).toBe('¥1751.298106');
  expect(openRouterQuotaCNY(4258180218, 500000, 1)).toBe('¥1703.272087');
  expect(openRouterQuotaCNY(500000, 500000, 2)).toBe('¥0.400000');
  expect(openRouterQuotaCNY(100, 100, 5)).toBe('¥1.000000');
  expect(openRouterUsageCNY(0)).toBe('¥0.000000');
  expect(openRouterQuotaCNY(0, 500000, 1)).toBe('¥0.000000');
  for (const value of [null, undefined, NaN, Infinity, '10']) {
    expect(openRouterUsageCNY(value)).toBe('--');
    expect(openRouterQuotaCNY(value, 500000, 1)).toBe('--');
  }
  for (const invalid of [0, -1, NaN, Infinity]) {
    expect(openRouterQuotaCNY(1, invalid, 1)).toBe('--');
    expect(openRouterQuotaCNY(1, 500000, invalid)).toBe('--');
  }
  const csv = cashCSV(
    [{ name: 'test', quota: 4258180218, requests: 84549 }],
    (v) => v,
    (q) => openRouterQuotaCNY(q, 500000, 1),
    '本站消耗 (CNY)',
  );
  expect(csv).toContain('本站消耗 (CNY)');
  expect(csv).toContain('¥1703.272087');
});
