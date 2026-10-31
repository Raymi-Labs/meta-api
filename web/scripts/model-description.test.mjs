import { test } from 'node:test';
import assert from 'node:assert/strict';
import { SITE_LANGUAGES } from '../src/helpers/localeNormalize.js';
import { localizeModelDescription } from '../src/helpers/modelDescription.js';
import { omitUnchangedModelMetadata } from '../src/helpers/modelMetadataPatch.js';

test('all 17 site languages switch from the same immutable API snapshot', () => {
  const model = {
    model_name: 'gpt-6-astra',
    description: 'Original English',
    description_translations: Object.fromEntries(
      SITE_LANGUAGES.map(([locale]) => [locale, `${locale} translation`]),
    ),
  };
  for (const [locale] of SITE_LANGUAGES) {
    const view = localizeModelDescription(model, locale);
    assert.equal(view.description, `${locale} translation`);
    assert.equal(view.description_original, 'Original English');
    assert.equal(view.model_name, model.model_name);
  }
  assert.equal(model.description, 'Original English');
  assert.equal(
    localizeModelDescription(model, 'zh-Hant-HK').description,
    'zh-TW translation',
  );
  assert.equal(
    localizeModelDescription(model, 'ja-JP').description,
    'ja translation',
  );
});
test('missing or empty translations retain source, not the previously selected translation', () => {
  const m = {
    description: 'Source',
    description_translations: { en: 'English', ja: ' ' },
  };
  const en = localizeModelDescription(m, 'en');
  assert.equal(localizeModelDescription(en, 'ja').description, 'Source');
  assert.equal(localizeModelDescription(en, 'fr').description, 'Source');
});
test('only edited language overrides are sent; clearing restores auto and read-only translations never save', () => {
  const initial = { description_overrides: { en: 'English', ja: '日本語' } };
  const values = {
    description_overrides: { ja: '新しい紹介', de: 'Deutsch' },
    description_translations: { 'zh-CN': '自动中文' },
    description_automatic: { en: 'auto' },
  };
  assert.deepEqual(omitUnchangedModelMetadata(values, values, initial), {
    description_overrides: { en: null, ja: '新しい紹介', de: 'Deutsch' },
  });
  assert.deepEqual(omitUnchangedModelMetadata(initial, initial, initial), {});
});
