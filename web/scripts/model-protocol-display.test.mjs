import assert from 'node:assert/strict';
import test from 'node:test';
import {
  getDeclaredModelProtocols,
  getModelProtocolOptions,
  getModelEndpointRows,
  getModelProtocolLabel,
  canOfferTextProtocolExamples,
} from '../src/helpers/modelProtocols.js';
import { getModelQuickStartExamples } from '../src/pages/ModelDetail/modelQuickStartExamples.js';

const model = (endpoints, extra = {}) => ({
  model_name: 'gpt-6.1-sol',
  supported_endpoint_types: endpoints,
  ...extra,
});

test('Chat-only exposes undeclared Responses template without changing declarations', () => {
  const input = model(['openai']);
  const before = structuredClone(input);
  assert.deepEqual(getDeclaredModelProtocols(input), ['openai']);
  assert.deepEqual(
    getModelProtocolOptions(input, true).map((x) => [x.label, x.declared]),
    [
      ['Chat Completions', true],
      ['Responses', false],
      ['Messages', false],
    ],
  );
  const rows = getModelEndpointRows(input);
  assert.equal(
    rows.find((x) => x.endpoint === 'openai-response').path,
    '/v1/responses',
  );
  assert.deepEqual(input, before);
  assert.deepEqual(
    getModelQuickStartExamples(input, 'https://example.com', '').map(
      (x) => x.endpoint,
    ),
    ['openai'],
  );
});

test('declared protocols, duplicates, unknown and invalid declarations remain explicit', () => {
  const input = model([
    'openai-response',
    'openai-response',
    null,
    17,
    '',
    'custom-api',
  ]);
  assert.deepEqual(getDeclaredModelProtocols(input), [
    'openai-response',
    'custom-api',
  ]);
  assert.equal(getModelProtocolLabel('custom-api'), 'custom-api');
  const rows = getModelEndpointRows(input, {
    'custom-api': { path: '/private/{model}', method: 'GET' },
  });
  assert.equal(rows.filter((x) => x.endpoint === 'openai-response').length, 1);
  assert.equal(
    rows.find((x) => x.endpoint === 'openai-response').declared,
    true,
  );
  assert.equal(
    rows.find((x) => x.endpoint === 'custom-api').path,
    '/private/gpt-6.1-sol',
  );
  for (const value of [null, {}, model(null), model('openai'), model([])])
    assert.deepEqual(getModelEndpointRows(value), []);
});

test('media, embedding and unknown models never gain text protocol templates', () => {
  for (const endpoints of [
    ['openai', 'image-generation'],
    ['openai', 'openai-video'],
    ['openai', 'audio'],
    ['openai', 'embeddings'],
    ['openai', 'rerank'],
    ['gemini'],
    ['custom-only'],
    [],
  ]) {
    const input = model(endpoints);
    assert.equal(canOfferTextProtocolExamples(input), false);
    assert.equal(
      getModelEndpointRows(input).some((x) => !x.declared),
      false,
    );
    assert.deepEqual(
      getModelProtocolOptions(input, true),
      getModelProtocolOptions(input, false),
    );
  }
  assert.equal(
    canOfferTextProtocolExamples(
      model(['openai'], { output_modalities: ['image'] }),
    ),
    false,
  );
  assert.equal(
    canOfferTextProtocolExamples(
      model(['openai'], {
        input_modalities: ['image'],
        output_modalities: ['text'],
      }),
    ),
    true,
  );
});

test('unknown mapping and hostile model names are displayed without becoming links or template destinations', () => {
  const input = model(['openai', 'custom'], {
    model_name: 'a/b?x=<img onerror=alert(1)>#tail',
  });
  const rows = getModelEndpointRows(input, {
    openai: { path: '/gateway/chat' },
    custom: { path: '/custom/{model}', method: 'GET' },
    'openai-response': { path: 'https://unrelated.invalid/private' },
  });
  assert.equal(rows[0].path, '/gateway/chat');
  assert.equal(rows[1].path, '/custom/' + encodeURIComponent(input.model_name));
  assert.equal(
    rows.find((x) => x.endpoint === 'openai-response').path,
    '/v1/responses',
  );
  assert.equal(getModelEndpointRows(model(['toString']))[0].path, '');
});

test('detail examples and endpoint rows share undeclared classification across all supported languages', () => {
  for (const language of [
    'Python',
    'TypeScript',
    'Go',
    'Java',
    'C#',
    'Ruby',
    'cURL',
  ]) {
    const input = model(['openai']);
    const rows = getModelEndpointRows(input);
    const examples = getModelQuickStartExamples(
      input,
      'https://example.com',
      '',
      language,
      { includeTextProtocolTemplates: true },
    );
    for (const example of examples)
      assert.equal(
        example.declared,
        rows.find((x) => x.endpoint === example.endpoint).declared,
      );
    assert.match(
      examples.find((x) => x.endpoint === 'openai-response').code,
      /\/v1\/responses/,
    );
  }
});

test('Ali TTS exposes POST speech and model-bound GET duplex without chat templates', () => {
  const input = model(['audio-speech', 'ali-tts-stream'], {
    model_name: 'voice/中文',
  });
  const rows = getModelEndpointRows(input);
  assert.deepEqual(
    rows.map(({ method, path }) => ({ method, path })),
    [
      { method: 'POST', path: '/v1/audio/speech' },
      {
        method: 'GET',
        path: '/v1/audio/speech/stream?model=voice%2F%E4%B8%AD%E6%96%87',
      },
    ],
  );
  assert.equal(canOfferTextProtocolExamples(input), false);
  assert.deepEqual(getModelProtocolOptions(input, true), []);
});
