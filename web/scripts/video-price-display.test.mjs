import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  videoSecondRates,
  videoPriceItems,
  isVideoConsumption,
  videoLogSegments,
} from '../src/helpers/videoPriceDisplay.js';
const expression =
  'param("duration") == nil ? tier("video_config_preview", 0) : (param("duration") <= 180 ? tier("muse", param("duration") * 12000000.0) : tier("unsupported_video_parameters", -1))';
test('linear video prices include group and cash conversion, independently of token units', () => {
  assert.deepEqual(videoSecondRates(expression), [12]);
  const result = videoPriceItems(expression, 0.25, {
    currencySymbol: '¥',
    toAmount: (n) => n * 0.2,
  });
  assert.equal(result[0].value, '¥0.6000');
  assert.equal(result[0].suffix, ' / s');
  assert.equal(
    videoPriceItems(expression, 0.5, {
      currencySymbol: '⚡',
      toAmount: (n) => n,
    })[0].value,
    '⚡6.0000',
  );
});
test('fixed, mixed and nonlinear expressions are not invented as per-second rates', () => {
  for (const value of [
    'tier("fixed", 5)',
    expression + ' + tier("other", 5)',
    '(' + expression + ') * 2',
    'tier("x", param("duration") * 2000000 * param("duration"))',
  ])
    assert.deepEqual(videoSecondRates(value), []);
});
test('historical video logs display actual debit without recalculating zero model price', () => {
  const record = { type: 2, quota: 123 };
  const other = { request_path: '/v1/videos', model_price: 0 };
  assert.equal(isVideoConsumption(record, other), true);
  assert.deepEqual(
    videoLogSegments(record, other, (n) => `Q${n}`).map((x) => x.text),
    ['扣费：Q123'],
  );
  assert.equal(isVideoConsumption({ ...record, type: 6 }, other), false);
  assert.equal(
    isVideoConsumption(record, { request_path: '/v1/chat/completions' }),
    false,
  );
});
test('new task snapshots describe actual per-request-second debit', () => {
  const segments = videoLogSegments(
    { type: 2, quota: 180 },
    { task_billing: { requested_seconds: 60, matched_tier: 'muse' } },
    (n) => `Q${n}`,
  );
  assert.equal(segments[1].text, '60 s · Q3 / s');
});

test('multiple resolutions use a range and unsupported expression-wide arithmetic stays dynamic', () => {
  const expr =
    'param("duration") == nil ? tier("video_config_preview",0) : (param("resolution") == "480p" ? tier("low",param("duration") * 3000000) : tier("high",param("duration") * 6000000))';
  assert.deepEqual(videoSecondRates(expr), [3, 6]);
  assert.equal(
    videoPriceItems(expr, 0.25, {
      currencySymbol: '¥',
      toAmount: (n) => n * 0.2,
    })[0].value,
    '¥0.1500 – ¥0.3000',
  );
  assert.deepEqual(videoSecondRates('(' + expr + ') * 2'), []);
});

test('video-related violation fees retain their separate fee rendering', () => {
  assert.equal(
    isVideoConsumption(
      { type: 2, quota: 1 },
      { request_path: '/v1/videos', violation_fee: true },
    ),
    false,
  );
});
