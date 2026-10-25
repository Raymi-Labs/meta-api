import assert from 'node:assert/strict';
import { test, expect } from 'bun:test';
import { DOCS } from '../src/pages/Docs/catalog.js';
import {
  getActiveDoc,
  docPath,
  searchDocs,
  standaloneMarkdown,
  extractHeadings,
} from '../src/pages/Docs/docsNavigation.js';
import { createPublicDocsAPI } from '../src/pages/Docs/publicDocsAPI.js';

test('all thirteen guides resolve legacy and standalone deep links', () => {
  expect(DOCS.length).toBe(13);
  for (const doc of DOCS) {
    expect(getActiveDoc(doc.path)).toBe(doc);
    expect(getActiveDoc(`${docPath(doc.path, true)}/`, true)).toBe(doc);
    expect(getActiveDoc(doc.path, true)).toBe(doc);
  }
  expect(getActiveDoc('/clients/unknown', true)).toBeNull();
  expect(getActiveDoc('/docs/not-found')).toBeNull();
  expect(getActiveDoc('/console', true)).toBeNull();
});

test('search finds translated body terms, requires all terms, ranks titles without regex', () => {
  const docs = [
    { title: 'Codex', description: '', content: 'API setup' },
    { title: 'API setup', description: '', content: 'Codex' },
  ];
  expect(searchDocs('  api SETUP ', docs).map((d) => d.title)).toEqual([
    'API setup',
    'Codex',
  ]);
  expect(searchDocs('missing api', docs)).toEqual([]);
  expect(searchDocs('[', docs)).toEqual([]);
  expect(searchDocs(' ', docs)).toEqual([]);
});

test('link mapping keeps docs local and moves account links home without changing code', () => {
  const code =
    '```python\nurl = "https://meta-api.vip/v1"\n# [example](/docs)\n```';
  const source = `[Docs](/docs/clients/codex)\n[Examples](https://meta-api.vip/docs/api-examples)\n[Account](/console/token)\n[Download](/downloads/skill.zip)\n![Image](/docs-images/a.png)\n${code}`;
  const output = standaloneMarkdown(source, 'ja');
  expect(output).toContain('[Docs](/clients/codex)');
  expect(output).toContain('[Examples](/api-examples)');
  expect(output).toContain(
    '[Account](https://meta-api.vip/console/token?lang=ja)',
  );
  expect(output).toContain(
    '[Download](https://meta-api.vip/downloads/skill.zip)',
  );
  expect(output).toContain('![Image](/docs-images/a.png)');
  expect(output).toContain(code);
});

test('TOC excludes headings within executable examples', () => {
  expect(extractHeadings('## Intro\n```sh\n## hidden\n```\n### Next')).toEqual([
    { id: 'intro', title: 'Intro', level: 2 },
    { id: 'next', title: 'Next', level: 3 },
  ]);
});

test('public client omits credentials and rejects other endpoints', async () => {
  const calls = [];
  const client = createPublicDocsAPI(async (path, options) => {
    calls.push({ path, options });
    return { ok: true, json: async () => ({ success: true, data: [] }) };
  });
  await client.get('/api/status');
  await client.get('/api/pricing');
  expect(calls.map((c) => c.path)).toEqual(['/api/status', '/api/pricing']);
  for (const { options } of calls) {
    expect(options.credentials).toBe('omit');
    expect(options.headers).toBeUndefined();
  }
  await expect(client.get('/api/user/self')).rejects.toThrow('Unsupported');
  expect(calls.length).toBe(2);
});

test('public requests reject HTTP failures and time out', async () => {
  await expect(
    createPublicDocsAPI(async () => ({ ok: false })).get('/api/status'),
  ).rejects.toThrow('unavailable');
  const client = createPublicDocsAPI(
    (_, { signal }) =>
      new Promise((resolve, reject) =>
        signal.addEventListener('abort', () => reject(new Error('aborted'))),
      ),
  );
  await expect(client.get('/api/status', { timeout: 5 })).rejects.toThrow(
    'aborted',
  );
});

test('only the official documentation host receives a language preference', async () => {
  const { withDocsLanguage } = await import('../src/helpers/docsLink.js');
  assert.equal(
    withDocsLanguage(
      'https://docs.meta-api.vip/api-examples?from=home#sample',
      'ja',
    ),
    'https://docs.meta-api.vip/api-examples?from=home&lang=ja#sample',
  );
  for (const url of [
    '/docs',
    '',
    'https://example.test/docs',
    'https://docs.meta-api.vip.evil.test/',
    'http://docs.meta-api.vip',
  ]) {
    assert.equal(withDocsLanguage(url, 'ja'), url);
  }
});
