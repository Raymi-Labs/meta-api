import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import test from 'node:test';
import {
  buildDocsRequest,
  getDocsBaseUrl,
  getDocsModels,
  renderRuntimeDocs,
  selectDocsModel,
} from '../src/pages/Docs/runtimeDocs.js';

const model = (name, groups, endpoints) => ({
  model_name: name,
  enable_groups: groups,
  supported_endpoint_types: endpoints,
});
const pricing = {
  success: true,
  usable_group: { 'OpenAI · 优质': '公开', 'Claude · 优质': '公开' },
  data: [
    model('gpt-6-test', ['OpenAI · 优质', 'private-only'], ['openai']),
    model('gpt-7-test', ['OpenAI · 优质'], ['openai-response']),
    model('claude-test', ['Claude · 优质'], ['anthropic', 'openai']),
    model('hidden-model', ['private-only'], ['openai']),
    model('gpt-image-test', ['OpenAI · 优质'], ['openai', 'image-generation']),
  ],
};
const section = (name) =>
  `<!-- docs:${name} -->stale value<!-- /docs:${name} -->`;
const options = {
  pricing,
  serverAddress: 'https://configured.example/v1/',
  origin: 'https://browser.example',
};

test('server config wins; protocol suffix is normalized; fallback and unsafe characters are handled', () => {
  assert.equal(
    getDocsBaseUrl(options.serverAddress, options.origin),
    'https://configured.example',
  );
  assert.equal(
    getDocsBaseUrl('javascript:alert(1)', options.origin),
    options.origin,
  );
  assert.equal(
    getDocsBaseUrl('https://user:secret@example.com/gateway/v1/?a=b#c', ''),
    'https://example.com/gateway',
  );
  assert.doesNotMatch(
    getDocsBaseUrl("https://example.com/'$(id)", ''),
    /['$()]/,
  );
  const result = renderRuntimeDocs(
    'https://meta-api.vip/v1 https://docs.newapi.pro/zh/docs',
    options,
  );
  assert.equal(
    result,
    'https://configured.example/v1 https://docs.newapi.pro/zh/docs',
  );
});

test('models and groups honor current usable groups without leaking unavailable membership', () => {
  const models = getDocsModels(pricing);
  assert.equal(models.length, 4);
  assert.deepEqual(
    models.find((m) => m.model_name === 'gpt-6-test').enable_groups,
    ['OpenAI · 优质'],
  );
  const result = renderRuntimeDocs(
    section('groups') + section('catalog'),
    options,
  );
  assert.match(result, /OpenAI · 优质/);
  assert.doesNotMatch(result, /private-only|hidden-model|stale value/);
  assert.equal(getDocsModels({ ...pricing, usable_group: {} }).length, 0);
});

test('selected models match protocol and modality, never infer Responses from Chat', () => {
  const models = getDocsModels(pricing);
  assert.equal(
    selectDocsModel(models, 'openai', /^gpt-/).model_name,
    'gpt-6-test',
  );
  assert.equal(
    selectDocsModel(models, 'openai-response', /^gpt-/).model_name,
    'gpt-7-test',
  );
  assert.equal(selectDocsModel(models, 'anthropic', /^gpt-/), undefined);
  assert.equal(buildDocsRequest(models[0], 'gemini', options.origin), '');
  const result = renderRuntimeDocs(
    '# Config\nMODEL_OPENAI_MESSAGES MODEL_OPENAI MODEL_CODEX MODEL_CLAUDE',
    options,
  );
  assert.match(result, /MODEL_OPENAI_MESSAGES/);
  assert.doesNotMatch(result, /gpt-6-test_MESSAGES/);
  assert.match(result, /gpt-6-test gpt-7-test claude-test/);
  assert.match(result, /占位符不能直接调用/);
});

test('copied curl commands preserve JSON, URL, API key header and shell quoting across protocols', () => {
  const name = "gemini-[特价]'$(echo unsafe)";
  for (const endpoint of [
    'openai',
    'openai-response',
    'anthropic',
    'gemini',
    'image-generation',
  ]) {
    const sample = model(name, ['public'], [endpoint]);
    const fenced = buildDocsRequest(
      sample,
      endpoint,
      'https://example.com',
      endpoint === 'openai',
    );
    const command = fenced.replace(/^```bash\n/, '').replace(/\n```$/, '');
    // Shadow curl to inspect actual shell arguments; never send a billable request.
    const result = spawnSync(
      'bash',
      ['-c', 'curl() { printf "%s\\0" "$@"; };\n' + command],
      {
        env: { ...process.env, NAN_API_KEY: 'sk-test-only' },
        encoding: 'utf8',
      },
    );
    assert.equal(result.status, 0, result.stderr);
    const args = result.stdout.split('\0').filter(Boolean);
    const body = JSON.parse(args[args.indexOf('-d') + 1]);
    if (endpoint === 'gemini') {
      assert.ok(
        args.includes(
          `https://example.com/v1beta/models/${encodeURIComponent(name)}:generateContent`,
        ),
      );
      assert.equal(body.contents[0].parts[0].text, '你好，请简短介绍自己。');
    } else assert.equal(body.model, name);
    if (endpoint === 'openai') assert.equal(body.stream, true);
    assert.ok(args.some((arg) => arg.includes('sk-test-only')));
  }
});

test('empty, failed and loading states do not fall back to old groups or models', () => {
  for (const [data, loading, expected] of [
    [null, true, /正在读取/],
    [null, false, /暂时无法读取/],
    [{ success: true, data: [], usable_group: {} }, false, /没有可用/],
  ]) {
    const result = renderRuntimeDocs(
      '# Guide\n注册和充值\n' + section('groups'),
      { ...options, pricing: data, loading },
    );
    assert.match(result, expected);
    assert.match(result, /注册和充值/);
    assert.doesNotMatch(result, /stale value|private-only/);
  }
});

test('group and model metadata is escaped in tables; unsafe model names are not used in inline configs', () => {
  const group = 'bad|group\n[link](https://evil.example)`';
  const data = {
    success: true,
    usable_group: { [group]: '' },
    data: [model('gpt-<script>$bad', [group], ['openai'])],
  };
  const result = renderRuntimeDocs(
    '# Config\nMODEL_OPENAI\n' + section('catalog'),
    { ...options, pricing: data },
  );
  assert.doesNotMatch(result, /<script>|bad\|group|\[link\]/);
  assert.match(result, /MODEL_OPENAI/);
});

test('all documentation has current origins, valid JSON samples and no subscription sales or stale group screenshot', () => {
  const docsDir = new URL('../../docs/', import.meta.url);
  for (const name of readdirSync(docsDir).filter((name) =>
    /^NAN_.*\.md$/.test(name),
  )) {
    const source = readFileSync(new URL(name, docsDir), 'utf8');
    assert.doesNotMatch(
      source,
      /https:\/\/(cn|nan)\.meta-api\.vip|`vip`|`svip`|nan-token-management-overview\.png|购买订阅|订阅套餐|订阅管理/,
      name,
    );
    const rendered = renderRuntimeDocs(source, options);
    assert.doesNotMatch(rendered, /<!-- docs:/, name);
    for (const match of rendered.matchAll(/```json\n([\s\S]*?)\n```/g)) {
      assert.doesNotThrow(
        () => JSON.parse(match[1]),
        `${name}: invalid JSON example`,
      );
    }
  }
});
