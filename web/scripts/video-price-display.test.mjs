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

test('H3 generated templates retain each resolution, bounds and zero price', async () => {
  const { buildH3SecondsExpression, h3VideoProfiles } = await import(
    '../src/helpers/h3VideoPricing.js'
  );
  const { videoResolutionRates } = await import(
    '../src/helpers/videoPriceDisplay.js'
  );
  for (const [model, profile] of Object.entries(h3VideoProfiles)) {
    const prices = Object.fromEntries(
      profile.resolutions.map((r, i) => [r, i ? 4 : 0]),
    );
    const expr = buildH3SecondsExpression(model, prices);
    const rows = videoResolutionRates(expr);
    assert.equal(rows.length, profile.resolutions.length);
    assert.deepEqual(
      rows.map((r) => r.resolution),
      profile.resolutions,
    );
    assert.ok(
      rows.every((r) => r.min === profile.min && r.max === profile.max),
    );
    assert.equal(rows[0].rate, 0);
    assert.deepEqual(videoResolutionRates('v1:' + expr), rows);
    assert.deepEqual(
      videoResolutionRates(expr.replaceAll(' ? ', '\n ?\n ')),
      rows,
    );
    for (const altered of [
      expr + ' * 2',
      '(' + expr + ') + 500000',
      expr.replace(
        'param("duration") *',
        'param("duration") * param("duration") *',
      ),
    ])
      assert.deepEqual(videoResolutionRates(altered), []);
  }
});

test('standard video templates preserve same-resolution duration tiers and sample semantics', async () => {
  const { buildVideoPricingTemplate } = await import(
    '../src/helpers/videoPricingTemplate.js'
  );
  const { videoResolutionRates } = await import(
    '../src/helpers/videoPriceDisplay.js'
  );
  const rows = [
    { resolution: '720P', min: 4, max: 8, rate: 0.5 },
    { resolution: '720P', min: 9, max: 15, rate: 0.8 },
    { resolution: '1080P', min: 4, max: 15, rate: 1e-9 },
  ];
  const expr = buildVideoPricingTemplate(rows);
  assert.deepEqual(
    videoResolutionRates(expr),
    rows.map((row) => ({ ...row, multiple: true })),
  );
  const items = videoPriceItems(expr, 1, {
    currencySymbol: '¥',
    toAmount: (n) => n * 0.2,
  });
  assert.equal(items[0].label, '720P · 4–8 秒');
  assert.equal(items[1].label, '720P · 9–15 秒');
  assert.equal(items[2].value, '<¥0.0001');
  assert.equal(items[0].multiple, true);
  assert.deepEqual(
    videoResolutionRates(buildVideoPricingTemplate(rows, 'fixed')),
    [],
  );
  assert.deepEqual(videoPriceItems(expr, -1, {}), []);
  assert.deepEqual(videoPriceItems(expr, null, {}), []);
});

test('unsupported conditions cannot be assigned an invented resolution', async () => {
  const { buildH3SecondsExpression } = await import(
    '../src/helpers/h3VideoPricing.js'
  );
  const { videoResolutionRates } = await import(
    '../src/helpers/videoPriceDisplay.js'
  );
  const expr = buildH3SecondsExpression('MiniMax-H3', { '768P': 2.5, '2K': 4 });
  for (const value of [
    expr.replace('== "768P"', '!= "768P"'),
    expr.replace('>= 4', '>= 3'),
    '(' + expr + ') * (header("X-Tier") == "premium" ? 2 : 1)',
    '',
    null,
  ])
    assert.deepEqual(videoResolutionRates(value), []);
});
