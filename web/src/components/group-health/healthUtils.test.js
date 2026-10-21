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
  getHealthCounts,
  getHealthLatency,
  healthTone,
  isHealthSampleLimited,
  resolveGroupHealthMetric,
  selectHealthGroups,
} from './healthUtils.js';

describe('group health display semantics', () => {
  test('keeps disconnections inside unknown failures and exclusions outside the denominator', () => {
    const stats = Object.freeze({
      request_count: 7,
      success_count: 1,
      failure_count: 6,
      service_failure_count: 2,
      unknown_failure_count: 4,
      client_canceled_count: 3,
      excluded_user_error_count: 5,
      excluded_count: 5,
      observed_count: 12,
      success_rate: (1 / 7) * 100,
    });
    assert.deepEqual(getHealthCounts(stats), {
      request: 7,
      success: 1,
      failure: 6,
      service: 2,
      unknown: 4,
      disconnected: 3,
      excludedUser: 5,
      excluded: 5,
      observed: 12,
    });
    assert.equal(formatHealthRate(stats.success_rate), '14.29%');
    assert.equal(healthTone(stats), 'degraded');
    assert.equal(isHealthSampleLimited(stats), true);
  });

  test('treats historical failures without classification as unknown, never as user errors', () => {
    assert.deepEqual(
      getHealthCounts({
        request_count: 10,
        success_count: 2,
        failure_count: 8,
      }),
      {
        request: 10,
        success: 2,
        failure: 8,
        service: 0,
        unknown: 8,
        disconnected: 0,
        excludedUser: 0,
        excluded: 0,
        observed: 10,
      },
    );
  });

  test('preserves observed excluded-only calls without reporting a success rate or limited samples', () => {
    const stats = {
      request_count: 0,
      success_count: 0,
      failure_count: 0,
      excluded_user_error_count: 40,
      excluded_count: 40,
      observed_count: 40,
      success_rate: null,
    };
    assert.equal(getHealthCounts(stats).observed, 40);
    assert.equal(getHealthCounts(stats).request, 0);
    assert.equal(healthTone(stats), 'empty');
    assert.equal(isHealthSampleLimited(stats), false);
    assert.equal(formatHealthRate(stats.success_rate), '—');
    assert.equal(getHealthCounts(undefined).observed, 0);
  });

  test('uses the same measured-rate boundaries for every positive sample size', () => {
    for (const request_count of [1, 7, 29, 30, 99, 100, 101, 10000]) {
      for (const [success_rate, expected] of [
        [100, 'healthy'],
        [95, 'healthy'],
        [94.99, 'warning'],
        [80, 'warning'],
        [79.99, 'degraded'],
        [0, 'degraded'],
      ]) {
        const stats = Object.freeze({ request_count, success_rate });
        assert.equal(healthTone(stats), expected);
        assert.equal(stats.success_rate, success_rate);
        assert.equal(
          formatHealthRate(stats.success_rate),
          `${success_rate.toFixed(2)}%`,
        );
      }
    }
    assert.equal(
      healthTone({ request_count: '7', success_rate: '80' }),
      'warning',
    );
  });

  test('keeps absent, zero, and invalid samples gray rather than marking them healthy', () => {
    for (const stats of [
      undefined,
      {},
      { request_count: null },
      { request_count: 0, success_rate: null },
      { request_count: 0, success_rate: 100 },
      { request_count: '0', success_rate: 0 },
      { request_count: -1 },
      { request_count: NaN },
      { request_count: Infinity, success_rate: 100 },
      { request_count: -Infinity, success_rate: 0 },
      { request_count: 'invalid', success_rate: 100 },
      { request_count: 7 },
      { request_count: 7, success_rate: null },
      { request_count: 7, success_rate: NaN },
      { request_count: 100, success_rate: null },
      { request_count: 100, success_rate: NaN },
      { request_count: 100, success_rate: Infinity },
      { request_count: 100, success_rate: -Infinity },
      { request_count: 100, success_rate: -0.01 },
      { request_count: 100, success_rate: 100.01 },
      { request_count: 100, success_rate: 'invalid' },
    ]) {
      assert.equal(healthTone(stats), 'empty');
      assert.equal(isHealthSampleLimited(stats), false);
    }
  });

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

  test('reports sample limitations separately from the measured health tone', () => {
    for (const request_count of [1, 7, 29, '29']) {
      for (const success_rate of [0, 80, 95, 100]) {
        assert.equal(
          isHealthSampleLimited({ request_count, success_rate }),
          true,
        );
      }
    }
    for (const request_count of [30, 99, 100, 10000]) {
      assert.equal(
        isHealthSampleLimited({ request_count, success_rate: 0 }),
        false,
      );
    }
  });

  test('shows the reported Kimi daily and hourly failures with the same tone', () => {
    const daily = Object.freeze({
      request_count: 201,
      success_count: 65,
      success_rate: (65 / 201) * 100,
    });
    const hour = Object.freeze({
      request_count: 7,
      success_count: 1,
      success_rate: (1 / 7) * 100,
    });
    assert.equal(formatHealthRate(daily.success_rate), '32.34%');
    assert.equal(formatHealthRate(hour.success_rate), '14.29%');
    assert.equal(healthTone(daily), 'degraded');
    assert.equal(healthTone(hour), 'degraded');
    assert.equal(isHealthSampleLimited(daily), false);
    assert.equal(isHealthSampleLimited(hour), true);
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

  test('retains an allowed exact-model group with only excluded calls', () => {
    const excludedOnly = {
      group: 'premium',
      request_count: 0,
      excluded_user_error_count: 3,
      excluded_count: 3,
      observed_count: 3,
      success_rate: null,
    };
    assert.deepEqual(
      selectHealthGroups({
        groups: [excludedOnly],
        models: [],
        usableGroup,
        exactModel: true,
      }),
      [excludedOnly],
    );
    assert.deepEqual(
      selectHealthGroups({
        groups: [{ ...excludedOnly, group: 'private' }],
        models: [],
        usableGroup,
        exactModel: true,
      }),
      [],
    );
    assert.deepEqual(
      selectHealthGroups({
        groups: [excludedOnly],
        models: [models[0]],
        usableGroup,
        filterByModels: true,
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
