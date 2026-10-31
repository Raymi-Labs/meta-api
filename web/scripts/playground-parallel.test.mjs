import assert from 'node:assert/strict';
import test from 'node:test';
import {
  sharedConversation,
  updateParallelReply,
  isPlainConversation,
  isTextComparisonModel,
  canUseParallelReply,
  recoverParallelReplies,
} from '../src/helpers/playgroundParallel.js';

test('reload and import interrupt every in-flight branch without changing complete replies', () => {
  const done = {
    id: 'done',
    parallelRound: 'r',
    status: 'complete',
    content: 'safe',
    parallelChosen: true,
  };
  const active = {
    id: 'pending',
    parallelRound: 'r',
    status: 'incomplete',
    content: 'partial',
    parallelChosen: true,
  };
  const rows = recoverParallelReplies([
    done,
    active,
    { ...active, id: 'loading', status: 'loading' },
  ]);
  assert.equal(rows[0], done);
  assert.ok(
    rows
      .slice(1)
      .every(
        (m) =>
          m.parallelStopped && !m.parallelChosen && m.status === 'complete',
      ),
  );
  assert.equal(active.status, 'incomplete');
});

test('interleaved updates and terminal callbacks cannot overwrite another model', () => {
  const a = {
    id: 'a',
    role: 'assistant',
    content: '',
    status: 'loading',
    parallelRound: 'r',
    parallelModel: 'A',
  };
  const b = { ...a, id: 'b', parallelModel: 'B' };
  let rows = [{ id: 'r', role: 'user', content: 'same question' }, a, b];
  rows = updateParallelReply(rows, 'a', ([m]) => [
    { ...m, content: 'A1', status: 'incomplete' },
  ]);
  rows = updateParallelReply(rows, 'b', ([m]) => [
    { ...m, content: 'B1', status: 'complete' },
  ]);
  rows = updateParallelReply(rows, 'a', ([m]) => [
    { ...m, content: m.content + 'A2', status: 'complete' },
  ]);
  assert.deepEqual(
    rows.slice(1).map((m) => m.content),
    ['A1A2', 'B1'],
  );
  assert.equal(a.content, '');
  assert.equal(
    updateParallelReply(rows, 'deleted', () => []),
    rows,
  );
  const stopped = rows.map((m) =>
    m.id === 'a' ? { ...m, parallelStopped: true } : m,
  );
  assert.equal(
    updateParallelReply(stopped, 'a', () => []),
    stopped,
  );
});
test('each round requires exactly one complete chosen reply, including code export', () => {
  const user = { id: 'r', role: 'user', content: 'question' };
  const a = {
    id: 'a',
    role: 'assistant',
    content: 'A',
    status: 'complete',
    parallelRound: 'r',
  };
  const b = { ...a, id: 'b', content: 'B' };
  assert.throws(() => sharedConversation([user, a, b]));
  assert.deepEqual(
    sharedConversation([user, { ...a, parallelChosen: true }, b]).map(
      (m) => m.content,
    ),
    ['question', 'A'],
  );
  assert.throws(() =>
    sharedConversation([
      user,
      { ...a, parallelChosen: true },
      { ...b, parallelChosen: true },
    ]),
  );
  for (const patch of [
    { finishReason: 'length' },
    { finishReason: 'content_filter' },
    { status: 'error' },
    { parallelStopped: true },
    { content: '' },
    { tool_calls: [{}] },
  ])
    assert.equal(canUseParallelReply({ ...a, ...patch }), false);
});
test('text eligibility never infers capabilities from a model name', () => {
  assert.equal(isTextComparisonModel({ model_name: 'gpt-anything' }), false);
  assert.equal(
    isTextComparisonModel({ supported_endpoint_types: ['openai'] }),
    true,
  );
  for (const endpoint of [
    'image-generation',
    'video',
    'audio',
    'embeddings',
    'rerank',
  ])
    assert.equal(
      isTextComparisonModel({ supported_endpoint_types: ['openai', endpoint] }),
      false,
    );
  assert.equal(
    isTextComparisonModel({
      supported_endpoint_types: ['openai'],
      output_modalities: ['image'],
    }),
    false,
  );
  assert.equal(isPlainConversation([{ role: 'user', content: 'text' }]), true);
  assert.equal(
    isPlainConversation([{ role: 'user', content: [{ type: 'image_url' }] }]),
    false,
  );
  assert.equal(
    isPlainConversation([{ role: 'assistant', content: '', tool_calls: [{}] }]),
    false,
  );
});
