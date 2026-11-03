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
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { mock, test } from 'bun:test';

const Skeleton = ({ loading, children }) =>
  loading ? <span>loading-placeholder</span> : children;
Skeleton.Title = () => null;
mock.module('@douyinfe/semi-ui', () => ({
  Skeleton,
  Button: ({ children, icon, loading, theme, type, ...props }) => (
    <button {...props}>
      {icon}
      {children}
    </button>
  ),
}));
mock.module('react-i18next', () => ({
  useTranslation: () => ({ t: (key) => key }),
}));
const { default: Header } = await import(
  '../src/components/dashboard/DashboardHeader.jsx'
);
const { default: StatsCards } = await import(
  '../src/components/dashboard/StatsCards.jsx'
);
const { useDashboardStats } = await import(
  '../src/hooks/dashboard/useDashboardStats.jsx'
);
const t = (key) => key;
const render = (props = {}) => {
  const groupedStatsData = [
    'spend',
    'requests',
    'tokens',
    'cache',
    'unit-price',
  ].map((id, i) => ({ id, title: id, value: `value-${i}` }));
  return renderToStaticMarkup(
    <StatsCards
      groupedStatsData={groupedStatsData}
      loading={false}
      quotaError={false}
      cacheLoading={false}
      cacheError={false}
      retryCache={() => {}}
      {...props}
    />,
  );
};

test('cache loading and failures do not hide successful aggregate cards', () => {
  for (const props of [{ cacheLoading: true }, { cacheError: true }]) {
    const html = render(props);
    for (const i of [0, 1, 2, 4]) assert.ok(html.includes(`value-${i}`));
    assert.ok(!html.includes('value-3'));
  }
  assert.match(render({ cacheError: true }), /加载失败，请重试/);
  assert.doesNotMatch(render({ cacheLoading: true }), /加载失败，请重试/);
});
test('aggregation failure hides stale values in every card', () => {
  assert.doesNotMatch(render({ quotaError: true }), /value-\d/);
});
const renderHeader = (isAdminUser) =>
  renderToStaticMarkup(
    <Header
      getGreeting='👋 欢迎回来'
      rangePreset='24h'
      onRangeChange={() => {}}
      inputs={{
        start_timestamp: '2026-10-29T14:00:00+08:00',
        end_timestamp: '2026-10-30T14:00:00+08:00',
        username: isAdminUser ? 'account-filter' : '',
      }}
      dataExportDefaultTime='hour'
      isAdminUser={isAdminUser}
      showSearchModal={() => {}}
      refresh={() => {}}
      loading={false}
      t={(key) => key}
    />,
  );

test('overview keeps query controls without duplicating sidebar destinations', () => {
  for (const isAdminUser of [false, true]) {
    const html = renderHeader(isAdminUser);
    assert.equal(html.includes('dashboard-quick-actions'), false);
    assert.equal(html.includes('<nav'), false);
    assert.equal(html.includes('/console/'), false);
    for (const label of [
      '令牌管理',
      '使用日志',
      '渠道管理',
      '用户管理',
      '充值',
    ]) {
      assert.equal(html.includes(label), false);
    }
    assert.match(html, /aria-label="时间范围"/);
    assert.match(html, /aria-label="筛选"/);
    assert.match(html, /aria-label="刷新"/);
    assert.match(html, /2026/);
    assert.match(html, /value="24h" selected/);
    assert.doesNotMatch(html, /<h1|概览|常用操作/);
    assert.doesNotMatch(
      html,
      /时间粒度|dashboard-query-summary|dashboard-timezone/,
    );
    for (const preset of ['today', '24h', '7d', '30d', 'custom']) {
      assert.ok(html.includes(`value="${preset}"`));
    }
  }
  assert.doesNotMatch(renderHeader(true), /account-filter/);
});

test('range selection, filter and refresh keep their callbacks', () => {
  const calls = [];
  const header = Header({
    inputs: {},
    rangePreset: 'custom',
    onRangeChange: (preset) => calls.push(preset),
    showSearchModal: () => calls.push('filter'),
    refresh: () => calls.push('refresh'),
    loading: true,
    t: (key) => key,
  });
  const elements = [];
  const visit = (element) => {
    if (!React.isValidElement(element)) return;
    elements.push(element);
    React.Children.forEach(element.props.children, visit);
  };
  visit(header);
  const select = elements.find((element) => element.type === 'select');
  assert.equal(select.props.value, 'custom');
  select.props.onChange({ target: { value: '7d' } });
  const filter = elements.find(
    (element) => element.props['aria-label'] === '筛选',
  );
  filter.props.onClick();
  const refresh = elements.find(
    (element) => element.props['aria-label'] === '刷新',
  );
  assert.equal(refresh.props.loading, true);
  refresh.props.onClick();
  assert.deepEqual(calls, ['7d', 'filter', 'refresh']);
});

test('invalid or missing timestamps render a placeholder without leaking invalid dates', () => {
  const html = renderToStaticMarkup(
    <Header
      inputs={{ start_timestamp: 'invalid' }}
      rangePreset='custom'
      t={(key) => key}
    />,
  );
  assert.doesNotMatch(html, /Invalid Date|NaN/);
  assert.match(html, /dashboard-applied-range/);
  assert.match(html, /—/);
});

test('metric formatting respects the existing currency settings and zero-token boundary', () => {
  const descriptor = Object.getOwnPropertyDescriptor(
    globalThis,
    'localStorage',
  );
  let settings = {
    quota_per_unit: '500000',
    quota_display_type: 'USD',
    status: JSON.stringify({
      usd_exchange_rate: 7.2,
      custom_currency_symbol: '€',
      custom_currency_exchange_rate: 0.9,
    }),
  };
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: { getItem: (key) => settings[key] ?? null },
  });
  function Content({ tokens = 28000000 }) {
    const { groupedStatsData } = useDashboardStats(
      18500000,
      tokens,
      2680,
      0.628,
      t,
    );
    return <StatsCards groupedStatsData={groupedStatsData} />;
  }
  try {
    assert.match(renderToStaticMarkup(<Content />), /\$37\.00/);
    assert.match(renderToStaticMarkup(<Content />), /\$1\.3214/);
    settings.quota_display_type = 'CNY';
    assert.match(renderToStaticMarkup(<Content />), /¥266\.40/);
    settings.quota_display_type = 'CUSTOM';
    assert.match(renderToStaticMarkup(<Content />), /€33\.30/);
    const empty = renderToStaticMarkup(<Content tokens={0} />);
    assert.doesNotMatch(empty, /Infinity|NaN|62\.8%/);
  } finally {
    if (descriptor)
      Object.defineProperty(globalThis, 'localStorage', descriptor);
    else delete globalThis.localStorage;
  }
});
