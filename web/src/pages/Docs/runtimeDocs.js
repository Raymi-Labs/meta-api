/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

// Documentation uses the same visibility and group data as the model plaza.
import { normalizeBaseUrl } from '../../utils/ccSwitch.js';

const ENDPOINTS = {
  openai: { label: 'OpenAI Chat Completions', path: '/v1/chat/completions' },
  'openai-response': { label: 'OpenAI Responses', path: '/v1/responses' },
  anthropic: { label: 'Anthropic Messages', path: '/v1/messages' },
  gemini: {
    label: 'Google Gemini',
    path: '/v1beta/models/{model}:generateContent',
  },
  'image-generation': {
    label: 'OpenAI Images',
    path: '/v1/images/generations',
  },
};

const FAMILIES = [
  ['OpenAI / Codex', /^(gpt-|o[134](?:-|$))/i],
  ['Claude', /^claude-/i],
  ['Gemini', /gemini-/i],
  ['DeepSeek', /^deepseek-/i],
  ['Grok', /^grok-/i],
  ['Kimi', /^kimi-/i],
  ['MiMo', /^mimo-/i],
  ['GLM', /^glm-/i],
];
const NON_TEXT =
  /image|imagen|banana|audio|speech|tts|asr|whisper|embedding|rerank|video/i;
const SAFE_MODEL = /^[a-zA-Z0-9][a-zA-Z0-9._:/-]*$/;
const sortNames = (a, b) => a.localeCompare(b, 'zh-CN', { numeric: true });
const unique = (items) => [...new Set(items)].sort(sortNames);
const strings = (value) =>
  Array.isArray(value) ? value.filter((v) => typeof v === 'string' && v) : [];
const familyOf = (name) => {
  if (/image|imagen|banana/i.test(name)) return '图像模型';
  if (/audio|speech|tts|asr|whisper/i.test(name)) return '语音模型';
  return FAMILIES.find(([, pattern]) => pattern.test(name))?.[0] || '其他模型';
};
const textModel = (model) => !NON_TEXT.test(model.model_name);

