// Stable catalog identifiers are independent of the interface language.
export const MODEL_FUNCTION_LABELS = Object.freeze({
  text_chat: '文本对话',
  image_understanding: '图像理解',
  image_generation: '文生图',
  image_editing: '图像编辑',
  speech_generation: '语音合成',
  speech_recognition: '语音识别',
  realtime_audio: '实时语音',
  video_generation: '视频生成',
  embedding: '向量嵌入',
  rerank: '重排序',
});
export const CATALOG_MODALITIES = Object.freeze({
  text: '文本',
  image: '图片',
  audio: '音频',
  video: '视频',
  file: '文件',
});
export const CATALOG_TAG_LABELS = Object.freeze({
  reasoning: '深度思考',
  tools: '工具调用',
  structured_output: '结构化输出',
  'open weights': '开放权重',
  coding: '编程',
  featured: '精选',
});
const tagAliases = {
  'function calling': 'tools',
  function_calling: 'tools',
  'tool use': 'tools',
  'structured output': 'structured_output',
  'open-weights': 'open weights',
  open_weights: 'open weights',
  code: 'coding',
  thinking: 'reasoning',
};
export const rawCatalogTags = (model) =>
  String(model?.tags || '')
    .split(/[,;|]+/)
    .map((x) => x.trim())
    .filter(Boolean);
export const normalizeCatalogTag = (tag) =>
  tagAliases[String(tag).toLowerCase()] || String(tag).toLowerCase();
const taskTags = {
  'text-generation': 'text_chat',
  'text-chat': 'text_chat',
  'image-generation': 'image_generation',
  'text-to-image': 'image_generation',
  'image-editing': 'image_editing',
  'text-to-speech': 'speech_generation',
  tts: 'speech_generation',
  'speech-to-text': 'speech_recognition',
  asr: 'speech_recognition',
  transcription: 'speech_recognition',
  'realtime-audio': 'realtime_audio',
  'video-generation': 'video_generation',
  'text-to-video': 'video_generation',
  embedding: 'embedding',
  embeddings: 'embedding',
  rerank: 'rerank',
  reranking: 'rerank',
};
export function catalogProtocols(model = {}) {
  if (Array.isArray(model.supported_endpoint_types))
    return model.supported_endpoint_types;
  try {
    const endpoints =
      typeof model.endpoints === 'string'
        ? JSON.parse(model.endpoints)
        : model.endpoints;
    return Array.isArray(endpoints)
      ? endpoints
      : endpoints && typeof endpoints === 'object'
        ? Object.entries(endpoints)
            .filter(
              ([, value]) =>
                typeof value === 'string' ||
                (value !== null &&
                  typeof value === 'object' &&
                  !Array.isArray(value)),
            )
            .map(([key]) => key)
        : [];
  } catch {
    return [];
  }
}
export function automaticModelFunctions(model = {}) {
  const input = model.input_modalities || [],
    output = model.output_modalities || [];
  const tags = rawCatalogTags(model).map((x) => x.toLowerCase());
  const protocols = catalogProtocols(model);
  const functions = new Set(tags.map((x) => taskTags[x]).filter(Boolean));
  const hasProtocol = (...ids) => ids.some((x) => protocols.includes(x));
  if (
    hasProtocol('image-generation') ||
    (input.includes('text') && output.includes('image'))
  )
    functions.add('image_generation');
  if (hasProtocol('image-edit', 'image-editing'))
    functions.add('image_editing');
  if (
    hasProtocol('audio-speech', 'ali-tts-stream') ||
    (input.includes('text') && output.includes('audio'))
  )
    functions.add('speech_generation');
  if (
    hasProtocol(
      'audio-transcription',
      'audio-transcriptions',
      'audio-translation',
    )
  )
    functions.add('speech_recognition');
  if (hasProtocol('realtime')) functions.add('realtime_audio');
  if (hasProtocol('openai-video') || output.includes('video'))
    functions.add('video_generation');
  if (hasProtocol('embedding', 'embeddings')) functions.add('embedding');
  if (hasProtocol('rerank', 'reranking')) functions.add('rerank');
  const specialized = [...functions].some((x) => x !== 'text_chat');
  // A generic Chat endpoint is not proof that ASR/TTS/image models are chat models.
  if (
    !specialized &&
    input.includes('text') &&
    output.includes('text') &&
    hasProtocol('openai', 'openai-response', 'anthropic', 'gemini')
  )
    functions.add('text_chat');
  if (input.includes('image') && output.includes('text') && !specialized)
    functions.add('image_understanding');
  return Object.keys(MODEL_FUNCTION_LABELS).filter((x) => functions.has(x));
}
export function getModelFunctions(model) {
  const found = new Set(automaticModelFunctions(model));
  for (const [key, value] of Object.entries(model?.function_overrides || {})) {
    if (value === true) found.add(key);
    if (value === false) found.delete(key);
  }
  return Object.keys(MODEL_FUNCTION_LABELS).filter((x) => found.has(x));
}
export function getCatalogTags(model) {
  const tags = new Set(
    rawCatalogTags(model)
      .map(normalizeCatalogTag)
      .filter(
        (x) =>
          !/^[\d.]+\s*[kmb]$/i.test(x) &&
          ![
            'audio',
            'image',
            'video',
            'vision',
            'files',
            'file',
            'text',
            'deprecated',
            'glm',
            'qwen',
          ].includes(x) &&
          !taskTags[x],
      ),
  );
  for (const [tag, key] of Object.entries({
    reasoning: 'reasoning',
    tools: 'function_calling',
    structured_output: 'structured_output',
  })) {
    if (model.capabilities?.[key] === true) tags.add(tag);
    if (model.capabilities?.[key] === false) tags.delete(tag);
  }
  return [...tags];
}
export const catalogTagLabel = (tag, t, models = []) =>
  CATALOG_TAG_LABELS[tag]
    ? t(CATALOG_TAG_LABELS[tag])
    : models
        .flatMap(rawCatalogTags)
        .find((raw) => normalizeCatalogTag(raw) === tag) || tag;
