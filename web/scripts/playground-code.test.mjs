import assert from 'node:assert/strict';
import { test } from 'bun:test';
import {
  buildPlaygroundCode,
  convertPlaygroundPayload,
  getPlaygroundSetup,
} from '../src/helpers/playgroundCode';
const source = {
  model: 'test-model',
  group: 'vip',
  stream: true,
  temperature: 0,
  top_p: 1,
  frequency_penalty: 0,
  presence_penalty: 0,
  messages: [
    { role: 'system', content: 'Be helpful' },
    { role: 'user', content: 'Hello' },
  ],
};

test('protocol conversion preserves system, text, zero values and image semantics without mutating input', () => {
  const payload = structuredClone(source);
  payload.messages[1].content = [
    { type: 'text', text: 'Describe' },
    { type: 'image_url', image_url: { url: 'data:image/png;base64,aGVsbG8=' } },
  ];
  const original = JSON.stringify(payload);
  const messages = convertPlaygroundPayload(payload, 'messages');
  assert.deepEqual(messages.errors, []);
  assert.deepEqual(messages.notes, ['max_tokens']);
  assert.deepEqual(messages.body.system, [
    { type: 'text', text: 'Be helpful' },
  ]);
  assert.equal(messages.body.max_tokens, 4096);
  assert.equal(messages.body.temperature, 0);
  assert.equal(messages.body.top_p, undefined);
  assert.deepEqual(messages.body.messages[0].content[1].source, {
    type: 'base64',
    media_type: 'image/png',
    data: 'aGVsbG8=',
  });
  const responses = convertPlaygroundPayload(payload, 'responses');
  assert.deepEqual(responses.errors, []);
  assert.equal(responses.body.input[0].role, 'system');
  assert.equal(responses.body.input[1].content[0].type, 'input_text');
  assert.equal(responses.body.input[1].content[1].type, 'input_image');
  assert.equal(responses.body.temperature, 0);
  assert.equal(JSON.stringify(payload), original);
});

test('tool definitions, parallel calls and results convert into each native protocol', () => {
  const payload = {
    ...source,
    tools: [
      {
        type: 'function',
        function: {
          name: 'weather',
          parameters: { type: 'object', properties: {} },
        },
      },
    ],
    tool_choice: 'required',
    parallel_tool_calls: false,
    messages: [
      ...source.messages,
      {
        role: 'assistant',
        content: null,
        tool_calls: [
          {
            id: 'c1',
            type: 'function',
            function: { name: 'weather', arguments: '{"city":"上海"}' },
          },
          {
            id: 'c2',
            type: 'function',
            function: { name: 'weather', arguments: '{"city":"北京"}' },
          },
        ],
      },
      { role: 'tool', tool_call_id: 'c1', content: 'sunny' },
      { role: 'tool', tool_call_id: 'c2', content: 'rainy' },
    ],
  };
  const native = convertPlaygroundPayload(payload, 'messages');
  assert.deepEqual(native.errors, []);
  assert.deepEqual(native.body.tool_choice, {
    type: 'any',
    disable_parallel_tool_use: true,
  });
  assert.equal(native.body.messages[1].content[1].input.city, '北京');
  assert.equal(native.body.messages[2].content.length, 2);
  assert.equal(native.body.messages[2].content[0].type, 'tool_result');
  const responses = convertPlaygroundPayload(payload, 'responses');
  assert.deepEqual(responses.errors, []);
  assert.equal(responses.body.tools[0].strict, false);
  assert.deepEqual(responses.body.input.at(-1), {
    type: 'function_call_output',
    call_id: 'c2',
    output: 'rainy',
  });
  assert.equal(responses.body.input[2].type, 'function_call');
  assert.equal(responses.body.parallel_tool_calls, false);
  const pending = { ...payload, messages: payload.messages.slice(0, -1) };
  for (const protocol of ['chat', 'responses', 'messages']) {
    assert.ok(
      convertPlaygroundPayload(pending, protocol).errors.includes(
        'tool_results',
      ),
    );
    assert.equal(
      buildPlaygroundCode(pending, 'python', 'https://meta-api.com', protocol),
      '',
    );
  }
});

test('unsupported meaningful fields block conversion; Chat extensions stay in extra_body', () => {
  const payload = {
    ...source,
    seed: 0,
    input: 'custom',
    reasoning_effort: 'medium',
  };
  assert.deepEqual(convertPlaygroundPayload(payload, 'responses').errors, [
    'seed',
    'input',
  ]);
  assert.deepEqual(convertPlaygroundPayload(payload, 'messages').errors, [
    'seed',
    'input',
    'reasoning_effort',
  ]);
  assert.equal(
    buildPlaygroundCode(payload, 'python', 'https://meta-api.com', 'messages'),
    '',
  );
  const code = buildPlaygroundCode(payload, 'python', 'https://meta-api.com');
  assert.match(code, /extra_body=\{/);
  assert.match(code, /"input": "custom"/);
  assert.doesNotMatch(code, /\n    input=/);
  const responses = convertPlaygroundPayload(
    {
      ...source,
      max_tokens: 100,
      reasoning_effort: 'low',
      response_format: {
        type: 'json_schema',
        json_schema: {
          name: 'answer',
          schema: { type: 'object' },
          strict: true,
        },
      },
    },
    'responses',
  );
  assert.deepEqual(responses.errors, []);
  assert.equal(responses.body.max_output_tokens, 100);
  assert.deepEqual(responses.body.reasoning, { effort: 'low' });
  assert.equal(responses.body.text.format.name, 'answer');
});

test('empty conversation uses an explicit example; malformed custom payload never crashes the panel', () => {
  for (const messages of [[], [source.messages[0]]]) {
    const converted = convertPlaygroundPayload({ model: 'test', messages });
    assert.deepEqual(converted.notes, ['example']);
    assert.equal(converted.body.messages.at(-1).content, 'Hello!');
  }
  for (const payload of [
    null,
    {},
    { model: 'test', messages: [null] },
    { model: 'test', messages: [{ role: 'assistant', tool_calls: {} }] },
  ]) {
    assert.ok(convertPlaygroundPayload(payload).errors.length);
    assert.equal(
      buildPlaygroundCode(payload, 'python', 'https://meta-api.com'),
      '',
    );
  }
  assert.ok(
    convertPlaygroundPayload(
      { ...source, messages: [{ role: 'user', content: [null] }] },
      'messages',
    ).errors.length,
  );
  assert.match(
    getPlaygroundSetup('javascript', 'messages'),
    /npm install @anthropic-ai\/sdk/,
  );
  assert.match(
    getPlaygroundSetup('python', 'responses'),
    /python playground.py/,
  );
});
