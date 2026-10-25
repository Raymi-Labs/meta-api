import assert from 'node:assert/strict';
import { readFileSync, realpathSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'bun:test';
import { marked } from 'marked';

const handbookRoot = fileURLToPath(
  new URL('../../docs/handbook/', import.meta.url),
);
const catalogNames = [
  'guide-catalog.json',
  'console-catalog.json',
  'api-catalog.json',
  'integrations-catalog.json',
];

// These public routes predate the handbook and remain valid link destinations.
// Keep this compatibility contract independent of the frontend catalog loader.
const legacyRoutes = new Map([
  ['usage', '/docs'],
  ['clients', '/docs/clients'],
  ['client-codex', '/docs/clients/codex'],
  ['client-claude-code', '/docs/clients/claude-code'],
  ['client-claude-code-openai', '/docs/clients/claude-code-openai'],
  ['client-gemini', '/docs/clients/gemini'],
  ['client-opencode', '/docs/clients/opencode'],
  ['client-openclaw', '/docs/clients/openclaw'],
  ['client-ccswitch', '/docs/clients/ccswitch'],
  ['image-generation', '/docs/image-generation'],
  ['mimo-audio', '/docs/mimo-audio'],
  ['api-examples', '/docs/api-examples'],
  ['troubleshooting', '/docs/troubleshooting'],
]);

function readJSON(name) {
  const file = path.join(handbookRoot, name);
  let source;
  try {
    source = readFileSync(file, 'utf8');
  } catch (error) {
    throw new Error(
      `Required handbook file is missing or unreadable: ${name}`,
      {
        cause: error,
      },
    );
  }
  try {
    return JSON.parse(source);
  } catch (error) {
    throw new Error(`Invalid handbook JSON: ${name}`, { cause: error });
  }
}

function catalogs() {
  return catalogNames.flatMap((name) => {
    const rows = readJSON(name);
    assert.ok(
      Array.isArray(rows) && rows.length,
      `${name}: expected a nonempty array`,
    );
    return rows.map((row) => ({ ...row, catalog: name }));
  });
}

function readArticle(row) {
  assert.equal(
    typeof row.file,
    'string',
    `${row.catalog}: file must be a string`,
  );
  assert.ok(
    !path.isAbsolute(row.file) &&
      !row.file.includes('\\') &&
      row.file.endsWith('.md') &&
      row.file
        .split('/')
        .every((part) => part && part !== '.' && part !== '..'),
    `${row.catalog}: unsafe or non-Markdown file path ${row.file}`,
  );
  const file = path.resolve(handbookRoot, row.file);
  const resolved = realpathSync(file);
  assert.ok(
    resolved.startsWith(`${realpathSync(handbookRoot)}${path.sep}`),
    `${row.file}: symlink escapes the handbook directory`,
  );
  return readFileSync(resolved, 'utf8');
}

function allRoutes(rows) {
  return new Set([...legacyRoutes.values(), ...rows.map((row) => row.path)]);
}

function normalizedDocPath(href) {
  let url;
  try {
    url = new URL(href, 'https://meta-api.vip');
  } catch {
    return null;
  }
  if (url.hostname === 'docs.meta-api.vip') {
    const pathname = url.pathname.replace(/\/+$/, '') || '/';
    return pathname === '/'
      ? '/docs'
      : /^\/docs(?:\/|$)/.test(pathname)
        ? pathname
        : `/docs${pathname}`;
  }
  if (
    url.hostname !== 'meta-api.vip' ||
    !/^\/docs(?:\/|$)/.test(url.pathname)
  ) {
    return null;
  }
  return url.pathname.replace(/\/+$/, '');
}

function assertClosedFences(source, file) {
  let opened = null;
  for (const [index, line] of source.split('\n').entries()) {
    const match = line.match(/^ {0,3}(`{3,}|~{3,})(.*)$/);
    if (!match) continue;
    const [, fence, rest] = match;
    if (!opened) {
      opened = { char: fence[0], length: fence.length, line: index + 1 };
    } else if (
      fence[0] === opened.char &&
      fence.length >= opened.length &&
      !rest.trim()
    ) {
      opened = null;
    }
  }
  assert.equal(
    opened,
    null,
    `${file}: unclosed code fence at line ${opened?.line}`,
  );
}

function docsHostAPIUrls(code) {
  const modelRequest =
    /(?:Authorization|api[_-]?key|chat\.completions|\/v1(?:beta)?\b)/i.test(
      code,
    );
  const violations = [];
  for (const line of code.split('\n')) {
    if (/^\s*(?:#|\/\/|\*)/.test(line)) continue;
    for (const match of line.matchAll(/(?:https?|wss?):\/\/[^\s"'`<>),;]+/g)) {
      let url;
      try {
        url = new URL(match[0]);
      } catch {
        continue; // Other tools may legitimately show a templated host.
      }
      if (url.hostname !== 'docs.meta-api.vip') continue;
      const apiPath = /^\/(?:v1(?:beta)?|v2|v3|mj|suno)(?:\/|$)/.test(
        url.pathname,
      );
      const providerSetting =
        /(?:base[_-]?url|apiBase|api[_-]?base|endpoint|BASE_URL)["']?\s*[=:]/i.test(
          line,
        );
      const commandRequest =
        modelRequest && /\b(?:curl|fetch|requests\.(?:get|post))\b/.test(line);
      if (apiPath || providerSetting || commandRequest)
        violations.push(url.href);
    }
  }
  return violations;
}

