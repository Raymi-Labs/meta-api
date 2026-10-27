import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  buildVideoPricingTemplate,
  canUseVideoTemplate,
} from '../src/helpers/videoPricingTemplate.js';
const rows = [{ resolution: '720p', min: 4, max: 15, rate: 0.1 }];
const seconds = buildVideoPricingTemplate(rows);
assert(seconds.includes('param("duration") * 100000'));
assert(seconds.includes('"720P"'));
assert(seconds.includes('param("sampleCount")'));
const fixed = buildVideoPricingTemplate(
  [{ resolution: '720p', min: 5, rate: 2 }],
  'fixed',
);
assert(fixed.includes('param("duration") <= 5'));
assert(!fixed.includes('param("duration") *'));
assert(fixed.includes('2000000'));
assert.equal(
  buildVideoPricingTemplate([
    ...rows,
    { resolution: '720P', min: 10, max: 20, rate: 2 },
  ]),
  null,
);
for (const rate of ['', null, NaN, Infinity, -1])
  assert.equal(buildVideoPricingTemplate([{ ...rows[0], rate }]), null);
assert(buildVideoPricingTemplate([{ ...rows[0], rate: 0 }]).includes('* 0'));
assert.equal(
  buildVideoPricingTemplate([{ ...rows[0], resolution: 'x"inject' }]),
  null,
);
assert.equal(buildVideoPricingTemplate([{ ...rows[0], min: 1.5 }]), null);
for (const name of [
  'grok-imagine-video',
  'sora-2-pro',
  'wan2.5-t2v-preview',
  'kling-v2-master',
  'viduq2',
  'veo-3.0',
  'MiniMax-H3',
  'MiniMax-Hailuo-02',
  'doubao-seedance-1-0-pro',
])
  assert(canUseVideoTemplate(name), name);
for (const name of [
  'MiniMax-M2.5',
  'runway-new',
  'happyhorse',
  'jimeng_v30',
  'gpt-6',
])
  assert(!canUseVideoTemplate(name), name);
assert.equal(
  seconds,
  readFileSync(
    new URL(
      '../../relay/testdata/video_seconds_expression.txt',
      import.meta.url,
    ),
    'utf8',
  ).trim(),
);
assert.equal(
  fixed,
  readFileSync(
    new URL('../../relay/testdata/video_fixed_expression.txt', import.meta.url),
    'utf8',
  ).trim(),
);
console.log(
  'Video seconds/fixed templates, zero/missing/overlapping tiers and supported families passed.',
);