// Escape data before inserting it into a Markdown table, never treat it as markup.
const cell = (value) =>
  String(value).replace(/[&<>|`\\*_\[\]\r\n]/g, (c) => `&#${c.charCodeAt(0)};`);
const table = (headers, rows) =>
  [
    `| ${headers.join(' | ')} |`,
    `| ${headers.map(() => '---').join(' | ')} |`,
    ...rows.map((row) => `| ${row.join(' | ')} |`),
  ].join('\n');
const fence = (language, content) => `\`\`\`${language}\n${content}\n\`\`\``;
const shellQuote = (value) => `'${value.replace(/'/g, `'"'"'`)}'`;

export function getDocsBaseUrl(serverAddress, origin) {
  return (normalizeBaseUrl(serverAddress) || normalizeBaseUrl(origin)).replace(
    /['"`$\\()[\]{}<>|]/g,
    (c) => `%${c.charCodeAt(0).toString(16).toUpperCase()}`,
  );
}

export function getDocsModels(pricing) {
  if (!pricing?.success || !Array.isArray(pricing.data)) return [];
  const allowed = new Set(Object.keys(pricing.usable_group || {}));
  return pricing.data
    .filter((m) => typeof m?.model_name === 'string' && m.model_name)
    .map((m) => ({
      model_name: m.model_name,
      enable_groups: unique(
        strings(m.enable_groups).filter((g) => allowed.has(g)),
      ),
      supported_endpoint_types: unique(strings(m.supported_endpoint_types)),
    }))
    .filter((m) => m.enable_groups.length)
    .sort((a, b) => sortNames(a.model_name, b.model_name));
}

export function selectDocsModel(models, endpoint, pattern = /./) {
  return models
    .filter(
      (m) =>
        pattern.test(m.model_name) &&
        SAFE_MODEL.test(m.model_name) &&
        (endpoint === 'image-generation' || textModel(m)) &&
        m.supported_endpoint_types.includes(endpoint),
    )
    .at(-1);
}

export function buildDocsRequest(model, endpoint, baseUrl, stream = false) {
  if (
    !model ||
    !ENDPOINTS[endpoint] ||
    !model.supported_endpoint_types.includes(endpoint)
  )
    return '';
  const name = model.model_name;
  let path = ENDPOINTS[endpoint].path;
  let headers = ['Authorization: Bearer $NAN_API_KEY'];
  let body;
  switch (endpoint) {
    case 'openai':
      body = {
        model: name,
        messages: [{ role: 'user', content: '你好，请简短介绍自己。' }],
      };
      break;
    case 'openai-response':
      body = { model: name, input: '你好，请简短介绍自己。' };
      break;
    case 'anthropic':
      headers = ['x-api-key: $NAN_API_KEY', 'anthropic-version: 2023-06-01'];
      body = {
        model: name,
        max_tokens: 1024,
        messages: [{ role: 'user', content: '你好，请简短介绍自己。' }],
      };
      break;
    case 'gemini':
      path = path.replace('{model}', encodeURIComponent(name));
      headers = ['x-goog-api-key: $NAN_API_KEY'];
      body = {
        contents: [
          { role: 'user', parts: [{ text: '你好，请简短介绍自己。' }] },
        ],
      };
      break;
    case 'image-generation':
      body = { model: name, prompt: '一只骑自行车的橘猫，简洁插画', n: 1 };
      break;
  }
  if (stream && ['openai', 'openai-response', 'anthropic'].includes(endpoint))
    body.stream = true;
  const url = shellQuote(`${baseUrl}${path}`);
  return fence(
    'bash',
    [
      `curl ${stream ? '-N' : '-sS'} ${url} \\`,
      ...headers.map((header) => `  -H "${header}" \\`),
      '  -H "Content-Type: application/json" \\',
      `  -d ${shellQuote(JSON.stringify(body, null, 2))}`,
    ].join('\n'),
  );
}

function groupTable(models) {
  const families = unique(models.map((m) => familyOf(m.model_name)));
  return (
    table(
      ['用途', '当前可选分组'],
      families.map((family) => [
        cell(family),
        unique(
          models
            .filter((m) => familyOf(m.model_name) === family)
            .flatMap((m) => m.enable_groups),
        )
          .map(cell)
          .join('、'),
      ]),
    ) +
    '\n\n同一行的分组不一定支持该厂商的每个模型。请在[模型广场](/pricing)确认目标模型的分组、协议和价格，再创建令牌。这里不按分组名称评价质量或稳定性。'
  );
}

function catalogTable(models) {
  return table(
    ['模型名称', '当前可选分组', '支持协议'],
    models.map((m) => [
      cell(m.model_name),
      m.enable_groups.map(cell).join('、'),
      m.supported_endpoint_types
        .map((e) => cell(ENDPOINTS[e]?.label || e))
        .join('、') || '未声明',
    ]),
  );
}

function requestExamples(models, baseUrl) {
  const examples = [];
  // One Chat sample per available family, plus protocol-specific samples.
  for (const [family, pattern] of FAMILIES) {
    const model = selectDocsModel(models, 'openai', pattern);
    if (model) examples.push([`${family} · Chat Completions`, model, 'openai']);
  }
  for (const endpoint of [
    'openai-response',
    'anthropic',
    'gemini',
    'image-generation',
  ]) {
    const model = selectDocsModel(models, endpoint);
    if (model) examples.push([ENDPOINTS[endpoint].label, model, endpoint]);
  }
  return (
    examples
      .map(
        ([label, model, endpoint]) =>
          `### ${label}\n\n模型：${cell(model.model_name)}。可选分组：${model.enable_groups.map(cell).join('、')}。\n\n${buildDocsRequest(model, endpoint, baseUrl)}`,
      )
      .join('\n\n') ||
    '当前没有可自动生成样例的模型，请到[模型广场](/pricing)查看协议说明。'
  );
}

const MODEL_SLOTS = {
  MODEL_OPENAI: ['openai', /^(gpt-|o[134](?:-|$))/i],
  MODEL_CODEX: ['openai-response', /^(gpt-|o[134](?:-|$))/i],
  MODEL_CLAUDE: ['anthropic', /^claude-/i],
  MODEL_OPENAI_MESSAGES: ['anthropic', /^(gpt-|o[134](?:-|$))/i],
  MODEL_GEMINI: ['gemini', /gemini-/i],
};

export function renderRuntimeDocs(
  markdown,
  { serverAddress, origin, pricing, loading = false } = {},
) {
  const baseUrl = getDocsBaseUrl(serverAddress, origin);
  const models = getDocsModels(pricing);
  const available = pricing?.success && Array.isArray(pricing.data);
  const missing = loading
    ? '正在读取当前模型和分组…'
    : '暂时无法读取当前模型和分组，请点击刷新重试，或到[模型广场](/pricing)查看。';
  const empty =
    '当前身份没有可用的模型和分组，请登录后查看[模型广场](/pricing)，或联系管理员。';
  const firstModel =
    selectDocsModel(models, 'openai', /^gpt-/) ||
    selectDocsModel(models, 'openai');
  const firstRequest = buildDocsRequest(firstModel, 'openai', baseUrl);
  const sections =
    available && models.length
      ? {
          groups: groupTable(models),
          catalog: catalogTable(models),
          examples: requestExamples(models, baseUrl),
          'quick-start': firstRequest
            ? `本例模型：${cell(firstModel.model_name)}。请为令牌选择以下分组之一：${firstModel.enable_groups.map(cell).join('、')}。\n\n${firstRequest.replace('```bash\n', '```bash\nexport NAN_API_KEY="sk-替换为你的API密钥"\n\n')}`
            : empty,
          stream:
            buildDocsRequest(
              selectDocsModel(models, 'openai'),
              'openai',
              baseUrl,
              true,
            ) || empty,
        }
      : {};
  let result = markdown
    .replaceAll('https://meta-api.vip', () => baseUrl)
    .replace(
      /<!-- docs:([a-z-]+) -->[\s\S]*?<!-- \/docs:\1 -->/g,
      (_, section) => sections[section] || (available ? empty : missing),
    );
  const unresolved = [];
  for (const [slot, [endpoint, pattern]] of Object.entries(MODEL_SLOTS).sort(
    ([a], [b]) => b.length - a.length,
  )) {
    const slotPattern = new RegExp(`\\b${slot}\\b`, 'g');
    if (!slotPattern.test(result)) continue;
    slotPattern.lastIndex = 0;
    const model = selectDocsModel(models, endpoint, pattern);
    if (model) result = result.replace(slotPattern, () => model.model_name);
    else unresolved.push(`${slot}（${ENDPOINTS[endpoint].label}）`);
  }
  if (unresolved.length) {
    const notice = available
      ? `当前模型目录未提供与此配置匹配的模型：${unresolved.join('、')}。这些占位符不能直接调用；请在模型广场确认该协议，或联系管理员补齐模型信息后刷新。`
      : `${missing} 配置中的 MODEL_* 是待替换占位符，不能直接调用。`;
    result = result.replace(/^(# [^\n]+\n)/, `$1\n> ${notice}\n`);
  }
  return result;
}
