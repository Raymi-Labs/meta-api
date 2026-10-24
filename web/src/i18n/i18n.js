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

import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';
import {
  normalizeLocale,
  PUBLIC_LANGUAGE_CHANGE_EVENT,
} from '../helpers/localeNormalize';

const localeLoaders = {
  'zh-CN': () => import('./locales/zh-CN.json'),
  en: () => import('./locales/en.json'),
  fr: () => import('./locales/fr.json'),
  'zh-TW': () => import('./locales/zh-TW.json'),
  ru: () => import('./locales/ru.json'),
  ja: () => import('./locales/ja.json'),
  vi: () => import('./locales/vi.json'),
};

const loadLocale = async (lng) => {
  const normalizedLng = normalizeLocale(lng);
  if (i18n.hasResourceBundle(normalizedLng, 'translation')) {
    return normalizedLng;
  }
  const loader = localeLoaders[normalizedLng];
  if (!loader) {
    return 'zh-CN';
  }
  const module = await loader();
  const resources = module.default || module;
  i18n.addResourceBundle(
    normalizedLng,
    'translation',
    resources.translation || resources,
    true,
    true,
  );
  return normalizedLng;
};

const loadLocaleWhenIdle = (lng) => {
  const load = () => {
    if (normalizeLocale(i18n.language) !== lng) return;
    // Background resource loading must never override a user's language choice.
    loadLocale(lng).catch((error) => {
      console.error('Failed to load locale:', error);
    });
  };

  if (typeof window !== 'undefined' && 'requestIdleCallback' in window) {
    window.requestIdleCallback(load, { timeout: 4000 });
  } else {
    setTimeout(load, 2000);
  }
};

const initPromise = i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    load: 'currentOnly',
    resources: {},
    fallbackLng: 'zh-CN',
    nsSeparator: false,
    interpolation: {
      escapeValue: false,
    },
  });

const changeLanguage = i18n.changeLanguage.bind(i18n);
let languageChangeRequest = 0;
i18n.changeLanguage = async (lng, callback) => {
  const request = ++languageChangeRequest;
  const normalizedLng = await loadLocale(lng);
  if (request !== languageChangeRequest) return i18n.t.bind(i18n);
  return changeLanguage(normalizedLng, callback);
};

// The console may already be loaded when a visitor switches the public menu.
if (typeof window !== 'undefined') {
  window.addEventListener(PUBLIC_LANGUAGE_CHANGE_EVENT, (event) => {
    i18n.changeLanguage(event.detail).catch((error) => {
      console.error('Failed to load locale:', error);
    });
  });
}

// 初始语言就绪信号：AppShell 在此 promise 完成后才渲染（见 src/index.jsx）。
// zh-CN 是翻译 key 的源语言，t() 直接返回 key 即为正确文案，
// 语言包留到空闲时补齐，不阻塞首屏；
// 其他语言必须等语言包下载完成，否则会先闪中文 key 再切换目标语言。
export const initialLocaleReady = initPromise.then(() => {
  const normalizedLng = normalizeLocale(i18n.language);
  if (normalizedLng === 'zh-CN') {
    loadLocaleWhenIdle(normalizedLng);
    return;
  }
  return i18n.changeLanguage(normalizedLng).catch((error) => {
    console.error('Failed to load locale:', error);
  });
});

export default i18n;
