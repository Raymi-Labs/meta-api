import { readFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { extractMarkdownSources } from '../src/helpers/contentLanguage.js';
import { PUBLIC_POLICIES } from '../src/pages/PublicLegal/content.js';

// This catalog is shared by translation tooling and completeness checks. Runtime
// values (keys, models, account groups, URLs) are deliberately not translation data.
export function collectContentSources() {
  const root = new URL('../../', import.meta.url);
  const sources = {};
  const add = (key) => {
    if (/[\u3400-\u9fff]/u.test(key)) sources[key] = key;
  };
  const markdown = (text) => extractMarkdownSources(text).forEach(add);
  const docs = new URL('docs/', root);
  for (const name of readdirSync(docs).filter((name) =>
    /^NAN_.*\.md$/.test(name),
  )) {
    markdown(readFileSync(new URL(name, docs), 'utf8'));
  }
  const handbook = new URL('handbook/', docs);
  for (const name of ['guide', 'console', 'api', 'integrations']) {
    const catalog = JSON.parse(
      readFileSync(new URL(`${name}-catalog.json`, handbook), 'utf8'),
    );
    for (const entry of catalog) {
      [entry.title, entry.description, entry.group].forEach(add);
      markdown(readFileSync(new URL(entry.file, handbook), 'utf8'));
    }
  }
  // Legacy metadata is also prose: the directory and search must switch with bodies.
  const legacyCatalog = readFileSync(
    new URL('../src/pages/Docs/catalog.js', import.meta.url),
    'utf8',
  );
  for (const match of legacyCatalog.matchAll(
    /(?:title|description|group): '([^']+)'/g,
  )) {
    add(match[1]);
  }
  for (const policy of PUBLIC_POLICIES) {
    add(policy.intro);
    for (const section of policy.sections) {
      add(section.title);
      section.paragraphs.forEach(add);
    }
  }
  const originals = JSON.parse(
    readFileSync(
      new URL('../src/helpers/contentLanguageSources.json', import.meta.url),
      'utf8',
    ),
  );
  Object.values(originals).forEach(markdown);
  const runtime = readFileSync(
    new URL('../src/pages/Docs/runtimeDocs.js', import.meta.url),
    'utf8',
  );
  for (const match of runtime.matchAll(/\bt\(\s*'([^']+)'/g))
    markdown(match[1]);
  ['图像模型', '语音模型', '其他模型'].forEach(markdown);
  return sources;
}

// Usage: node web/scripts/content-language-sources.mjs > /tmp/content-source.json
if (process.argv[1] === fileURLToPath(import.meta.url)) {
  process.stdout.write(JSON.stringify(collectContentSources(), null, 2) + '\n');
}
