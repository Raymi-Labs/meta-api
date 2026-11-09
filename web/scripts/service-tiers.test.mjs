import assert from 'node:assert/strict';
import { test } from 'bun:test';
import { buildApiPayload } from '../src/helpers/playgroundRequest';
import { convertPlaygroundPayload } from '../src/helpers/playgroundCode';
const messages = [{ role: 'user', content: 'hello' }];
test('paid tiers require current authorization and emit exact upstream enum', () => {
  const base = {
    model: 'tier-test',
    group: 'g',
    stream: true,
    serviceTierAllowed: true,
  };
  assert(!('service_tier' in buildApiPayload(messages, null, base, {})));
  for (const tier of ['priority', 'ultrafast']) {
    const payload = buildApiPayload(
      messages,
      null,
      { ...base, serviceTier: tier },
      {},
    );
    assert.equal(payload.service_tier, tier);
    assert.throws(() =>
      buildApiPayload(
        messages,
        null,
        { ...base, serviceTier: tier, serviceTierAllowed: false },
        {},
      ),
    );
  }
  assert.throws(() =>
    buildApiPayload(messages, null, { ...base, serviceTier: 'fast' }, {}),
  );
});
test('Responses export preserves paid tier and does not claim Messages compatibility', () => {
  const payload = { model: 'tier-test', messages, service_tier: 'ultrafast' };
  const responses = convertPlaygroundPayload(payload, 'responses');
  assert.equal(responses.body.service_tier, 'ultrafast');
  const messagesResult = convertPlaygroundPayload(payload, 'messages');
  assert(messagesResult.errors.includes('service_tier'));
});
