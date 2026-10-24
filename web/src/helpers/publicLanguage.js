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
import { normalizeLocale } from './localeNormalize.js';

export const PUBLIC_LANGUAGES = [
  ['en', 'English'],
  ['zh-CN', '简体中文'],
  ['zh-TW', '繁體中文'],
  ['ja', '日本語'],
  ['ko', '한국어'],
  ['de', 'Deutsch'],
  ['es', 'Español'],
  ['fr', 'Français'],
  ['pt', 'Português'],
  ['ru', 'Русский'],
  ['tr', 'Türkçe'],
  ['pl', 'Polski'],
  ['nl', 'Nederlands'],
  ['it', 'Italiano'],
  ['vi', 'Tiếng Việt'],
  ['uk', 'Українська'],
  ['bg', 'Български'],
];
export const PUBLIC_LANGUAGE_KEY = 'public-language';
export const CONSOLE_LANGUAGE_KEY = 'i18nextLng';
const languageCodes = new Set(PUBLIC_LANGUAGES.map(([code]) => code));
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

export const normalizePublicLanguage = (value) => {
  if (!value || typeof value !== 'string') return 'zh-CN';
  const tag = value.trim().replaceAll('_', '-').toLowerCase();
  if (/^zh(?:-|$)/.test(tag)) {
    return /(?:^|-)(hant|tw|hk|mo)(?:-|$)/.test(tag) ? 'zh-TW' : 'zh-CN';
  }
  const base = tag.split('-')[0];
  return languageCodes.has(base) ? base : 'en';
};

// Keep public preferences independent of the console's smaller locale catalog.
// A failed or superseded lazy import must never replace the visible language.
export const createPublicLanguageStore = ({
  storage = () => window.localStorage,
  browserLanguage = () =>
    typeof navigator === 'undefined' ? 'zh-CN' : navigator.language,
  load = (language) => loaders[language](),
} = {}) => {
  const listeners = new Set();
  const dictionaries = new Map([
    ['zh-CN', null],
    ['en', en],
  ]);
  const inflight = new Map();
  let request = 0;
  let transientLanguage = null;
  const read = (key) => {
    try {
      return storage()?.getItem(key);
    } catch {
      return null;
    }
  };
  const preferred = () =>
    normalizePublicLanguage(
      transientLanguage ||
        read(PUBLIC_LANGUAGE_KEY) ||
        read(CONSOLE_LANGUAGE_KEY) ||
        browserLanguage(),
    );
  const initial = preferred();
  let state = {
    language: dictionaries.has(initial) ? initial : 'en',
    dictionary: dictionaries.get(initial) || en,
    pending: null,
    error: false,
  };
  const publish = (next) => {
    state = { ...state, ...next };
    listeners.forEach((listener) => listener());
  };
  const change = async (value, persist = true) => {
    const language = normalizePublicLanguage(value);
    const id = ++request;
    publish({ pending: language, error: false });
    try {
      if (!dictionaries.has(language)) {
        if (!inflight.has(language)) {
          const promise = Promise.resolve()
            .then(() => load(language))
            .then((module) => {
              const dictionary = module.default || module;
              dictionaries.set(language, dictionary);
              return dictionary;
            })
            .finally(() => inflight.delete(language));
          inflight.set(language, promise);
        }
        await inflight.get(language);
      }
      if (id !== request) return false;
      if (persist) {
        transientLanguage = language;
        try {
          const target = storage();
          target?.setItem(PUBLIC_LANGUAGE_KEY, language);
          target?.setItem(CONSOLE_LANGUAGE_KEY, normalizeLocale(language));
          if (target) transientLanguage = null;
        } catch {
          /* Selection still works when browser storage is unavailable. */
        }
      }
      publish({
        language,
        dictionary: dictionaries.get(language),
        pending: null,
        error: false,
      });
      return true;
    } catch {
      if (id === request) publish({ pending: null, error: true });
      return false;
    }
  };
  return {
    getSnapshot: () => state,
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    change,
    restore: () => {
      const language = preferred();
      if (language !== state.language && language !== state.pending)
        return change(language, false);
    },
    handleStorage: (event) => {
      if (event.storageArea) {
        try {
          if (event.storageArea !== storage()) return;
        } catch {
          return;
        }
      }
      if (
        event.key === null ||
        event.key === PUBLIC_LANGUAGE_KEY ||
        (event.key === CONSOLE_LANGUAGE_KEY && !read(PUBLIC_LANGUAGE_KEY))
      ) {
        transientLanguage = null;
        return change(preferred(), false);
      }
    },
    translate: (key) =>
      state.language === 'zh-CN'
        ? key
        : state.dictionary?.[key] || en[key] || key,
  };
};
