import assert from 'node:assert/strict';
import {
  isVideoTestModel,
  isLiveTestSuccess,
  summarizeModelTests,
} from '../src/helpers/videoTest.js';
for (const model of [
  'grok-imagine-video-1.5',
  'MiniMax-H3',
  'org/MiniMax-H3-Max',
  'MiniMax-H3-Highspeed',
  'MiniMax-H3-Lite',
  'wan2.6-t2v',
  'veo3.1',
  'seedance-2.0',
  'MiniMax-Hailuo-02',
  'org/kling-v2.6',
])
  assert.equal(isVideoTestModel(model), true, model);
for (const model of [
  'gpt-4o',
  'kimi-k3',
  'qwen-vl',
  'seedream-4.0',
  'MiniMax-M2.5',
])
  assert.equal(isVideoTestModel(model), false, model);
assert.equal(isVideoTestModel('private-model', 50), true);
assert.equal(isVideoTestModel('private-model', 1, 'openai-video'), true);
const passed = { success: true, testKind: 'video_preflight' };
const incomplete = { success: false, testKind: 'video_preflight' };
assert.equal(isLiveTestSuccess(passed), false);
assert.equal(isLiveTestSuccess(incomplete), false);
assert.equal(isLiveTestSuccess({ success: true }), true);
assert.deepEqual(
  summarizeModelTests([
    passed,
    incomplete,
    { success: true },
    { success: false },
    undefined,
  ]),
  { success: 1, failed: 2, preflight: 2 },
);
console.log('video preflight UI semantics passed');
