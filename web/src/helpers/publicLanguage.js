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

import en from './publicLocales/en.json';
import { normalizeLocale, SITE_LANGUAGES } from './localeNormalize.js';
import { createSiteLanguageStore } from './siteLanguage.js';
export const PUBLIC_LANGUAGES = SITE_LANGUAGES;
export const PUBLIC_LANGUAGE_KEY = 'public-language';
export const CONSOLE_LANGUAGE_KEY = 'i18nextLng';
export const normalizePublicLanguage = normalizeLocale;
const loaders = {
  'zh-TW': () => import('./publicLocales/zh-TW.json'),
  ja: () => import('./publicLocales/ja.json'),
  ko: () => import('./publicLocales/ko.json'),
  de: () => import('./publicLocales/de.json'),
  es: () => import('./publicLocales/es.json'),
  fr: () => import('./publicLocales/fr.json'),
  pt: () => import('./publicLocales/pt.json'),
  ru: () => import('./publicLocales/ru.json'),
  tr: () => import('./publicLocales/tr.json'),
  pl: () => import('./publicLocales/pl.json'),
  nl: () => import('./publicLocales/nl.json'),
  it: () => import('./publicLocales/it.json'),
  vi: () => import('./publicLocales/vi.json'),
  uk: () => import('./publicLocales/uk.json'),
  bg: () => import('./publicLocales/bg.json'),
};
export const createPublicLanguageStore = ({
  storage,
  browserLanguage,
  load = (language) => loaders[language](),
  siteStore,
} = {}) => {
  const site =
    siteStore || createSiteLanguageStore({ storage, browserLanguage });
  const dictionaries = new Map([
    ['zh-CN', null],
    ['en', en],
  ]);
  const inflight = new Map();
  const listeners = new Set();
  const prepare = async (language) => {
    if (dictionaries.has(language)) return;
    if (!inflight.has(language)) {
      inflight.set(
        language,
        Promise.resolve()
          .then(() => load(language))
          .then((module) => {
            dictionaries.set(language, module.default || module);
          })
          .finally(() => inflight.delete(language)),
      );
    }
    await inflight.get(language);
  };
  site.registerLoader(prepare);
  const initial = site.getSnapshot().language;
  let state = {
    language: dictionaries.has(initial) ? initial : 'en',
    dictionary: dictionaries.get(initial) || en,
    pending: null,
    error: false,
  };
  site.subscribe(() => {
    const next = site.getSnapshot();
    const language = dictionaries.has(next.language)
      ? next.language
      : state.language;
    state = {
      language,
      dictionary: dictionaries.get(language),
      pending: next.pending,
      error: next.error,
    };
    for (const listener of [...listeners]) listener();
  });
  return {
    getSnapshot: () => state,
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    change: (language, persist = true) =>
      site.change(language, { persist, source: persist ? 'user' : 'restore' }),
    restore: site.restore,
    handleStorage: site.handleStorage,
    translate: (key) =>
      state.language === 'zh-CN'
        ? key
        : state.dictionary?.[key] || en[key] || key,
  };
};