test('handbook catalogs have unique metadata and safe, existing article files', () => {
  const keys = new Set(legacyRoutes.keys());
  const routes = new Set(legacyRoutes.values());
  const files = new Set();
  for (const row of catalogs()) {
    for (const field of [
      'key',
      'path',
      'group',
      'title',
      'description',
      'file',
    ]) {
      assert.ok(
        typeof row[field] === 'string' && row[field].trim(),
        `${row.catalog}: missing ${field}`,
      );
    }
    assert.match(
      row.path,
      /^\/docs\/[a-z0-9-]+(?:\/[a-z0-9-]+)*$/,
      `${row.file}: invalid public route`,
    );
    assert.ok(!keys.has(row.key), `${row.catalog}: duplicate key ${row.key}`);
    assert.ok(
      !routes.has(row.path),
      `${row.catalog}: duplicate route ${row.path}`,
    );
    assert.ok(
      !files.has(row.file),
      `${row.catalog}: duplicate article ${row.file}`,
    );
    keys.add(row.key);
    routes.add(row.path);
    files.add(row.file);
    readArticle(row);
  }
});

test('standalone article URLs do not occupy API, authentication or asset routes', () => {
  // The independent docs host removes the /docs prefix. Its API proxy and
  // authentication denylist must keep these namespaces; articles use other names.
  const reserved = new Set([
    'api',
    'v1',
    'v1beta',
    'console',
    'login',
    'register',
    'assets',
    'docs-images',
  ]);
  for (const route of allRoutes(catalogs())) {
    const standalonePath = route.slice('/docs'.length) || '/';
    const namespace = standalonePath.split('/')[1];
    assert.ok(
      !reserved.has(namespace),
      `${route}: standalone URL ${standalonePath} conflicts with a reserved namespace; move the article instead of widening the API proxy`,
    );
  }
});

test('handbook articles contain useful prose and closed examples rather than placeholders', () => {
  for (const row of catalogs()) {
    const source = readArticle(row);
    assertClosedFences(source, row.file);
    const tokens = marked.lexer(source);
    const titles = tokens.filter(
      (token) => token.type === 'heading' && token.depth === 1,
    );
    assert.equal(titles.length, 1, `${row.file}: expected one document title`);
    const prose = tokens
      .filter(
        (token) => !['code', 'heading', 'space', 'html'].includes(token.type),
      )
      .map((token) => token.raw)
      .join('\n')
      .replace(/https?:\/\/\S+/g, '');
    const meaningful = prose.match(/[\p{L}\p{N}]/gu) || [];
    assert.ok(
      meaningful.length >= 250,
      `${row.file}: insufficient explanatory content (${meaningful.length} characters)`,
    );
    const paragraphs = tokens.filter((token) => token.type === 'paragraph');
    assert.ok(
      paragraphs.length >= 2,
      `${row.file}: needs explanatory paragraphs beyond a link list`,
    );
    const structure = tokens.filter(
      (token) =>
        ['table', 'list', 'code'].includes(token.type) ||
        (token.type === 'heading' && token.depth > 1),
    );
    assert.ok(
      structure.length >= 2,
      `${row.file}: needs actionable sections, steps, fields or examples`,
    );
  }
});

