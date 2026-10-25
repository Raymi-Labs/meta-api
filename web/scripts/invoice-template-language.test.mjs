import test from 'node:test';
import assert from 'node:assert/strict';
import {
  localizeInvoiceTemplate,
  createInvoiceTemplateTranslator,
} from '../src/helpers/invoiceTemplateLanguage.js';

test('invoice translation sees only fixed text and preserves already-escaped account and order data', () => {
  const seen = [];
  const html = createInvoiceTemplateTranslator((source) => {
    seen.push(source);
    return source === '客户：{{PRINT_VALUE_0}}'
      ? 'Customer: {{PRINT_VALUE_0}}'
      : source;
  });
  const account = '张三 &lt;script&gt;';
  const result = html`<p>客户：${account}</p>
    <td>${'订单-123'}</td>`;
  assert.deepEqual(seen, ['客户：{{PRINT_VALUE_0}}']);
  assert.equal(
    result.replace(/>\s+</g, '><'),
    '<p>Customer: 张三 &lt;script&gt;</p><td>订单-123</td>',
  );
  assert.ok(
    !seen.some((key) => key.includes('张三') || key.includes('订单-123')),
  );
});

test('legal identity, company seal, currency, timezone and numeric values remain protected', () => {
  const seen = [];
  const source =
    '<p>本证明经上海曜算智能科技有限公司加盖公司公章后生效。</p><p>支付金额人民币 {{PRINT_VALUE_0}}，北京时间（UTC+08:00）。</p>';
  const result = localizeInvoiceTemplate([source], ['¥1,234.50'], (key) => {
    seen.push(key);
    return 'EN ' + key;
  });
  assert.ok(
    seen.every(
      (key) =>
        !key.includes('上海曜算智能科技有限公司') &&
        !key.includes('公司公章') &&
        !key.includes('人民币') &&
        !key.includes('UTC+08:00'),
    ),
  );
  assert.match(result, /上海曜算智能科技有限公司/);
  assert.match(result, /公司公章/);
  assert.match(result, /人民币/);
  assert.match(result, /UTC\+08:00/);
  assert.match(result, /¥1,234.50/);
});

test('malformed translations fall back to original and translated text cannot inject HTML', () => {
  assert.equal(
    localizeInvoiceTemplate(
      ['<p>客户：', '</p>'],
      ['Alice'],
      () => 'Missing placeholder',
    ),
    '<p>客户：Alice</p>',
  );
  assert.equal(
    localizeInvoiceTemplate(
      ['<p>客户：', '</p>'],
      ['Alice'],
      () => '<script>{{PRINT_VALUE_0}}</script>',
    ),
    '<p>&lt;script&gt;Alice&lt;/script&gt;</p>',
  );
});

test('styles and attributes are not translated; inserted content never becomes another template', () => {
  const seen = [];
  const result = localizeInvoiceTemplate(
    [
      '<style>/*中文样式*/</style><p title="公司公章">备注：',
      '</p><span>',
      '</span>',
    ],
    ['{{PRINT_VALUE_1}}', 'private'],
    (key) => {
      seen.push(key);
      return 'EN ' + key;
    },
  );
  assert.deepEqual(seen, ['备注：{{PRINT_VALUE_0}}']);
  assert.match(result, /<style>\/\*中文样式\*\/<\/style>/);
  assert.match(result, /title="公司公章"/);
  assert.match(result, /备注：\{\{PRINT_VALUE_1\}\}<\/p><span>private<\/span>/);
});

import { readFileSync } from 'node:fs';
import { collectInvoiceTemplateSources } from './invoice-language-sources.mjs';
import { SITE_LANGUAGES } from '../src/helpers/localeNormalize.js';

test('all languages cover the current invoice templates and preserve every protected placeholder', () => {
  const sources = collectInvoiceTemplateSources();
  assert.ok(Object.keys(sources).length >= 88);
  const tokens = (value) =>
    [...value.matchAll(/\{\{[^{}]+\}\}/g)].map(([token]) => token).sort();
  for (const [language] of SITE_LANGUAGES) {
    const dictionary = JSON.parse(
      readFileSync(
        new URL(`../src/i18n/locales/${language}.json`, import.meta.url),
        'utf8',
      ),
    ).translation;
    for (const source of Object.keys(sources)) {
      assert.ok(
        typeof dictionary[source] === 'string' && dictionary[source].trim(),
        `${language}: ${source}`,
      );
      assert.deepEqual(
        tokens(dictionary[source]),
        tokens(source),
        `${language}: ${source}`,
      );
    }
  }
});
