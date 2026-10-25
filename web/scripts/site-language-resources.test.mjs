import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import test from 'node:test';
import { SITE_LANGUAGES } from '../src/helpers/localeNormalize.js';

const require = createRequire(import.meta.url);
const { parse } = require('@babel/parser');
const traverse = require('@babel/traverse').default;
const root = fileURLToPath(new URL('../src/', import.meta.url));
const readLocale = (language) =>
  JSON.parse(
    fs.readFileSync(
      path.join(root, 'i18n/locales', `${language}.json`),
      'utf8',
    ),
  ).translation;
const source = readLocale('zh-CN');
const placeholders = (text) =>
  [...text.matchAll(/\{\{[^{}]+\}\}|\$\{[^{}]+\}/g)]
    .map(([token]) => token)
    .sort();

function sourceKeys(directory, keys = new Set()) {
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const file = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      if (!['i18n', 'publicLocales', 'contentLocales'].includes(entry.name))
        sourceKeys(file, keys);
    } else if (/\.[jt]sx?$/.test(entry.name)) {
      const ast = parse(fs.readFileSync(file, 'utf8'), {
        sourceType: 'unambiguous',
        plugins: ['jsx', 'typescript'],
      });
      traverse(ast, {
        CallExpression({ node }) {
          if (
            !(
              (node.callee.type === 'Identifier' && node.callee.name === 't') ||
              (node.callee.type === 'MemberExpression' &&
                node.callee.property.name === 't')
            )
          )
            return;
          const argument = node.arguments[0];
          if (argument?.type === 'StringLiteral') keys.add(argument.value);
          else if (
            argument?.type === 'TemplateLiteral' &&
            !argument.expressions.length
          )
            keys.add(argument.quasis[0].value.cooked);
        },
      });
    }
  }
  return keys;
}

test('all statically referenced interface strings have a canonical translation', () => {
  const missing = [...sourceKeys(root)].filter(
    (key) => typeof source[key] !== 'string' || !source[key].trim(),
  );
  assert.deepEqual(
    missing,
    [],
    'Add new interface text to every site language',
  );
});

for (const [language] of SITE_LANGUAGES) {
  test(`${language} covers the canonical interface without empty text or lost placeholders`, () => {
    const dictionary = readLocale(language);
    const missing = Object.keys(source).filter(
      (key) => typeof dictionary[key] !== 'string' || !dictionary[key].trim(),
    );
    assert.deepEqual(missing, [], `${language}: missing interface text`);
    const invalid = Object.keys(source).filter(
      (key) =>
        JSON.stringify(placeholders(source[key])) !==
        JSON.stringify(placeholders(dictionary[key])),
    );
    assert.deepEqual(
      invalid,
      [],
      `${language}: interpolation placeholders must stay intact`,
    );
    assert.deepEqual(
      Object.keys(source).filter((key) =>
        /ZX[QVE]\s*\d{5}/i.test(dictionary[key]),
      ),
      [],
      `${language}: temporary translation markers must not be shipped`,
    );
    for (const [key, original] of Object.entries(source)) {
      for (const term of ['Meta Pulse', 'Pulse', 'RPM']) {
        if (original.includes(term)) {
          assert.ok(
            dictionary[key].includes(term),
            `${language}: preserve product name or API metric ${term} in ${key}`,
          );
        }
      }
    }
  });
}
