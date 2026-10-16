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

import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import {
  formatHealthLatency,
  formatHealthRate,
  getHealthLatency,
  healthTone,
  resolveGroupHealthMetric,
  selectHealthGroups,
} from './healthUtils.js';

describe('group health display semantics', () => {
  test('distinguishes missing samples from measured zero success or latency', () => {
    assert.equal(formatHealthRate(null), '—');
    assert.equal(formatHealthRate(undefined), '—');
    assert.equal(formatHealthRate(0), '0.00%');
    assert.equal(formatHealthLatency(null), '—');
    assert.equal(formatHealthLatency(NaN), '—');
    assert.equal(formatHealthLatency(-1), '—');
    assert.equal(formatHealthLatency(0), '0 ms');
    assert.equal(formatHealthLatency(1234), '1.23 s');
  });

  test('does not label low-volume 100% traffic as stable or empty traffic as failed', () => {
    assert.equal(healthTone({ request_count: 0, success_rate: null }), 'empty');
    assert.equal(
      healthTone({ request_count: 29, success_rate: 100 }),
      'limited',
    );
    assert.equal(
      healthTone({ request_count: 30, success_rate: 99 }),
      'healthy',
    );
    assert.equal(
      healthTone({ request_count: 30, success_rate: 98.99 }),
      'warning',
    );
    assert.equal(
      healthTone({ request_count: 30, success_rate: 90 }),
      'warning',
    );
    assert.equal(
      healthTone({ request_count: 30, success_rate: 89.99 }),
      'degraded',
    );
    assert.equal(
      healthTone({ request_count: 30, success_rate: 0 }),
      'degraded',
    );
  });

  test('keeps first-output and completed-image populations independent', () => {
    const stats = {
      avg_ttft_ms: 900,
      ttft_count: 45,
      avg_completion_ms: 5000,
      completion_count: 20,
    };
    assert.equal(resolveGroupHealthMetric(stats, 'OpenAI · 优质'), 'ttft');
    assert.equal(resolveGroupHealthMetric(stats, 'gpt-image-2'), 'completion');
    assert.equal(resolveGroupHealthMetric(null, 'OpenAI · 生图'), 'completion');
    assert.equal(
      resolveGroupHealthMetric({ completion_count: 10 }),
      'completion',
    );
    assert.deepEqual(getHealthLatency(stats, 'ttft'), {
      value: 900,
      count: 45,
    });
    assert.deepEqual(getHealthLatency(stats, 'completion'), {
      value: 5000,
      count: 20,
    });
    assert.deepEqual(getHealthLatency({ ttft_count: 4 }, 'completion'), {
      value: undefined,
      count: 0,
    });
  });
});

describe('group health visibility', () => {
  const groups = ['standard', 'premium', 'private', 'auto'].map((group) => ({
    group,
    request_count: 100,
    success_rate: 97.25,
  }));
  const models = [
    {
      model_name: 'text-model',
      enable_groups: ['standard', 'private', 'auto'],
    },
    { model_name: 'image-model', enable_groups: ['premium'] },
  ];
  const usableGroup = {
    standard: 'Standard',
    premium: 'Premium',
    auto: 'Auto',
  };

  test('shows allowed groups including outages, excluding private and auto groups', () => {
    assert.deepEqual(
      selectHealthGroups({ groups, models, usableGroup }).map(
        ({ group }) => group,
      ),
      ['standard', 'premium'],
    );
    assert.deepEqual(
      selectHealthGroups({ groups, models, usableGroup: {} }),
      [],
    );
    assert.deepEqual(
      selectHealthGroups({ groups, models: [], usableGroup }).map(
        ({ group }) => group,
      ),
      ['standard', 'premium'],
    );
  });

  test('an exact model or group filter cannot inherit another model group', () => {
    // Exact-model API results have zero counts for groups without this model.
    const modelGroups = groups.map((stats) =>
      stats.group === 'premium'
        ? { ...stats, request_count: 0, success_rate: null }
        : stats,
    );
    const exactModel = selectHealthGroups({
      groups: modelGroups,
      models: [models[0]],
      usableGroup,
      exactModel: true,
    });
    assert.deepEqual(
      exactModel.map(({ group }) => group),
      ['standard'],
    );
    assert.deepEqual(
      selectHealthGroups({
        groups: modelGroups,
        models: [models[0]],
        usableGroup,
        filterGroup: 'premium',
        exactModel: true,
      }),
      [],
    );
    assert.deepEqual(
      selectHealthGroups({
        groups,
        models,
        usableGroup,
        filterGroup: 'premium',
      }).map(({ group }) => group),
      ['premium'],
    );
    // The backend's weighted aggregate is retained; the view never averages rates.
    assert.equal(exactModel[0], groups[0]);
  });

  test('preserves recorded failures when all channels for that model are disabled', () => {
    const outage = { group: 'standard', request_count: 50, success_rate: 0 };
    assert.deepEqual(
      selectHealthGroups({
        groups: [outage],
        models: [],
        usableGroup,
        exactModel: true,
      }),
      [outage],
    );
    assert.deepEqual(
      selectHealthGroups({
        groups: [{ ...outage, group: 'private' }],
        models: [],
        usableGroup,
        exactModel: true,
      }),
      [],
    );
  });

  test('filtered aggregates use model relationships, never unrelated request counts', () => {
    const stats = [
      { group: 'standard', request_count: 0 },
      { group: 'premium', request_count: 100 },
      { group: 'shared', request_count: 80 },
      { group: 'private', request_count: 100 },
      { group: 'auto', request_count: 100 },
    ];
    const matchingModels = [
      { enable_groups: ['standard', 'shared', 'private', 'auto'] },
    ];
    const result = selectHealthGroups({
      groups: stats,
      models: matchingModels,
      usableGroup: { ...usableGroup, shared: '' },
      filterByModels: true,
    });
    assert.deepEqual(result, [stats[0], stats[2]]);
    assert.equal(result[1], stats[2]); // Retain the server aggregate unchanged.
    assert.deepEqual(
      selectHealthGroups({
        groups: stats,
        models: matchingModels,
        usableGroup,
        filterGroup: 'premium',
        filterByModels: true,
      }),
      [],
    );
  });

  test('zero matching models stays empty until filters are reset', () => {
    assert.deepEqual(
      selectHealthGroups({
        groups,
        models: [],
        usableGroup,
        filterByModels: true,
      }),
      [],
    );
    assert.deepEqual(
      selectHealthGroups({ groups, models: [], usableGroup }).map(
        ({ group }) => group,
      ),
      ['standard', 'premium'],
    );
  });

  test('exact-model samples still preserve outages under a sidebar filter', () => {
    const outage = { group: 'premium', request_count: 50, success_rate: 0 };
    assert.deepEqual(
      selectHealthGroups({
        groups: [outage],
        models: [models[0]],
        usableGroup,
        exactModel: true,
        filterByModels: true,
      }),
      [outage],
    );
  });
});
