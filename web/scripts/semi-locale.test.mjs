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

import assert from 'node:assert/strict';
import test from 'node:test';
import { format } from 'date-fns';
import en from '@douyinfe/semi-ui/lib/es/locale/source/en_GB';
import zhTW from '@douyinfe/semi-ui/lib/es/locale/source/zh_TW';
import ja from '@douyinfe/semi-ui/lib/es/locale/source/ja_JP';
import uk from '../src/i18n/semi/uk.js';
import bg from '../src/i18n/semi/bg.js';
import { SITE_LANGUAGES } from '../src/helpers/siteLanguage.js';
import { getSemiLocale, SEMI_LOCALES } from '../src/helpers/semiLocale.js';

const strings = (value, prefix = '') =>
  Object.entries(value).flatMap(([key, text]) => {
    if (key === 'dateFnsLocale') return [];
    const path = prefix ? `${prefix}.${key}` : key;
    return typeof text === 'object' ? strings(text, path) : [[path, text]];
  });
const placeholders = (value) =>
  [...value.matchAll(/\$\{([^}]+)\}/g)].map((match) => match[1]).sort();

test('every site language selects its own complete Semi locale', () => {
  const languages = SITE_LANGUAGES.map(([language]) => language).sort();
  assert.equal(languages.length, 17);
  assert.deepEqual(Object.keys(SEMI_LOCALES).sort(), languages);
  for (const language of languages) {
    assert.equal(getSemiLocale(language), SEMI_LOCALES[language], language);
    assert.ok(getSemiLocale(language).dateFnsLocale, language);
    assert.ok(getSemiLocale(language).Modal.confirm, language);
    assert.ok(getSemiLocale(language).DatePicker.placeholder.date, language);
  }
  assert.equal(
    new Set(languages.map((language) => getSemiLocale(language))).size,
    17,
  );
});

test('regional and legacy choices use the matching component locale', () => {
  assert.equal(getSemiLocale('zh-HK'), zhTW);
  assert.equal(getSemiLocale('ja-JP'), ja);
  assert.equal(getSemiLocale('en-US'), en);
  assert.equal(getSemiLocale('uk-UA'), uk);
  assert.equal(getSemiLocale('bg-BG'), bg);
});

for (const [language, locale, month] of [
  ['uk', uk, 'січень'],
  ['bg', bg, 'януари'],
]) {
  test(`${language} covers the installed Semi schema and preserves all interpolation variables`, () => {
    const expected = new Map(strings(en));
    const translated = new Map(strings(locale));
    assert.deepEqual(
      [...translated.keys()].sort(),
      [...expected.keys()].sort(),
    );
    for (const [path, value] of translated) {
      assert.equal(typeof value, 'string', path);
      assert.deepEqual(
        placeholders(value),
        placeholders(expected.get(path)),
        path,
      );
      if (expected.get(path)) assert.ok(value.trim(), path);
      if (
        path !== 'code' &&
        path !== 'DatePicker.monthText' &&
        path !== 'DatePicker.localeFormatToken.FORMAT_SWITCH_DATE' &&
        value
      ) {
        assert.match(value, /[А-Яа-яІіЇїЄєҐґ]/, path);
      }
    }
    assert.equal(locale.DatePicker.placeholder.dateRange.length, 2);
    assert.equal(locale.DatePicker.placeholder.dateTimeRange.length, 2);
    assert.equal(locale.DatePicker.placeholder.monthRange.length, 2);
  });

  test(`${language} formats dates with native month names and day-first dates`, () => {
    const date = new Date(2026, 0, 5, 12);
    assert.equal(format(date, 'LLLL', { locale: locale.dateFnsLocale }), month);
    assert.equal(
      format(date, locale.DatePicker.localeFormatToken.FORMAT_SWITCH_DATE, {
        locale: locale.dateFnsLocale,
      }),
      '05.01.2026',
    );
  });
}
