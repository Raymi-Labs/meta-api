import assert from 'node:assert/strict';
import test from 'node:test';
import fs from 'node:fs';
import {
  automaticModelFunctions,
  catalogProtocols,
  getModelFunctions,
  getCatalogTags,
  catalogTagLabel,
  catalogVendor,
  matchesCatalogFilters,
  MODEL_FUNCTION_LABELS,
  CATALOG_TAG_LABELS,
  filterValues,
  hasCatalogFilters,
} from '../src/helpers/modelCatalogFacets.js';
import { sanitizeModelCatalogSnapshot } from '../src/helpers/modelDetailNavigation.js';
import { omitUnchangedModelMetadata } from '../src/helpers/modelMetadataPatch.js';
const chat = {
  model_name: 'vision-chat',
  display_name: '视觉助手',
  vendor_name: 'OpenAI',
  input_modalities: ['text', 'image'],
  output_modalities: ['text'],
  supported_endpoint_types: ['openai'],
  capabilities: { reasoning: true, function_calling: true },
  context_length: 128000,
  tags: '1m,vision,tools,CustomTag',
  quota_type: 0,
};
const image = {
  model_name: 'image',
  vendor_name: 'Google',
  input_modalities: ['text', 'image'],
  output_modalities: ['image'],
  supported_endpoint_types: ['image-generation'],
  quota_type: 1,
};
const speech = {
  model_name: 'speech',
  vendor_name: '小米 MiMo',
  input_modalities: ['text'],
  output_modalities: ['audio'],
  supported_endpoint_types: ['openai'],
  quota_type: 0,
};
const models = [chat, image, speech];
test('derive tasks conservatively, not from model name or generic Chat endpoints', () => {
  assert.deepEqual(automaticModelFunctions(chat), [
    'text_chat',
    'image_understanding',
  ]);
  assert.deepEqual(automaticModelFunctions(image), ['image_generation']);
  assert.deepEqual(automaticModelFunctions(speech), ['speech_generation']);
  assert.deepEqual(
    automaticModelFunctions({
      model_name: 'text-chat-tts',
      supported_endpoint_types: ['openai'],
    }),
    [],
  );
  assert.deepEqual(
    automaticModelFunctions({
      input_modalities: ['audio'],
      output_modalities: ['text'],
      supported_endpoint_types: ['openai'],
    }),
    [],
  );
  for (const [endpoint, fn] of [
    ['audio-transcription', 'speech_recognition'],
    ['realtime', 'realtime_audio'],
    ['embedding', 'embedding'],
    ['rerank', 'rerank'],
    ['openai-video', 'video_generation'],
    ['image-edit', 'image_editing'],
  ])
    assert.ok(
      automaticModelFunctions({
        supported_endpoint_types: [endpoint],
      }).includes(fn),
    );
});
test('management endpoint JSON and public API derive identical classifications', () => {
  const { supported_endpoint_types, ...metadata } = chat;
  for (const endpoints of [
    '["openai"]',
    '{"openai":{"path":"/v1/chat/completions","method":"POST"}}',
  ])
    assert.deepEqual(
      automaticModelFunctions({ ...metadata, endpoints }),
      automaticModelFunctions(chat),
    );
  assert.deepEqual(automaticModelFunctions({ endpoints: 'invalid' }), []);
  assert.deepEqual(
    catalogProtocols({ endpoints: '{"openai":false,"image-generation":null}' }),
    [],
  );
});
test('manual exclude and include survive metadata changes; empty map restores automatic', () => {
  const overrides = { text_chat: false, image_editing: true };
  assert.deepEqual(
    getModelFunctions({ ...chat, function_overrides: overrides }),
    ['image_understanding', 'image_editing'],
  );
  assert.deepEqual(
    getModelFunctions({ ...image, function_overrides: overrides }),
    ['image_generation', 'image_editing'],
  );
  assert.deepEqual(
    getModelFunctions({ ...chat, function_overrides: {} }),
    automaticModelFunctions(chat),
  );
});
test('tag labels separate tasks, raw modalities/context/families and capability facts', () => {
  assert.deepEqual(
    getCatalogTags({
      ...chat,
      tags: '1m,1.1M,vision,files,glm,qwen,reasoning,tools,open_weights,CustomTag,text-to-image',
      capabilities: { reasoning: false, function_calling: true },
    }),
    ['tools', 'open weights', 'customtag'],
  );
  assert.equal(
    catalogTagLabel('customtag', (x) => x, [chat]),
    'CustomTag',
  );
  assert.equal(catalogVendor('ChatGLM'), catalogVendor('Zhipu'));
  assert.equal(catalogVendor('CustomVendor'), 'CustomVendor');
});
test('OR within provider/function, AND within tag/input and across dimensions', () => {
  const filter = (filters) =>
    models
      .filter((m) => matchesCatalogFilters(m, filters))
      .map((m) => m.model_name);
  assert.deepEqual(filter({ filterVendor: ['OpenAI', 'Google'] }), [
    'vision-chat',
    'image',
  ]);
  assert.deepEqual(
    filter({ filterFunctions: ['text_chat', 'speech_generation'] }),
    ['vision-chat', 'speech'],
  );
  assert.deepEqual(
    filter({
      filterTag: ['tools', 'reasoning'],
      filterInputModalities: ['text', 'image'],
    }),
    ['vision-chat'],
  );
  assert.deepEqual(
    filter({
      filterFunctions: ['speech_generation'],
      filterInputModalities: ['image'],
    }),
    [],
  );
  assert.deepEqual(filter({ filterContextLength: 128000 }), ['vision-chat']);
  assert.deepEqual(filter({ filterContextLength: 256000 }), []);
  assert.deepEqual(filter({ searchValue: '视觉助手' }), ['vision-chat']);
  assert.deepEqual(
    filter({ filterVendor: ['OpenAI'], filterQuotaType: 1 }),
    [],
  );
  const base = models.filter((m) =>
    matchesCatalogFilters(
      m,
      { filterVendor: ['OpenAI'], filterFunctions: ['image_generation'] },
      ['vendor'],
    ),
  );
  assert.deepEqual(base, [image]);
});
test('history retains bounded multi-selects, exact filter semantics and no permission data', () => {
  const state = sanitizeModelCatalogSnapshot({
    filterVendor: ['OpenAI', 'Google'],
    filterFunctions: ['text_chat'],
    filterTag: ['tools', 'reasoning'],
    filterInputModalities: ['text', 'image'],
    filterContextLength: 128000,
    filterGroup: 'private',
    models,
  });
  assert.deepEqual(sanitizeModelCatalogSnapshot(state), state);
  assert.deepEqual(state.filterVendor, ['OpenAI', 'Google']);
  assert.equal(state.filterGroup, 'all');
  assert.equal(state.models, undefined);
  assert.deepEqual(filterValues([null, 'all', 'OpenAI', 'OpenAI', 4]), [
    'OpenAI',
  ]);
  assert.equal(
    filterValues(Array.from({ length: 100 }, (_, i) => `tag${i}`)).length,
    64,
  );
});
test('classification IDs agree with backend whitelist and all 17 locales translate labels', () => {
  const go = fs.readFileSync(
    new URL('../../model/model_function_overrides.go', import.meta.url),
    'utf8',
  );
  for (const key of Object.keys(MODEL_FUNCTION_LABELS))
    assert.ok(go.includes(`"${key}"`), key);
  const dir = new URL('../src/i18n/locales/', import.meta.url);
  const locales = fs.readdirSync(dir).filter((x) => x.endsWith('.json'));
  assert.equal(locales.length, 17);
  for (const lang of locales) {
    const { translation: t } = JSON.parse(fs.readFileSync(new URL(lang, dir)));
    for (const key of [
      ...Object.values(MODEL_FUNCTION_LABELS),
      ...Object.values(CATALOG_TAG_LABELS),
      '广场功能分类',
      '广场展示预览',
      '功能',
      '已选条件',
      '自动使用模型资料；包含或排除为人工设置，不会被同步覆盖。',
    ])
      assert.ok(t[key], `${lang}: ${key}`);
  }
});

test('saving unrelated metadata omits stale function overrides and explicit reset is retained', () => {
  const initial = { function_overrides: { text_chat: false } },
    unchanged = { ...initial };
  assert.equal(
    omitUnchangedModelMetadata(unchanged, unchanged, initial)
      .function_overrides,
    undefined,
  );
  const reset = { function_overrides: {} };
  assert.deepEqual(
    omitUnchangedModelMetadata(reset, reset, initial).function_overrides,
    {},
  );
});

test('health narrowing follows each dimension and clearing multi-selects restores unfiltered mode', () => {
  for (const filters of [
    { filterFunctions: ['text_chat'] },
    { filterInputModalities: ['image'] },
    { filterContextLength: 128000 },
  ])
    assert.equal(hasCatalogFilters(filters), true);
  assert.equal(
    hasCatalogFilters({
      filterVendor: [],
      filterTag: [],
      filterFunctions: [],
      filterInputModalities: [],
    }),
    false,
  );
});