test('all handbook documentation links resolve within the new and legacy route set', () => {
  const rows = catalogs();
  const routes = allRoutes(rows);
  const broken = [];
  for (const row of rows) {
    marked.walkTokens(marked.lexer(readArticle(row)), (token) => {
      if (token.type !== 'link') return;
      const target = normalizedDocPath(token.href);
      if (target && !routes.has(target))
        broken.push(`${row.file}: ${token.href} -> ${target}`);
    });
  }
  assert.deepEqual(
    broken,
    [],
    `Unresolved documentation links:\n${broken.join('\n')}`,
  );
});

test('model request examples never send traffic to the documentation host', () => {
  const wrong = [];
  for (const row of catalogs()) {
    marked.walkTokens(marked.lexer(readArticle(row)), (token) => {
      if (token.type !== 'code') return;
      for (const url of docsHostAPIUrls(token.text))
        wrong.push(`${row.file}: ${url}`);
    });
  }
  assert.deepEqual(
    wrong,
    [],
    `Documentation host used as API host:\n${wrong.join('\n')}`,
  );
});

test('API host validation distinguishes model connections from references and local tools', () => {
  assert.equal(
    docsHostAPIUrls('base_url="https://docs.meta-api.vip"').length,
    1,
  );
  assert.equal(
    docsHostAPIUrls('{"baseURL":"https://docs.meta-api.vip"}').length,
    1,
  );
  assert.equal(
    docsHostAPIUrls("curl 'https://docs.meta-api.vip/v1/chat/completions'")
      .length,
    1,
  );
  assert.equal(
    docsHostAPIUrls('const endpoint = "wss://docs.meta-api.vip/v1/realtime";')
      .length,
    1,
  );
  for (const example of [
    'base_url="https://meta-api.vip/v1"',
    'base_url="http://localhost:3000/v1"',
    'base_url="https://api.openai.com/v1"',
    '# Read https://docs.meta-api.vip/api/chat-completions\nbase_url="https://meta-api.vip/v1"',
    'const docs = "https://docs.meta-api.vip/api/overview";',
    'curl https://docs.meta-api.vip/llms.txt',
  ])
    assert.deepEqual(docsHostAPIUrls(example), [], example);
});

test('all 181 Chinese reference topics have unique sources and valid local destinations', () => {
  const coverage = readJSON('reference-coverage.json');
  assert.ok(
    Array.isArray(coverage.entries),
    'reference-coverage.json: entries must be an array',
  );
  assert.equal(
    coverage.entries.length,
    181,
    'reference-coverage.json: expected the audited 181 Chinese topics',
  );
  const routes = allRoutes(catalogs());
  const sources = new Set();
  for (const entry of coverage.entries) {
    const source = new URL(entry.sourceUrl);
    assert.equal(
      source.protocol,
      'https:',
      `${entry.sourceUrl}: source must use HTTPS`,
    );
    assert.equal(
      source.hostname,
      'docs.aihubmix.com',
      `${entry.sourceUrl}: unexpected reference host`,
    );
    assert.match(
      source.pathname,
      /^\/cn(?:\/|$)/,
      `${entry.sourceUrl}: expected a Chinese reference page`,
    );
    assert.ok(
      !source.search && !source.hash,
      `${entry.sourceUrl}: use the canonical source URL`,
    );
    const canonical = source.href.replace(/\/+$/, '');
    assert.ok(
      !sources.has(canonical),
      `reference-coverage.json: duplicate source ${canonical}`,
    );
    sources.add(canonical);
    for (const field of ['title', 'status', 'reason']) {
      assert.ok(
        typeof entry[field] === 'string' && entry[field].trim(),
        `${entry.sourceUrl}: missing ${field}`,
      );
    }
    assert.ok(
      Array.isArray(entry.targets) && entry.targets.length,
      `${entry.sourceUrl}: no local targets`,
    );
    assert.equal(
      new Set(entry.targets).size,
      entry.targets.length,
      `${entry.sourceUrl}: duplicate local targets`,
    );
    for (const target of entry.targets) {
      assert.ok(
        routes.has(target),
        `${entry.sourceUrl}: unresolved target ${target}`,
      );
    }
  }
});
