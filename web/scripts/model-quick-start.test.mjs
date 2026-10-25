import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import test from 'node:test';
import vm from 'node:vm';
import {
  getModelQuickStartExamples,
  MODEL_EXAMPLE_LANGUAGES,
} from '../src/pages/ModelDetail/modelQuickStartExamples.js';
import { buildDocsRequestData } from '../src/pages/Docs/runtimeDocs.js';

const model = (name, protocols) => ({
  model_name: name,
  supported_endpoint_types: protocols,
});

const hostileName = `org/model%2Fpart'$(echo unsafe)"\\newline\n尾😀 #{raise 'unsafe'}\u2028`;
const protocols = [
  'openai',
  'openai-response',
  'anthropic',
  'gemini',
  'image-generation',
];
const declaredModel = model(hostileName, protocols);
const generate = (language) =>
  getModelQuickStartExamples(
    declaredModel,
    'https://example.com/v1',
    '',
    language,
  );
const expectedRequest = (endpoint) => {
  const { headers, ...request } = buildDocsRequestData(
    declaredModel,
    endpoint,
    'https://example.com',
  );
  return {
    ...request,
    headers: Object.fromEntries(
      [...headers, 'Content-Type: application/json'].map((header) => {
        const colon = header.indexOf(':');
        return [
          header.slice(0, colon).toLowerCase(),
          header
            .slice(colon + 1)
            .trim()
            .replace('$NAN_API_KEY', 'YOUR_API_KEY'),
        ];
      }),
    ),
  };
};
const lowerHeaders = (headers) =>
  Object.fromEntries(
    Object.entries(headers).map(([key, value]) => [key.toLowerCase(), value]),
  );

test('each language offers the same declared protocols and never returns an empty snippet', () => {
  assert.equal(MODEL_EXAMPLE_LANGUAGES.length, 7);
  for (const language of MODEL_EXAMPLE_LANGUAGES) {
    const examples = generate(language);
    assert.deepEqual(
      examples.map((example) => example.endpoint),
      protocols,
    );
    for (const example of examples) {
      assert.ok(example.code.trim());
      assert.match(example.code, /YOUR_API_KEY/);
      assert.doesNotMatch(example.code, /sk-[a-zA-Z0-9]{10}/);
    }
    assert.deepEqual(
      getModelQuickStartExamples(
        model('gpt-test', ['video-generation']),
        'https://example.com',
        '',
        language,
      ),
      [],
    );
  }
  assert.deepEqual(generate('unknown'), []);
});

test('TypeScript fetch examples preserve request data without evaluating model text', async () => {
  for (const example of generate('TypeScript')) {
    let captured;
    await vm.runInNewContext(`(async () => {${example.code}\n})()`, {
      fetch: async (url, options) => {
        assert.equal(options.method, 'POST');
        captured = {
          url,
          body: JSON.parse(options.body),
          headers: lowerHeaders(options.headers),
        };
        return { ok: true, json: async () => ({}) };
      },
      console: { log() {} },
    });
    assert.deepEqual(captured, expectedRequest(example.endpoint));
  }
});

test('Python examples preserve every protocol and model string with requests stubbed', (context) => {
  if (spawnSync('python3', ['--version']).status !== 0) {
    context.skip('Python is not installed');
    return;
  }
  const stub = `import sys, types, json as json_module
def capture(url, headers, json, timeout):
    print("REQUEST:" + json_module.dumps({"url": url, "body": json, "headers": headers}))
    return types.SimpleNamespace(raise_for_status=lambda: None, json=lambda: {})
sys.modules["requests"] = types.SimpleNamespace(post=capture)
`;
  for (const example of generate('Python')) {
    const result = spawnSync('python3', ['-c', stub + example.code], {
      encoding: 'utf8',
    });
    assert.equal(result.status, 0, result.stderr);
    const captured = JSON.parse(result.stdout.split('\n')[0].slice(8));
    captured.headers = lowerHeaders(captured.headers);
    assert.deepEqual(captured, expectedRequest(example.endpoint));
  }
});

test('Ruby examples suppress interpolation and preserve requests with HTTP stubbed', (context) => {
  if (spawnSync('ruby', ['--version']).status !== 0) {
    context.skip('Ruby is not installed');
    return;
  }
  const stub = `require "net/http"
require "json"
class << Net::HTTP
def start(*args, **options)
  capture = Object.new
  capture.define_singleton_method(:request) do |request|
    puts "REQUEST:" + JSON.generate({url: request.uri.to_s, body: JSON.parse(request.body), headers: request.each_header.to_h})
    Struct.new(:body).new("{}")
  end
  yield capture
end
end
`;
  for (const example of generate('Ruby')) {
    const result = spawnSync('ruby', ['-e', stub + example.code], {
      encoding: 'utf8',
    });
    assert.equal(result.status, 0, result.stderr);
    const captured = JSON.parse(result.stdout.split('\n')[0].slice(8));
    const expected = expectedRequest(example.endpoint);
    // Net::HTTP adds its own Accept/User-Agent defaults.
    for (const key of Object.keys(captured.headers)) {
      if (!(key in expected.headers)) delete captured.headers[key];
    }
    assert.deepEqual(captured, expected);
  }
});

test('Go, Java and C# payload literals retain JSON escaping and exact URLs', () => {
  const stringLiteral = '"(?:\\\\.|[^"\\\\])*"';
  const patterns = {
    Go: {
      body: new RegExp(`payload := strings.NewReader\\((${stringLiteral})\\)`),
      url: new RegExp(
        `http.NewRequest\\("POST", (${stringLiteral}), payload\\)`,
      ),
    },
    Java: {
      body: new RegExp(`String payload = (${stringLiteral});`),
      url: new RegExp(`URI.create\\((${stringLiteral})\\)`),
    },
    'C#': {
      body: new RegExp(`string payload = (${stringLiteral});`),
      url: new RegExp(`HttpMethod.Post, (${stringLiteral})\\)`),
    },
  };
  for (const [language, pattern] of Object.entries(patterns)) {
    for (const example of generate(language)) {
      const expected = expectedRequest(example.endpoint);
      const body = example.code.match(pattern.body)?.[1];
      const url = example.code.match(pattern.url)?.[1];
      assert.ok(body, `${language} has a safely quoted payload`);
      assert.ok(url, `${language} has a safely quoted URL`);
      assert.deepEqual(JSON.parse(JSON.parse(body)), expected.body);
      assert.equal(JSON.parse(url), expected.url);
      for (const [header, value] of Object.entries(expected.headers)) {
        if (language !== 'C#' || header !== 'content-type') {
          assert.ok(example.code.toLowerCase().includes(header));
        }
        assert.ok(example.code.includes(value));
      }
    }
  }
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
