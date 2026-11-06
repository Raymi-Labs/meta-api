import assert from 'node:assert/strict';
import { test } from 'bun:test';
import {
  newPlaygroundSession,
  recoverSession,
  sessionBackup,
  parseSessionBackup,
  createConversationGate,
  sessionTitle,
} from '../src/helpers/playgroundSessions.js';

test('new conversation has independent identity and no inherited history/custom body', () => {
  const a = newPlaygroundSession();
  a.config.inputs.model = 'model-fixture';
  a.config.inputs.group = 'private';
  a.config.inputs.systemPrompt = 'old context';
  a.config.customRequestBody = '{"messages":["secret"]}';
  a.messages.push({ role: 'user', content: 'old question' });
  const b = newPlaygroundSession(a.config);
  assert.notEqual(a.id, b.id);
  assert.deepEqual(b.messages, []);
  assert.equal(b.config.inputs.model, 'model-fixture');
  assert.equal(b.config.inputs.group, 'private');
  assert.equal(b.config.inputs.systemPrompt, '');
  assert.equal(b.config.customRequestBody, '');
  b.config.inputs.model = 'other';
  assert.equal(a.config.inputs.model, 'model-fixture');
});
test('reload and import preserve partial output without resuming a request', () => {
  const a = newPlaygroundSession();
  a.messages = [
    {
      role: 'assistant',
      id: 'old',
      content: 'partial',
      status: 'incomplete',
      parallelRound: 'round',
    },
  ];
  const r = recoverSession(a);
  assert.equal(r.messages[0].status, 'error');
  assert.equal(r.messages[0].content, 'partial');
  assert.equal(r.messages[0].localInterrupted, true);
  const b = parseSessionBackup(sessionBackup([a]))[0];
  assert.notEqual(a.id, b.id);
  assert.equal(b.messages[0].status, 'error');
});
test('malformed and oversized imports fail without accepting executable configuration', () => {
  for (const text of [
    '{}',
    '[]',
    'null',
    'broken',
    JSON.stringify({
      format: 'meta-api-local-conversations',
      version: 1,
      sessions: [{ messages: [], config: { inputs: [] } }],
    }),
    '{"__proto__":{}}',
  ])
    assert.throws(() => parseSessionBackup(text));
  const a = newPlaygroundSession();
  a.config.inputs.model = { evil: true };
  assert.throws(() => parseSessionBackup(sessionBackup([a])));
});
test('same conversation may issue up to existing model controls; other conversations are blocked synchronously', () => {
  const gate = createConversationGate();
  assert(gate.acquire('a'));
  assert(gate.acquire('a'));
  assert(!gate.acquire('b'));
  gate.release('b');
  assert.equal(gate.current, 'a');
  gate.release('a');
  assert(gate.acquire('b'));
});
test('titles are plain user text and imported IDs never overwrite an existing session', () => {
  const a = newPlaygroundSession();
  a.messages = [{ role: 'user', content: '<img src=x onerror=alert(1)>' }];
  assert.equal(sessionTitle(a, 'New'), a.messages[0].content);
  const imported = parseSessionBackup(sessionBackup([a, a]));
  assert.notEqual(imported[0].id, imported[1].id);
});
