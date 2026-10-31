import { describe, test, expect, mock } from 'bun:test';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import {
  getUptimeRate,
  uptimeTone,
  healthTone,
  selectHealthGroups,
} from '../src/components/group-health/healthUtils';
import {
  chartDisplayValue,
  chartDomain,
  formatChartValue,
} from '../src/helpers/modelChartDisplay';

if (process.env.META_UPTIME_TEST === import.meta.url) {
  mock.module('@douyinfe/semi-ui', () => ({
    Button: ({ children }) => <button>{children}</button>,
    Spin: () => null,
    Tag: ({ children }) => <span>{children}</span>,
  }));
  mock.module('react-i18next', () => ({
    useTranslation: () => ({
      t: (key, args = {}) => key.replace(/{{(\w+)}}/g, (_, name) => args[name]),
    }),
  }));
  const { GroupHealthPanel } = await import('../src/components/group-health');
  const passing = {
    request_count: 0,
    success_rate: null,
    probe_count: 12,
    probe_success_count: 1,
    probe_bucket_count: 1,
    probe_success_bucket_count: 1,
    uptime_sample_count: 1,
    uptime_rate: 100,
  };
  describe('model uptime evidence', () => {
    test('probe-only rows show availability and source without inventing traffic', () => {
      expect(healthTone(passing)).toBe('empty');
      expect(uptimeTone(passing)).toBe('healthy');
      const html = renderToStaticMarkup(
        <GroupHealthPanel
          uptime
          stats={{
            ...passing,
            group: 'public',
            series: [{ ...passing, ts: 1800000000 }],
          }}
        />,
      );
      expect(html).toContain('100.00%');
      expect(html).toContain('模型测试 1 / 12 次通过');
      expect(html).toContain('测试补充 1 个时段');
      expect(html).toContain('group-health-bar-healthy');
      expect(html).not.toContain('该时段暂无调用记录');
      expect(html).not.toContain('计入统计 1 次');
      const requestView = renderToStaticMarkup(
        <GroupHealthPanel stats={{ ...passing, series: [] }} />,
      );
      expect(requestView).not.toContain('100.00%');
      expect(requestView).not.toContain('测试补充');
    });
    test('failed tests, no evidence and limited real requests remain distinct', () => {
      expect(
        uptimeTone({ ...passing, uptime_rate: 0, probe_success_count: 0 }),
      ).toBe('degraded');
      expect(
        uptimeTone({
          request_count: 0,
          probe_bucket_count: 0,
          uptime_rate: null,
        }),
      ).toBe('empty');
      expect(
        uptimeTone({
          ...passing,
          request_count: 2,
          success_rate: 0,
          uptime_rate: 33.333,
        }),
      ).toBe('limited');
      expect(
        uptimeTone({
          request_count: 30,
          success_rate: 0,
          probe_count: 100,
          probe_bucket_count: 0,
          uptime_rate: 0,
        }),
      ).toBe('degraded');
      expect(getUptimeRate({ request_count: 30, success_rate: 90 })).toBe(90);
      expect(getUptimeRate({ ...passing, uptime_rate: null })).toBeNull();
    });
    test('chart uses the same metric, units and missing-hour gaps', () => {
      expect(chartDisplayValue(passing, 'uptime_rate')).toBe(100);
      expect(
        chartDisplayValue({ uptime_rate: null }, 'uptime_rate'),
      ).toBeNull();
      expect(formatChartValue(100, 'uptime_rate')).toBe('100%');
      expect(chartDomain([100, 0], 'uptime_rate').max).toBe(100);
    });
    test('exact model retains probe failures only within authorized groups', () => {
      expect(
        selectHealthGroups({
          groups: [
            { ...passing, group: 'public' },
            { ...passing, group: 'private' },
          ],
          models: [],
          usableGroup: { public: 1 },
          exactModel: true,
        }).map((x) => x.group),
      ).toEqual(['public']);
    });
  });
} else {
  test('uptime component and metric regression in isolated process', () => {
    const child = spawnSync(
      process.execPath,
      ['test', fileURLToPath(import.meta.url)],
      {
        env: { ...process.env, META_UPTIME_TEST: import.meta.url },
        encoding: 'utf8',
      },
    );
    expect(child.status, child.stdout + child.stderr).toBe(0);
  });
}