const vendorAliases = {
  claude: 'Anthropic',
  anthropic: 'Anthropic',
  openai: 'OpenAI',
  gemini: 'Google',
  google: 'Google',
  deepseek: 'DeepSeek',
  moonshotai: 'Moonshot',
  moonshot: 'Moonshot',
  kimi: 'Moonshot',
  grok: 'xAI',
  xai: 'xAI',
  'z.ai': '智谱',
  zhipu: '智谱',
  chatglm: '智谱',
  智谱: '智谱',
  qwen: '阿里巴巴',
  alibaba: '阿里巴巴',
  xiaomi: '小米 MiMo',
  mimo: '小米 MiMo',
  minimax: 'MiniMax',
};
export const catalogVendor = (value) =>
  vendorAliases[String(value || '').toLowerCase()] || value || 'unknown';
export const filterValues = (value) =>
  [
    ...new Set(
      (Array.isArray(value)
        ? value
        : value && value !== 'all'
          ? [value]
          : []
      ).filter(
        (x) =>
          typeof x === 'string' &&
          x !== 'all' &&
          x.length > 0 &&
          x.length <= 512,
      ),
    ),
  ].slice(0, 64);
export const toggleFilterValue = (value, item) =>
  filterValues(value).includes(item)
    ? filterValues(value).filter((x) => x !== item)
    : [...filterValues(value), item];

// Results, facet counts and health association share this exact predicate.
export function matchesCatalogFilters(model, filters = {}, ignore = []) {
  const check = (key, fn) => ignore.includes(key) || fn();
  const any = (selected, available) =>
    !selected.length || selected.some((x) => available.includes(x));
  const all = (selected, available) =>
    selected.every((x) => available.includes(x));
  return (
    check(
      'group',
      () =>
        !filters.filterGroup ||
        filters.filterGroup === 'all' ||
        model.enable_groups?.includes(filters.filterGroup),
    ) &&
    check(
      'quota',
      () =>
        filters.filterQuotaType == null ||
        filters.filterQuotaType === 'all' ||
        model.quota_type === filters.filterQuotaType,
    ) &&
    check(
      'endpoint',
      () =>
        !filters.filterEndpointType ||
        filters.filterEndpointType === 'all' ||
        model.supported_endpoint_types?.includes(filters.filterEndpointType),
    ) &&
    check('vendor', () =>
      any(filterValues(filters.filterVendor).map(catalogVendor), [
        catalogVendor(model.vendor_name),
      ]),
    ) &&
    check('function', () =>
      any(filterValues(filters.filterFunctions), getModelFunctions(model)),
    ) &&
    check('tag', () =>
      all(
        filterValues(filters.filterTag).map(normalizeCatalogTag),
        getCatalogTags(model),
      ),
    ) &&
    check('input', () =>
      all(
        filterValues(filters.filterInputModalities),
        model.input_modalities || [],
      ),
    ) &&
    check(
      'context',
      () =>
        !filters.filterContextLength ||
        Number(model.context_length) >= filters.filterContextLength,
    ) &&
    check(
      'search',
      () =>
        !filters.searchValue ||
        [
          model.model_name,
          model.display_name,
          model.description,
          model.description_original,
          model.tags,
          model.vendor_name,
          ...getModelFunctions(model).map((x) => MODEL_FUNCTION_LABELS[x]),
          ...getCatalogTags(model),
        ].some((x) =>
          String(x || '')
            .toLowerCase()
            .includes(filters.searchValue.toLowerCase()),
        ),
    )
  );
}

export const hasCatalogFilters = (filters = {}) =>
  [
    'filterVendor',
    'filterTag',
    'filterFunctions',
    'filterInputModalities',
  ].some((key) => filterValues(filters[key]).length > 0) ||
  Boolean(filters.filterContextLength) ||
  (filters.filterQuotaType != null && filters.filterQuotaType !== 'all') ||
  Boolean(filters.filterEndpointType && filters.filterEndpointType !== 'all') ||
  Boolean(filters.searchValue);
