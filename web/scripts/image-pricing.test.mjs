import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  buildImagePriceExpression,
  parseImagePriceExpression,
  emptyImagePrices,
  IMAGE_PRICE_REFERENCES,
} from '../src/helpers/imagePricing.js';
import { isVideoTestModel } from '../src/helpers/videoTest.js';
import { canUseVideoTemplate } from '../src/helpers/videoPricingTemplate.js';
const rows = [
  { resolution: '1K', input: 0.01, output: 0.1 },
  { resolution: '2K', input: 0.02, output: 0.2 },
];
const expr = buildImagePriceExpression(rows);
assert.equal(
  expr,
  readFileSync(
    new URL(
      '../../service/testdata/image_price_expression.txt',
      import.meta.url,
    ),
    'utf8',
  ).trim(),
);
assert.deepEqual(parseImagePriceExpression(expr), rows);
assert.equal(buildImagePriceExpression(emptyImagePrices()), null);
for (const invalid of [
  '',
  null,
  undefined,
  -1,
  Infinity,
  NaN,
  'bad',
  true,
  1000001,
])
  assert.equal(
    buildImagePriceExpression([{ ...rows[0], input: invalid }, rows[1]]),
    null,
  );
assert(
  buildImagePriceExpression(rows.map((r) => ({ ...r, input: 0, output: 0 }))),
);
assert.equal(parseImagePriceExpression(expr + ' * 2'), null);
assert.equal(IMAGE_PRICE_REFERENCES['qwen-image-3.0-pro'].rows[0].input, 0.02);
assert.equal(IMAGE_PRICE_REFERENCES['unknown'], undefined);
for (const name of [
  'wan2.7-image-pro',
  'wan2.6-image',
  'wanx2.1-t2i-turbo',
  'org/wan2.5-i2i-preview',
]) {
  assert.equal(isVideoTestModel(name), false, name);
  assert.equal(canUseVideoTemplate(name), false, name);
  assert.equal(isVideoTestModel(name, 50), true);
  assert.equal(isVideoTestModel(name, 3, 'openai-video'), true);
}
for (const name of [
  'wan2.7-t2v',
  'wan2.7-i2v',
  'wan2.7-videoedit',
  'wan2.7-image-t2v',
  'wan2.7-unknown',
])
  assert.equal(isVideoTestModel(name), true, name);
console.log(
  'image pricing round-trip, shared expression and Wan classification passed',
);
