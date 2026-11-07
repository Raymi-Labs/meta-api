import { test } from 'node:test';
import fs from 'node:fs';
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

test('reviewed public descriptions render every language without provider placeholders', () => {
  const entries = JSON.parse(
    fs.readFileSync(
      new URL(
        '../../model/data/public_model_descriptions.json',
        import.meta.url,
      ),
      'utf8',
    ),
  );
  assert.equal(entries.length, 5);
  for (const entry of entries) {
    const snapshot = {
      model_name: entry.model,
      description: entry.source,
      description_translations: entry.translations,
    };
    for (const [locale] of SITE_LANGUAGES) {
      const localized = localizeModelDescription(snapshot, locale);
      assert.equal(localized.description, entry.translations[locale]);
      assert(localized.description.length > 0);
      assert(
        !/venice|qihang-ai|vivgrid|openrouter\.ai|威尼斯|奏鸣曲|俳句/i.test(
          localized.description,
        ),
      );
      assert.equal(localized.model_name, entry.model);
    }
    assert.equal(snapshot.description, entry.source);
  }
});

test('FAQ follows all site locales with immutable source fallback', () => {
  const question_translations = Object.fromEntries(
    SITE_LANGUAGES.map(([locale]) => [locale, `${locale} question`]),
  );
  const model = {
    faq: [
      {
        question: 'Original question',
        answer: 'Original answer',
        question_translations,
        answer_translations: { en: 'English answer' },
      },
    ],
  };
  for (const [locale] of SITE_LANGUAGES) {
    const view = localizeModelDescription(model, locale);
    assert.equal(view.faq[0].question, `${locale} question`);
    assert.equal(
      view.faq[0].answer,
      locale === 'en' ? 'English answer' : 'Original answer',
    );
  }
  assert.equal(
    localizeModelDescription(localizeModelDescription(model, 'en'), 'ja').faq[0]
      .answer,
    'Original answer',
  );
  assert.equal(model.faq[0].question, 'Original question');
  assert.deepEqual(localizeModelDescription({ faq: [] }, 'en').faq, []);
});
