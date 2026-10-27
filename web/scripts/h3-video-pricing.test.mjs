import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  buildH3SecondsExpression,
  h3VideoProfiles,
} from '../src/helpers/h3VideoPricing.js';
const expected = readFileSync(
  new URL(
    '../../relay/channel/task/hailuo/testdata/h3_seconds_expression.txt',
    import.meta.url,
  ),
  'utf8',
).trim();
assert.equal(
  buildH3SecondsExpression('MiniMax-H3', { '768P': 0.1, '2K': 0.2 }),
  expected,
);
assert.equal(buildH3SecondsExpression('MiniMax-M2.5', {}), null);
for (const invalid of ['', null, undefined, -1, Infinity, NaN, 'bad', 1000001])
  assert.equal(
    buildH3SecondsExpression('MiniMax-H3', { '768P': invalid, '2K': 0.2 }),
    null,
  );
assert.equal(buildH3SecondsExpression('MiniMax-H3', { '768P': 0.1 }), null);
assert(
  buildH3SecondsExpression('MiniMax-H3-Lite', {
    '480P': 0,
    '768P': 0.2,
  }).includes('param("duration") * 0'),
);
assert(
  buildH3SecondsExpression('MiniMax-H3-Max', {
    '480P': 0.1,
    '768P': 0.2,
  }).includes('>= 5'),
);
assert.deepEqual(h3VideoProfiles['MiniMax-H3-Highspeed'].resolutions, ['768P']);
console.log(
  'H3 seconds pricing template and shared backend expression fixture passed.',
);
