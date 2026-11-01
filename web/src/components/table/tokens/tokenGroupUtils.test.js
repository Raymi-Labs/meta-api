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
  TOKEN_GROUP_OTHER_VENDOR,
  tokenGroupPriceLabel,
  filterTokenGroupsByHealth,
  buildTokenGroupVendorOptions,
  filterTokenGroupsByVendor,
  formatTokenGroupSelectedLabel,
  prioritizeTokenGroups,
  resolveTokenGroupVendor,
  shouldClearTokenGroupForVendor,
  sortTokenGroupOptions,
  sortTokenGroupsByRatio,
} from './tokenGroupUtils.js';

describe('tokenGroupUtils', () => {
  test('pins exact priority groups without changing other candidates or input', () => {
    const groups = Object.freeze(
      [
        { value: 'OpenAI · 福利', ratio: 9 },
        { value: '福利', ratio: 5 },
        { value: '企业专属', label: '企业级', ratio: 8 },
        { value: '优选', ratio: 0 },
        { value: '企业级', ratio: null },
        { value: 'default', ratio: 1 },
      ].map(Object.freeze),
    );
    assert.deepEqual(prioritizeTokenGroups(groups), [
      groups[4],
      groups[3],
      groups[1],
      groups[0],
      groups[2],
      groups[5],
    ]);
    assert.deepEqual(prioritizeTokenGroups(sortTokenGroupsByRatio(groups)), [
      groups[4],
      groups[3],
      groups[1],
      groups[0],
      groups[2],
      groups[5],
    ]);
  });

  test('never adds unavailable groups and preserves the rest of each existing order', () => {
    assert.deepEqual(prioritizeTokenGroups(), []);
    const groups = [
      { value: 'auto', ratio: '自动' },
      { value: 'low', ratio: 0.1 },
      { value: '福利', ratio: 0 },
      { value: 'high', ratio: 5 },
    ];
    assert.deepEqual(prioritizeTokenGroups(groups), [
      groups[2],
      groups[0],
      groups[1],
      groups[3],
    ]);
    assert.deepEqual(prioritizeTokenGroups(sortTokenGroupsByRatio(groups)), [
      groups[2],
      groups[3],
      groups[1],
      groups[0],
    ]);
  });

  test('ratio sort is descending, stable, non-mutating and handles unknown values', () => {
    const groups = [
      { value: 'auto', ratio: 99 },
      { value: 'unknown', ratio: null },
      { value: 'low', ratio: 0.4 },
      { value: 'same-a', ratio: 0.55 },
      { value: 'zero', ratio: 0 },
      { value: 'high', ratio: 0.68 },
      { value: 'same-b', ratio: 0.55 },
      { value: 'string', ratio: '0.9' },
      { value: 'negative', ratio: -1 },
      { value: 'nan', ratio: NaN },
      { value: 'infinite', ratio: Infinity },
    ];
    const original = groups.slice();
    assert.deepEqual(
      sortTokenGroupsByRatio(groups).map((g) => g.value),
      [
        'high',
        'same-a',
        'same-b',
        'low',
        'zero',
        'auto',
        'unknown',
        'string',
        'negative',
        'nan',
        'infinite',
      ],
    );
    assert.deepEqual(groups, original);
  });
  test('resolves configured and legacy vendor group names', () => {
    assert.equal(resolveTokenGroupVendor('OpenAI · 优质'), 'OpenAI');
    assert.equal(resolveTokenGroupVendor('Claude · 企业专属'), 'Claude');
    assert.equal(
      resolveTokenGroupVendor('Deepseek · 优质（第三方）'),
      'DeepSeek',
    );
    assert.equal(resolveTokenGroupVendor('gemini'), 'Gemini');
    assert.equal(resolveTokenGroupVendor('Anthropic · 优质'), 'Anthropic');
  });

  test('keeps unscoped and automatic groups under the fallback vendor', () => {
    assert.equal(resolveTokenGroupVendor('官方渠道'), TOKEN_GROUP_OTHER_VENDOR);
    assert.equal(resolveTokenGroupVendor('default'), TOKEN_GROUP_OTHER_VENDOR);
    assert.equal(resolveTokenGroupVendor('auto'), TOKEN_GROUP_OTHER_VENDOR);
    assert.equal(resolveTokenGroupVendor(''), '');
  });

  test('Inception and Mercury group aliases share one facet without changing group values', () => {
    const groups = [
      'Inception · 标准',
      'Mercury · 优质',
      ' inception labs ',
      'InceptionLabs·企业专属',
      'default',
    ].map((value) => ({ value }));
    const original = groups.map((group) => group.value);
    for (const group of groups.slice(0, 4))
      assert.equal(resolveTokenGroupVendor(group.value), 'Inception');
    assert.equal(
      resolveTokenGroupVendor('notinception'),
      TOKEN_GROUP_OTHER_VENDOR,
    );
    assert.deepEqual(buildTokenGroupVendorOptions(groups), [
      { value: TOKEN_GROUP_OTHER_VENDOR, label: '多厂商通用 · 1' },
      { value: 'Inception', label: 'Inception · 4' },
    ]);
    assert.deepEqual(
      filterTokenGroupsByVendor(groups, 'Inception').map(
        (group) => group.value,
      ),
      original.slice(0, 4),
    );
    assert.deepEqual(
      groups.map((group) => group.value),
      original,
    );
  });

  test('builds compact vendor options and puts common vendor first', () => {
    assert.deepEqual(
      buildTokenGroupVendorOptions(
        [
          { value: 'OpenAI · 优质' },
          { value: 'OpenAI · 企业专属' },
          { value: 'Claude · 优质' },
          { value: 'default' },
        ],
        '多厂商通用',
      ),
      [
        { value: TOKEN_GROUP_OTHER_VENDOR, label: '多厂商通用 · 1' },
        { value: 'OpenAI', label: 'OpenAI · 2' },
        { value: 'Claude', label: 'Claude · 1' },
      ],
    );
  });

  test('sorts vendors and service tiers in a stable business order', () => {
    const groups = [
      { value: 'Claude · 特价（第三方）' },
      { value: 'MiMo · 企业专属' },
      { value: 'OpenAI · 优质（第三方）' },
      { value: 'default' },
      { value: 'OpenAI · 企业专属' },
      { value: 'Claude · 优质' },
      { value: 'OpenAI · 优质' },
      { value: 'Deepseek · 标准（第三方）' },
    ];

    assert.deepEqual(
      sortTokenGroupOptions(groups).map((group) => group.value),
      [
        'OpenAI · 企业专属',
        'OpenAI · 优质',
        'OpenAI · 优质（第三方）',
        'Claude · 优质',
        'Claude · 特价（第三方）',
        'Deepseek · 标准（第三方）',
        'MiMo · 企业专属',
        'default',
      ],
    );
  });

  test('builds vendor options in the same fixed order', () => {
    const groups = [
      { value: 'MiMo · 优质' },
      { value: 'Claude · 优质' },
      { value: 'OpenAI · 优质' },
      { value: 'default' },
      { value: 'Gemini · 优质' },
    ];

    assert.deepEqual(
      buildTokenGroupVendorOptions(groups).map((item) => item.value),
      [TOKEN_GROUP_OTHER_VENDOR, 'OpenAI', 'Claude', 'MiMo', 'Gemini'],
    );
  });

  test('keeps Gemini last even with unknown vendors and shuffled input', () => {
    const names = [
      'gemini',
      'MiMo · 优质',
      '测试勿选',
      'Kimi · 优质',
      'Grok · 优质',
      'Deepseek · 优质',
      'Claude · 优质',
      'OpenAI · 优质',
      'Custom · 优质',
      'Gemini · 企业专属',
    ];
    const groups = names.map((value) => Object.freeze({ value, ratio: 0.45 }));
    Object.freeze(groups);
    const expected = [
      TOKEN_GROUP_OTHER_VENDOR,
      'OpenAI',
      'Claude',
      'Grok',
      'Kimi',
      'DeepSeek',
      'MiMo',
      'Custom',
      'Gemini',
    ];
    for (let offset = 0; offset < groups.length; offset++) {
      const shuffled = [...groups.slice(offset), ...groups.slice(0, offset)];
      assert.deepEqual(
        buildTokenGroupVendorOptions(shuffled).map((option) => option.value),
        expected,
      );
      assert.deepEqual(
        buildTokenGroupVendorOptions(shuffled.reverse()).map(
          (option) => option.value,
        ),
        expected,
      );
    }
    const options = buildTokenGroupVendorOptions(groups);
    assert.equal(options[0].label, '多厂商通用 · 1');
    assert.equal(options.at(-1).label, 'Gemini · 2');
    assert.deepEqual(
      groups.map((group) => group.value),
      names,
    );
    assert.ok(groups.every((group) => group.ratio === 0.45));
  });

  test('does not manufacture absent vendors or groups', () => {
    assert.deepEqual(buildTokenGroupVendorOptions(), []);
    assert.deepEqual(buildTokenGroupVendorOptions([{ value: '' }]), []);
    assert.deepEqual(buildTokenGroupVendorOptions([{ value: 'gemini' }]), [
      { value: 'Gemini', label: 'Gemini · 1' },
    ]);
    assert.deepEqual(
      buildTokenGroupVendorOptions([{ value: 'default' }], 'Multi-provider'),
      [{ value: TOKEN_GROUP_OTHER_VENDOR, label: 'Multi-provider · 1' }],
    );
  });

  test('retains fallback group membership and existing token selection', () => {
    const groups = ['default', '企业专属', 'group_1', '测试', '测试勿选'].map(
      (value) => ({ value }),
    );
    assert.deepEqual(
      filterTokenGroupsByVendor(groups, TOKEN_GROUP_OTHER_VENDOR),
      groups,
    );
    groups.forEach(({ value }) => {
      assert.equal(resolveTokenGroupVendor(value), TOKEN_GROUP_OTHER_VENDOR);
      assert.equal(
        shouldClearTokenGroupForVendor(value, TOKEN_GROUP_OTHER_VENDOR),
        false,
      );
    });
  });

  test('filters groups without changing their configured order', () => {
    const groups = [
      { value: 'Claude · 企业专属' },
      { value: 'OpenAI · 优质' },
      { value: 'Claude · 优质' },
    ];

    assert.deepEqual(filterTokenGroupsByVendor(groups, 'Claude'), [
      { value: 'Claude · 企业专属' },
      { value: 'Claude · 优质' },
    ]);
    assert.deepEqual(filterTokenGroupsByVendor(groups, ''), []);
  });

  test('optionally includes common groups after the selected vendor', () => {
    const groups = sortTokenGroupOptions([
      { value: '企业专属' },
      { value: 'OpenAI · 优质' },
      { value: 'Claude · 优质' },
      { value: 'default' },
    ]);

    assert.deepEqual(
      filterTokenGroupsByVendor(groups, 'OpenAI', true).map(
        (group) => group.value,
      ),
      ['OpenAI · 优质', 'default', '企业专属'],
    );
  });

  test('clears only groups that do not belong to the newly selected vendor', () => {
    assert.equal(
      shouldClearTokenGroupForVendor('Claude · 优质', 'OpenAI'),
      true,
    );
    assert.equal(
      shouldClearTokenGroupForVendor('Claude · 优质', 'Claude'),
      false,
    );
    assert.equal(shouldClearTokenGroupForVendor('', 'Claude'), false);
  });

  test('formats selected groups with numeric and automatic ratios', () => {
    assert.equal(
      formatTokenGroupSelectedLabel({
        value: 'Claude · 优质',
        ratio: 1,
      }),
      'Claude · 优质 · 1x',
    );
    assert.equal(
      formatTokenGroupSelectedLabel({ value: 'auto', ratio: '自动' }),
      'auto · 自动',
    );
    assert.equal(
      formatTokenGroupSelectedLabel({ value: 'default' }),
      'default',
    );
  });
});

