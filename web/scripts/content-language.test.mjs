import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { marked } from 'marked';
import {
  extractMarkdownSources,
  hasValidContentTranslation,
  translateContentText,
  translateMarkdown,
  translateConfiguredDocument,
} from '../src/helpers/contentLanguage.js';
import { renderRuntimeDocs } from '../src/pages/Docs/runtimeDocs.js';

const fixture = `# 平台文档

[快速开始](#快速开始)

## 快速开始

1. 点击[模型广场](/pricing)，使用 \`gpt-test\` 和 \`https://meta-api.vip/v1\`。

| 说明 | 请求 |
| --- | --- |
| 管道测试 | \`echo a|cat\` |
| 分隔符 | A\\|B |

<!-- docs:examples -->
动态样例
<!-- /docs:examples -->

\`\`\`json
{"model":"gpt-test", "prompt":"不应翻译", "url":"https://meta-api.vip/v1"}
\`\`\`

~~~bash
printf '不应翻译'
~~~
`;

function dictionaryFor(source, transform = (key) => `EN ${key}`) {
  return Object.fromEntries(
    extractMarkdownSources(source).map((key) => [key, transform(key)]),
  );
}

test('prose extraction and restoration preserve code, destinations, tables and runtime markers', () => {
  const keys = extractMarkdownSources(fixture);
  assert.ok(keys.includes('平台文档'));
  assert.ok(keys.includes('管道测试'));
  assert.ok(!keys.some((key) => key.includes('不应翻译')));
  assert.ok(!keys.some((key) => key.includes('https://meta-api.vip')));
  const translated = translateMarkdown(
    fixture,
    dictionaryFor(
      fixture,
      (key) => `EN ${key.replaceAll('快速开始', 'Quick start')}`,
    ),
  );
  assert.match(translated, /# EN 平台文档/);
  assert.match(translated, /EN \[Quick start\]\(#en-quick-start\)/);
  assert.match(translated, /\]\(\/pricing\)/);
  assert.match(translated, /`gpt-test` 和 `https:\/\/meta-api.vip\/v1`/);
  assert.match(translated, /`echo a\|cat`/);
  assert.match(translated, /A\\\|B/);
  assert.match(translated, /<!-- docs:examples -->/);
  assert.match(
    translated,
    /```json\n\{"model":"gpt-test", "prompt":"不应翻译", "url":"https:\/\/meta-api.vip\/v1"\}\n```/,
  );
  assert.match(translated, /~~~bash\nprintf '不应翻译'\n~~~/);
});

test('malformed placeholders reject a translated segment instead of corrupting commands or links', () => {
  const source = '打开 `https://meta-api.vip/v1`，查看[文档](/docs)。';
  const [key] = extractMarkdownSources(source);
  assert.equal(
    translateMarkdown(source, { [key]: 'Invalid {{CONTENT_99}}' }),
    source,
  );
  assert.equal(
    translateMarkdown(source, { [key]: 'Missing all placeholders' }),
    source,
  );
  assert.equal(
    hasValidContentTranslation('模型 {{model}}', '{{model}} {{model}}'),
    false,
  );
  assert.equal(
    translateContentText(
      '模型 {{model}}',
      { '模型 {{model}}': 'Model {{model}}' },
      { model: 'gpt-test' },
    ),
    'Model gpt-test',
  );
});

test('emphasis restores valid source boundaries despite translated token padding', () => {
  const source = '正文 **加粗**、*斜体*、~~删除~~、__粗体__ 和 _倾斜_ 测试。';
  const dictionary = dictionaryFor(source, (key) =>
    key
      .replaceAll('加粗', 'bold')
      .replaceAll('斜体', 'italic')
      .replaceAll('删除', 'removed')
      .replaceAll('粗体', 'strong')
      .replaceAll('倾斜', 'emphasis')
      .replace(/\{\{CONTENT_\d+\}\}/g, ' $& '),
  );
  const translated = translateMarkdown(source, dictionary);
  const html = marked.parseInline(translated);
  assert.match(html, /<strong>bold<\/strong>/);
  assert.match(html, /<em>italic<\/em>/);
  assert.match(html, /<del>removed<\/del>/);
  assert.match(html, /<strong>strong<\/strong>/);
  assert.match(html, /<em>emphasis<\/em>/);
  // Whitespace outside the paired markers belongs to the translation.
  assert.match(translated, /正文  \*\*bold\*\* 、/);
});

test('nested emphasis survives without trimming code, URLs, literals or unrelated spaces', () => {
  const source = [
    '**外层 *内层* 内容** 和 ***双层***、~~*删除*~~。',
    '**包含 ` 代码 ** 原样 ` 和 [链接](https://example.com/a_b?q=**value**) 的内容**。',
    '普通 ** 星号 **、a_b_c、未配对 * 标记 和 `** 代码 **`。',
    '```js',
    'const example = "** 不修改代码 **";',
    '```',
  ].join('\n');
  const dictionary = dictionaryFor(source, (key) =>
    key
      .replaceAll('外层', 'outer')
      .replaceAll('内层', 'inner')
      .replaceAll('双层', 'nested')
      .replaceAll('删除', 'removed')
      .replaceAll('内容', 'content')
      .replaceAll('链接', 'link')
      .replace(/\{\{CONTENT_\d+\}\}/g, ' $& '),
  );
  const translated = translateMarkdown(source, dictionary);
  const html = marked.parse(translated);
  assert.match(html, /<strong>outer\s+<em>inner<\/em>\s+content<\/strong>/);
  assert.match(html, /<em><strong>nested<\/strong><\/em>/);
  assert.match(html, /<del><em>removed<\/em><\/del>/);
  assert.match(translated, /` 代码 \*\* 原样 `/);
  assert.match(translated, /https:\/\/example.com\/a_b\?q=\*\*value\*\*/);
  assert.match(translated, /普通  \*\*  星号  \*\* /);
  assert.match(translated, /未配对  \*  标记/);
  assert.match(translated, /`\*\* 代码 \*\*`/);
  assert.match(
    translated,
    /```js\nconst example = "\*\* 不修改代码 \*\*";\n```/,
  );
});

test('the current English agreement renders protected emphasis as actual strong text', () => {
  const originals = JSON.parse(
    readFileSync(
      new URL('../src/helpers/contentLanguageSources.json', import.meta.url),
      'utf8',
    ),
  );
  const dictionary = JSON.parse(
    readFileSync(
      new URL('../src/helpers/contentLocales/en.json', import.meta.url),
      'utf8',
    ),
  );
  const source = originals.user_agreement
    .split('\n')
    .find((line) => line.includes('**第三方 API 管理与接入服务平台**'));
  assert.ok(source);
  const translated = translateMarkdown(source, dictionary);
  assert.match(
    marked.parseInline(translated),
    /<strong>third-party API management and access service platform<\/strong>/,
  );
});

test('only exact interface labels inside code spans translate, including spans without surrounding prose', () => {
  const source = [
    '`控制台 -> 令牌管理`',
    '点击 `添加令牌`，选择 `分组` 并复制 `密钥`。',
    '`钱包管理 -> 账单 -> 发票`',
    '`控制台 -> 使用日志`',
    '`echo "添加令牌"`',
    '`/home/你的用户名/.openclaw/openclaw.json`',
    '`冰糖`',
  ].join('\n');
  const labels = {
    '控制台 -> 令牌管理': 'Console -> Token Management',
    添加令牌: 'Create token',
    分组: 'Group',
    密钥: 'Key',
    '钱包管理 -> 账单 -> 发票': 'Wallet Management -> Bills -> Invoice',
    '控制台 -> 使用日志': 'Console -> Usage Logs',
  };
  const keys = extractMarkdownSources(source);
  for (const key of Object.keys(labels)) assert.ok(keys.includes(key), key);
  assert.ok(!keys.some((key) => key.includes('echo') || key.includes('冰糖')));
  const translated = translateMarkdown(source, labels);
  assert.match(translated, /`Console -> Token Management`/);
  assert.match(translated, /点击 `Create token`，选择 `Group` 并复制 `Key`。/);
  assert.match(translated, /`Wallet Management -> Bills -> Invoice`/);
  assert.match(translated, /`Console -> Usage Logs`/);
  assert.match(translated, /`echo "添加令牌"`/);
  assert.match(translated, /`\/home\/你的用户名\/\.openclaw\/openclaw.json`/);
  assert.match(translated, /`冰糖`/);
});

test('explicitly marked support templates translate labels and choices but preserve example values and code', () => {
  const source = [
    '```text i18n-template',
    '账号邮箱/用户ID：',
    '使用客户端：Codex / Claude Code / Gemini / OpenCode / OpenClaw / API',
    '是否流式请求：是/否',
    '模型名：模型保持原样',
    '```',
    '```text',
    '账号邮箱/用户ID：不翻译',
    '```',
    '```json',
    '{"添加令牌": "不翻译"}',
    '```',
  ].join('\n');
  const dictionary = {
    '账号邮箱/用户ID：': 'Account email/User ID:',
    '使用客户端：': 'Client:',
    '是否流式请求：': 'Streaming request:',
    '是/否': 'Yes/No',
    '模型名：': 'Model name:',
  };
  assert.deepEqual(
    new Set(extractMarkdownSources(source)),
    new Set(Object.keys(dictionary)),
  );
  const translated = translateMarkdown(source, dictionary);
  assert.match(translated, /Account email\/User ID:/);
  assert.match(
    translated,
    /Client:Codex \/ Claude Code \/ Gemini \/ OpenCode \/ OpenClaw \/ API/,
  );
  assert.match(translated, /Streaming request:Yes\/No/);
  assert.match(translated, /Model name:模型保持原样/);
  assert.match(translated, /```text\n账号邮箱\/用户ID：不翻译\n```/);
  assert.match(translated, /```json\n\{"添加令牌": "不翻译"\}\n```/);
});

test('configured agreements translate only the exact current source with a complete matching dictionary', () => {
  const originals = { user_agreement: '# 用户协议\n\n这是协议正文。\n' };
  const source = originals.user_agreement;
  const dictionary = dictionaryFor(source);
  const result = translateConfiguredDocument(
    source,
    'user_agreement',
    dictionary,
    originals,
    'de',
  );
  assert.equal(result.translated, true);
  assert.match(result.content, /EN 这是协议正文。/);
  for (const changed of [
    source + '\n新条款。',
    '# 管理员新的内容',
    'https://example.com/current-terms',
    '<h1>管理员的 HTML</h1>',
  ]) {
    assert.deepEqual(
      translateConfiguredDocument(
        changed,
        'user_agreement',
        dictionary,
        originals,
        'de',
      ),
      { content: changed, translated: false, original: true },
    );
  }
  assert.equal(
    translateConfiguredDocument(source, 'user_agreement', {}, originals, 'de')
      .content,
    source,
  );
  assert.deepEqual(
    translateConfiguredDocument(
      source,
      'user_agreement',
      null,
      originals,
      'zh-CN',
    ),
    { content: source, translated: false, original: false },
  );
});

test('runtime docs translate labels but preserve permission-filtered model/group data and executable examples', () => {
  const source =
    '# 平台文档\n<!-- docs:quick-start -->旧内容<!-- /docs:quick-start -->\n<!-- docs:groups -->旧内容<!-- /docs:groups -->';
  const pricing = {
    success: true,
    usable_group: { 公开组: 'public' },
    data: [
      {
        model_name: 'gpt-test',
        enable_groups: ['公开组', '隐藏组'],
        supported_endpoint_types: ['openai'],
      },
    ],
  };
  const translatedKeys = [];
  const translated = renderRuntimeDocs(source, {
    serverAddress: 'https://meta-api.vip',
    origin: 'https://example.test',
    pricing,
    translate: (key, values) => {
      translatedKeys.push(key);
      return translateContentText(key, { [key]: 'EN ' + key }, values);
    },
  });
  assert.ok(translatedKeys.includes('当前可选分组'));
  assert.match(translated, /EN 本例模型：gpt-test/);
  assert.match(translated, /公开组/);
  assert.doesNotMatch(translated, /隐藏组/);
  assert.match(
    translated,
    /curl -sS 'https:\/\/meta-api.vip\/v1\/chat\/completions'/,
  );
  assert.match(translated, /"model": "gpt-test"/);
});

test('all bundled Markdown bodies preserve executable and unmarked fenced code blocks through translation', () => {
  const docs = new URL('../../docs/', import.meta.url);
  for (const name of readdirSync(docs).filter((name) =>
    /^NAN_.*\.md$/.test(name),
  )) {
    const source = readFileSync(new URL(name, docs), 'utf8');
    const translated = translateMarkdown(source, dictionaryFor(source));
    const fences = (text) =>
      [...text.matchAll(/^```([^\n]*)\n[\s\S]*?^```\s*$/gm)]
        .filter(
          (match) => !/^(?:text|plaintext)\s+i18n-template$/.test(match[1]),
        )
        .map((match) => match[0]);
    assert.deepEqual(fences(translated), fences(source), name);
  }
});

test('the shipped English guide translates actual navigation and copyable support templates', () => {
  const dictionary = JSON.parse(
    readFileSync(
      new URL('../src/helpers/contentLocales/en.json', import.meta.url),
      'utf8',
    ),
  );
  const source = readFileSync(
    new URL('../../docs/NAN_USAGE.md', import.meta.url),
    'utf8',
  );
  const translated = translateMarkdown(source, dictionary);
  assert.match(translated, /`Console -> Token Management`/);
  assert.match(translated, /`Create token`/);
  assert.match(translated, /`Console -> Wallet Management`/);
  assert.match(translated, /`Wallet Management -> Bills -> Invoice`/);
  assert.match(translated, /`Console -> Usage Logs`/);
  assert.match(translated, /AI Image Generation/);
  assert.doesNotMatch(translated, /AI raw picture|model square/i);
  const template = translated.match(
    /^```text i18n-template\n[\s\S]*?^```/m,
  )?.[0];
  assert.ok(template);
  assert.doesNotMatch(template, /[\u3400-\u9fff]/u);
  assert.match(template, /Account email\/User ID:/);
  assert.match(template, /Streaming request:Yes\/No/);
  assert.match(
    template,
    /Codex \/ Claude Code \/ Gemini \/ OpenCode \/ OpenClaw \/ API/,
  );
});

// Run after content dictionaries are generated. A changed shipped paragraph must
// receive translations instead of silently falling back to Chinese forever.
import { collectContentSources } from './content-language-sources.mjs';
const contentLanguages = [
  'zh-TW',
  'en',
  'ja',
  'ko',
  'de',
  'es',
  'fr',
  'pt',
  'ru',
  'tr',
  'pl',
  'nl',
  'it',
  'vi',
  'uk',
  'bg',
];

test('every supported language covers shipped documents without lost placeholders or temporary translation markers', () => {
  const sources = collectContentSources();
  assert.ok(Object.keys(sources).length > 900);
  for (const language of contentLanguages) {
    const dictionary = JSON.parse(
      readFileSync(
        new URL(
          `../src/helpers/contentLocales/${language}.json`,
          import.meta.url,
        ),
        'utf8',
      ),
    );
    for (const source of Object.keys(sources)) {
      assert.ok(
        hasValidContentTranslation(source, dictionary[source]),
        `${language}: ${source}`,
      );
    }
    assert.deepEqual(
      Object.entries(dictionary)
        .filter(([, value]) => /ZX[QVE]\s*\d{5}/i.test(value))
        .map(([source]) => source),
      [],
      `${language}: temporary translation markers must not be shipped`,
    );
  }
});
