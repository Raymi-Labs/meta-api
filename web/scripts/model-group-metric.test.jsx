import assert from 'node:assert/strict';
import { test } from 'bun:test';
import React from 'react';
import { StaticRouter } from 'react-router-dom/server';
import { renderToStaticMarkup } from 'react-dom/server';
import ModelGroupMetric from '../src/pages/ModelDetail/ModelGroupMetric';
import { modelGroupHealthState } from '../src/helpers/modelGroupHealthState';
const health = {
  isAuthenticated: true,
  data: { enabled: true },
  loading: false,
  error: null,
  refresh: () => {},
};
const render = (patch = {}, value = null, extra = {}) =>
  renderToStaticMarkup(
    <StaticRouter>
      <ModelGroupMetric
        health={{ ...health, ...patch }}
        rate
        value={value}
        formatted={value == null ? '—' : Number(value).toFixed(2) + '%'}
        modelName='test:free'
        label='Public · Uptime'
        t={(s) => s}
        onToggle={() => {}}
        {...extra}
      />
    </StaticRouter>,
  );
test('every unavailable state explains itself without a blank rate or inactive arrow', () => {
  for (const [patch, text] of [
    [{ isAuthenticated: false }, '登录后查看'],
    [{ loading: true }, '加载中...'],
    [{ error: new Error('offline') }, '加载失败'],
    [{ data: { enabled: false } }, '暂未开启'],
    [{}, '暂无统计数据'],
  ]) {
    const html = render(patch);
    assert.ok(html.includes(text), html);
    assert.ok(!html.includes('—'), html);
    assert.ok(!html.includes('chevron-down'), html);
    assert.ok(!html.includes('aria-expanded'), html);
  }
  const login = render({ isAuthenticated: false }, 99);
  assert.ok(login.includes('/login?next=%2Fmodel%2Ftest%253Afree'));
  assert.ok(!login.includes('99.00%'));
  assert.ok(render({}, null, { samples: 2 }).includes('样本不足'));
});
test('zero, partial samples and probe-only uptime use exact real measurements', () => {
  for (const value of [0, 35.5, 100]) {
    const html = render({}, value);
    assert.ok(html.includes(value.toFixed(2) + '%'));
    assert.ok(html.includes('aria-expanded="false"'));
  }
  assert.ok(render({ loading: true }, 0).includes('0.00%'));
  assert.ok(render({ error: new Error('offline') }, 100).includes('加载失败'));
  for (const value of [undefined, null, '', NaN, Infinity, -1, 101])
    assert.notEqual(
      modelGroupHealthState(health, value, { rate: true }),
      'ready',
    );
});
test('missing latency/throughput share explicit states while genuine numeric zero survives', () => {
  assert.equal(modelGroupHealthState(health, null), 'empty');
  assert.equal(modelGroupHealthState(health, 0), 'ready');
  assert.equal(
    modelGroupHealthState({ ...health, isAuthenticated: false }, 123),
    'anonymous',
  );
  assert.equal(
    modelGroupHealthState({ ...health, data: { enabled: false } }, 123),
    'disabled',
  );
});