test('official price comparison uses each current ratio rather than group names', () => {
  const t = (key, values) => key.replace('{{percent}}', values?.percent);
  for (const name of [
    '企业级',
    '优选',
    '福利',
    '自定义分组',
    'OpenAI · 优质',
  ]) {
    for (const [ratio, label] of [
      [0.38, '比官方价格低62%'],
      [0.6, '比官方价格低40%'],
      [0.8, '比官方价格低20%'],
      [1, '与官方价格一致'],
      [0, '比官方价格低100%'],
      [1.2, '比官方价格高20%'],
      [0.333333, '比官方价格低66.67%'],
    ])
      assert.equal(tokenGroupPriceLabel({ value: name, ratio }, t), label);
  }
  for (const ratio of [
    undefined,
    null,
    '',
    '0.38',
    -1,
    NaN,
    Infinity,
    Number.MAX_VALUE,
  ])
    assert.equal(tokenGroupPriceLabel({ value: '福利', ratio }, t), '');
  assert.equal(tokenGroupPriceLabel({ value: 'auto', ratio: 0.38 }, t), '');
  assert.equal(tokenGroupPriceLabel({ value: '福利', ratio: 0.999999 }, t), '');
  assert.equal(tokenGroupPriceLabel(undefined, t), '');
});

describe('token group Uptime candidates', () => {
  const groups = ['down', 'healthy', 'unknown', 'auto'].map((value) => ({
    value,
  }));
  const snapshot = (stats, extra = {}) => ({
    data: { enabled: true, groups: [{ group: 'down', ...stats }], ...extra },
  });
  const values = (health, selected) =>
    filterTokenGroupsByHealth(groups, health, selected).map(
      ({ value }) => value,
    );
  test('hides exact zero Uptime with real samples, keeps partial success and unknown', () => {
    assert.deepEqual(
      values(snapshot({ uptime_sample_count: 1, uptime_rate: 0 })),
      ['healthy', 'unknown', 'auto'],
    );
    for (const stats of [
      { uptime_sample_count: 0, uptime_rate: 0 },
      { uptime_sample_count: 1, uptime_rate: null },
      { uptime_sample_count: 1000000, uptime_rate: 0.0001 },
      {
        uptime_sample_count: 2,
        uptime_rate: 50,
        request_count: 1,
        success_rate: 0,
      },
      {
        uptime_sample_count: 0,
        uptime_rate: null,
        request_count: 2,
        success_rate: 0,
      },
      { uptime_sample_count: NaN, uptime_rate: 0 },
    ])
      assert.equal(values(snapshot(stats)).length, 4);
  });
  test('supports older request-only response without inventing missing samples', () => {
    assert.equal(
      values(snapshot({ request_count: 1, success_rate: 0 })).length,
      3,
    );
    assert.equal(
      values(snapshot({ request_count: 0, success_rate: 0 })).length,
      4,
    );
    assert.equal(values(snapshot({ success_rate: 0 })).length, 4);
  });
  test('unavailable and incomplete health fail open, recovery restores candidates', () => {
    const down = snapshot({ uptime_sample_count: 1, uptime_rate: 0 });
    for (const health of [
      undefined,
      { loading: true },
      { ...down, error: Error('offline') },
      snapshot({ uptime_sample_count: 1, uptime_rate: 0 }, { enabled: false }),
      snapshot(
        { uptime_sample_count: 1, uptime_rate: 0 },
        { collection_incomplete: true },
      ),
      snapshot({ uptime_sample_count: 1, uptime_rate: 100 }),
    ])
      assert.equal(values(health).length, 4);
    assert.equal(values({ ...down, loading: true }).length, 3);
  });
  test('preserves selected routes within permission catalog, without mutating either input', () => {
    const selected = ['unknown', 'down', 'not-authorized'];
    const before = structuredClone(groups);
    const down = snapshot({ uptime_sample_count: 10, uptime_rate: 0 });
    assert.deepEqual(
      values(down, selected),
      groups.map(({ value }) => value),
    );
    assert.deepEqual(selected, ['unknown', 'down', 'not-authorized']);
    assert.deepEqual(groups, before);
    assert.equal(values(down, []).includes('down'), false);
  });
});
