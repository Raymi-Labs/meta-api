import assert from 'node:assert/strict';
import { test } from 'bun:test';
import { createServer } from 'node:http';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import {
  buildApiPayload,
  buildPlaygroundPreview,
  buildPlaygroundCode,
  reconcileModelTabs,
  getPlaygroundEndpoint,
} from '../src/helpers/playgroundRequest';
const run = promisify(execFile);
const inputs = {
  model: 'test-model',
  group: 'vip',
  systemPrompt: '  You are helpful.  ',
  stream: true,
  temperature: 0,
  max_tokens: 4096,
  seed: 0,
  imageEnabled: false,
  imageUrls: [],
};
const enabled = { temperature: true, max_tokens: false, seed: true };

test('system prompt, zero values and enabled parameters share the request/preview contract', () => {
  const messages = [
    { role: 'user', content: 'First' },
    { role: 'assistant', content: 'Failed', status: 'error' },
    { role: 'assistant', content: '', status: 'loading' },
  ];
  const preview = buildPlaygroundPreview(messages, 'Next', inputs, enabled);
  assert.deepEqual(
    preview,
    buildApiPayload(
      [...messages, { role: 'user', content: 'Next' }],
      null,
      inputs,
      enabled,
    ),
  );
  assert.deepEqual(preview.messages, [
    { role: 'system', content: 'You are helpful.' },
    { role: 'user', content: 'First' },
    { role: 'user', content: 'Next' },
  ]);
  assert.equal(preview.temperature, 0);
  assert.equal(preview.seed, 0);
  assert.equal('max_tokens' in preview, false);
  assert.equal(messages.length, 3);
});

test('image preview matches the draft and never rewrites historical messages', () => {
  const messages = [{ role: 'user', content: 'history' }];
  const preview = buildPlaygroundPreview(
    messages,
    'describe',
    {
      ...inputs,
      imageEnabled: true,
      imageUrls: ['https://example.test/image.png', ''],
    },
    enabled,
  );
  assert.deepEqual(preview.messages[2].content, [
    { type: 'text', text: 'describe' },
    { type: 'image_url', image_url: { url: 'https://example.test/image.png' } },
  ]);
  assert.equal(messages[0].content, 'history');
  assert.equal(
    buildPlaygroundPreview(messages, '', inputs, enabled).messages.length,
    2,
  );
});

test('model tabs follow group permissions, remove duplicates and keep a selected model', () => {
  const models = ['a', 'b', 'c'].map((value) => ({ value }));
  assert.deepEqual(reconcileModelTabs(['b', 'removed', 'b'], 'a', models), [
    'b',
    'a',
  ]);
  assert.deepEqual(reconcileModelTabs(['removed'], 'missing', models), [
    'a',
    'b',
    'c',
  ]);
  assert.deepEqual(reconcileModelTabs(['a'], 'a', []), []);
  assert.deepEqual(reconcileModelTabs(null, 'a', models), ['a']);
});

test('public endpoint strips credentials/query and does not duplicate v1', () => {
  assert.equal(
    getPlaygroundEndpoint(
      'https://user:secret@example.test/gateway/v1/?secret=value#x',
    ),
    'https://example.test/gateway/v1/chat/completions',
  );
  assert.equal(getPlaygroundEndpoint('javascript:alert(1)'), '');
});

test('exported cURL, Python and JavaScript send the exact public payload in both stream modes', async () => {
  const requests = [];
  const server = createServer(async (req, res) => {
    let raw = '';
    for await (const part of req) raw += part;
    const body = JSON.parse(raw);
    requests.push({ body, auth: req.headers.authorization, url: req.url });
    res.setHeader(
      'Content-Type',
      body.stream ? 'text/event-stream' : 'application/json',
    );
    res.end(
      body.stream
        ? 'data: {"choices":[{"delta":{"content":"ok"}}]}\n\ndata: [DONE]\n\n'
        : '{"choices":[{"message":{"content":"ok"}}]}',
    );
  });
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const address = `http://127.0.0.1:${server.address().port}`;
  try {
    for (const stream of [true, false]) {
      const payload = buildPlaygroundPreview(
        [],
        `中文 ' " \\ \n $HOME $(printf injected)`,
        { ...inputs, stream },
        enabled,
      );
      const { group, ...expected } = payload;
      for (const language of ['curl', 'python', 'javascript']) {
        const code = buildPlaygroundCode(payload, language, address);
        const [cmd, args] =
          language === 'curl'
            ? ['bash', ['-c', code]]
            : language === 'python'
              ? ['python3', ['-c', code]]
              : ['node', ['--input-type=module', '-e', code]];
        await run(cmd, args, {
          env: { ...process.env, META_API_KEY: 'fixture-key' },
          timeout: 10000,
        });
        const actual = requests.at(-1);
        assert.deepEqual(actual.body, expected, language);
        assert.equal(actual.auth, 'Bearer fixture-key');
        assert.equal(actual.url, '/v1/chat/completions');
        assert.equal(code.includes('New-Api-User'), false);
      }
      assert.deepEqual(
        JSON.parse(buildPlaygroundCode(payload, 'json', address)),
        expected,
      );
      assert.equal(payload.group, 'vip');
    }
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
}, 30000);
