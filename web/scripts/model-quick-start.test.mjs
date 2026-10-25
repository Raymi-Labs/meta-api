import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import test from 'node:test';
import { getModelQuickStartExamples } from '../src/pages/ModelDetail/modelQuickStartExamples.js';

const model = (name, protocols) => ({
  model_name: name,
  supported_endpoint_types: protocols,
});

test('only declared and supported protocols get examples, with stable unique choices', () => {
  const result = getModelQuickStartExamples(
    model('kimi-test', ['anthropic', 'openai', 'openai', 'future-protocol']),
    'https://configured.example/v1/',
    'https://browser.example',
  );
  assert.deepEqual(
    result.map((example) => example.endpoint),
    ['openai', 'anthropic'],
  );
  assert.match(
    result[0].code,
    /https:\/\/configured\.example\/v1\/chat\/completions/,
  );
  assert.doesNotMatch(result[0].code, /browser\.example|v1\/v1|```/);
  assert.match(result[0].code, /export NAN_API_KEY="YOUR_API_KEY"/);
  assert.match(result[1].code, /anthropic-version: 2023-06-01/);
});

test('unknown or absent endpoint metadata does not invent Chat support', () => {
  for (const value of [
    null,
    model('', ['openai']),
    model('gpt-test', null),
    model('gpt-test', []),
    model('gpt-test', ['video-generation']),
    model('gpt-test', 'openai'),
  ]) {
    assert.deepEqual(
      getModelQuickStartExamples(value, 'https://example.com', ''),
      [],
    );
  }
});

test('server address wins, invalid addresses fall back, and credentials are never copied', () => {
  const input = model('gpt-test', ['openai-response']);
  const [configured] = getModelQuickStartExamples(
    input,
    'https://user:secret@example.com/gateway/v1/?secret=hidden#fragment',
    'https://browser.example',
  );
  assert.match(
    configured.code,
    /https:\/\/example.com\/gateway\/v1\/responses/,
  );
  assert.doesNotMatch(configured.code, /secret|hidden|fragment|user:/);
  const [fallback] = getModelQuickStartExamples(
    input,
    'javascript:alert(1)',
    'https://browser.example',
  );
  assert.match(fallback.code, /https:\/\/browser.example\/v1\/responses/);
  assert.deepEqual(getModelQuickStartExamples(input, '', ''), []);
});

test('all copied protocols preserve exact model names and shell arguments without executing model text', () => {
  const name = `org/model%2Fpart'$(echo unsafe)"\\newline\n尾`;
  const protocols = [
    'openai',
    'openai-response',
    'anthropic',
    'gemini',
    'image-generation',
  ];
  const samples = getModelQuickStartExamples(
    model(name, protocols),
    'https://example.com/v1',
    '',
  );
  assert.equal(samples.length, 5);
  for (const sample of samples) {
    // Replace curl with an argument recorder; no network call is made.
    const result = spawnSync(
      'bash',
      ['-c', 'curl() { printf "%s\\0" "$@"; };\n' + sample.code],
      { encoding: 'utf8' },
    );
    assert.equal(result.status, 0, result.stderr);
    const args = result.stdout.split('\0').filter(Boolean);
    const body = JSON.parse(args[args.indexOf('-d') + 1]);
    if (sample.endpoint === 'gemini') {
      assert.ok(
        args.includes(
          `https://example.com/v1beta/models/${encodeURIComponent(name)}:generateContent`,
        ),
      );
      assert.ok(Array.isArray(body.contents));
    } else {
      assert.equal(body.model, name);
    }
    assert.ok(args.some((arg) => arg.includes('YOUR_API_KEY')));
    if (sample.endpoint === 'image-generation') assert.equal(body.n, 1);
    if (sample.endpoint === 'anthropic') assert.equal(body.max_tokens, 1024);
  }
});
