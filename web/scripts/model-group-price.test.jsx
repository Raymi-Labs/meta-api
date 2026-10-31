import assert from 'node:assert/strict';
import { test } from 'bun:test';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import ModelGroupPrice from '../src/pages/ModelDetail/ModelGroupPrice';

const renderPrice = (
  amount,
  { key = 'completion', currency = 'CNY', tokenUnit = 'M', officialTier } = {},
) =>
  renderToStaticMarkup(
    <ModelGroupPrice
      item={{
        key,
        label: '补全价格',
        value: '原价格',
        suffix: `/ 1${tokenUnit} Tokens`,
      }}
      pricing={{ unitPricesUSD: { [key]: amount } }}
      model={{ quota_type: 0, official_pricing: { currency } }}
      officialTier={
        officialTier === undefined
          ? {
              unit: 'million_tokens',
              input: 66.53,
              output: 66.53,
              cache_read: 66.53,
              cache_write: 66.53,
            }
          : officialTier
      }
      data={{
        t: (text, values) => text.replace('{{percent}}', values?.percent ?? ''),
        currency,
        tokenUnit,
        siteDisplayType: 'USD',
        displayPrice: { toAmount: (value) => value },
      }}
    />,
  );

test('group prices suppress the reported floating point tails in actual markup', () => {
  for (const [amount, expected, badge] of [
    [54.39999999999976, '54.4', '较官方低 18.2%'],
    [20.39999999999991, '20.4', '较官方低 69.3%'],
    [373.9999999999984, '374', '较官方高 462.2%'],
  ]) {
    const html = renderPrice(amount);
    assert.ok(html.includes(`<strong>¥${expected}</strong>`), html);
    assert.ok(html.includes('¥66.53'), html);
    assert.ok(html.includes(badge), html);
  }
});

test('input, output and cache prices share formatting in both currencies and units', () => {
  for (const key of [
    'input',
    'completion',
    'cache',
    'create-cache',
    'tiered-p',
    'tiered-c',
    'tiered-cr',
    'tiered-cc',
  ]) {
    for (const [currency, symbol] of [
      ['USD', '$'],
      ['CNY', '¥'],
    ]) {
      for (const [tokenUnit, divisor] of [
        ['M', 1],
        ['K', 1000],
      ]) {
        const html = renderPrice(54.39999999999976 / divisor, {
          key,
          currency,
          tokenUnit,
        });
        assert.ok(
          html.includes(`<strong>${symbol}${54.4 / divisor}</strong>`),
          html,
        );
      }
    }
  }
});

test('tiny nonzero prices and zero survive display normalization', () => {
  for (const [amount, expected] of [
    [0, '0'],
    [1e-9, '0.000000001'],
    [0.00000123456789, '0.00000123456789'],
  ]) {
    const html = renderPrice(amount);
    assert.ok(html.includes(`<strong>¥${expected}</strong>`), html);
    assert.ok(
      html.includes(amount === 0 ? '较官方低 100%' : '较官方低 99.9%'),
      html,
    );
  }
});

test('unavailable comparison keeps the existing price item', () => {
  const html = renderPrice(54.4, { officialTier: null });
  assert.ok(html.includes('<strong>原价格</strong>'), html);
  assert.ok(!html.includes('model-group-discount'), html);
});
