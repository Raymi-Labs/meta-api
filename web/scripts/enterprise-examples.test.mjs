import assert from 'node:assert/strict';
import test from 'node:test';
import { createEnterpriseExample } from '../src/pages/Enterprise/enterpriseData.js';

const base = 'https://meta-api.vip/v1';
test('cURL examples preserve protocol endpoints, authentication and valid payloads', () => {
  for (const [protocol, endpoint] of [
    ['chat', '/chat/completions'],
    ['messages', '/messages'],
    ['responses', '/responses'],
  ]) {
    const code = createEnterpriseExample(protocol, 'cURL', base);
    assert.ok(code.startsWith(`curl "${base}${endpoint}"`));
    assert.ok(code.includes('Content-Type: application/json'));
    const payload = JSON.parse(code.match(/-d '([\s\S]+)'$/)[1]);
    assert.equal(payload.model, 'MODEL_ID');
    if (protocol === 'responses') {
      assert.equal(payload.input, 'Hello!');
      assert.equal(payload.messages, undefined);
    } else {
      assert.deepEqual(payload.messages, [{ role: 'user', content: 'Hello!' }]);
    }
    if (protocol === 'messages') {
      assert.ok(code.includes('x-api-key: $API_KEY'));
      assert.ok(code.includes('anthropic-version: 2023-06-01'));
      assert.ok(payload.max_tokens > 0);
    } else {
      assert.ok(code.includes('Authorization: Bearer $API_KEY'));
    }
  }
});

test('SDK clients match protocols and preserve custom gateway prefixes', () => {
  for (const baseUrl of [base, 'https://gateway.example/team/v1']) {
    const chat = createEnterpriseExample('chat', 'Python', baseUrl);
    const responses = createEnterpriseExample('responses', 'Python', baseUrl);
    const messages = createEnterpriseExample('messages', 'Python', baseUrl);
    for (const code of [chat, responses, messages]) {
      assert.ok(code.includes('os.environ["API_KEY"]'));
      assert.ok(code.includes('model="MODEL_ID"'));
    }
    assert.ok(chat.includes('client.chat.completions.create'));
    assert.ok(responses.includes('client.responses.create'));
    assert.ok(chat.includes(`base_url="${baseUrl}"`));
    assert.ok(responses.includes(`base_url="${baseUrl}"`));
    assert.ok(messages.includes('from anthropic import Anthropic'));
    assert.ok(messages.includes(`base_url="${baseUrl.replace(/\/v1$/, '')}"`));
    assert.ok(messages.includes('client.messages.create'));
    assert.ok(messages.includes('max_tokens=1024'));
  }
});
