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
import { connectSiteLanguageI18n } from '../helpers/siteLanguageI18n';
import { siteLanguageStore, SITE_LANGUAGES } from '../helpers/siteLanguage';

const localeLoaders = {
  'zh-CN': () => import('./locales/zh-CN.json'),
  'zh-TW': () => import('./locales/zh-TW.json'),
  en: () => import('./locales/en.json'),
  ja: () => import('./locales/ja.json'),
  ko: () => import('./locales/ko.json'),
  de: () => import('./locales/de.json'),
  es: () => import('./locales/es.json'),
  fr: () => import('./locales/fr.json'),
  pt: () => import('./locales/pt.json'),
  ru: () => import('./locales/ru.json'),
  tr: () => import('./locales/tr.json'),
  pl: () => import('./locales/pl.json'),
  nl: () => import('./locales/nl.json'),
  it: () => import('./locales/it.json'),
  vi: () => import('./locales/vi.json'),
  uk: () => import('./locales/uk.json'),
  bg: () => import('./locales/bg.json'),
};

// Detection and persistence belong exclusively to the lightweight site store.
// In particular i18next must not keep writing its former i18nextLng cache.
const initPromise = i18n.use(initReactI18next).init({
  lng: siteLanguageStore.getSnapshot().language,
  supportedLngs: SITE_LANGUAGES.map(([language]) => language),
  load: 'currentOnly',
  resources: {},
  fallbackLng: 'zh-CN',
  returnEmptyString: false,
  nsSeparator: false,
  interpolation: { escapeValue: false },
});
export const initialLocaleReady = connectSiteLanguageI18n({
  i18n,
  store: siteLanguageStore,
  loaders: localeLoaders,
  initPromise,
}).ready;
export default i18n;
