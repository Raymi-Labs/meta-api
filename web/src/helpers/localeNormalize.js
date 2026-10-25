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

// Shared by the lightweight public shell, console and browser integrations.
export const SITE_LANGUAGES = [
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
export const SUPPORTED_LOCALES = SITE_LANGUAGES.map(([code]) => code);
export const SITE_LANGUAGE_KEY = 'site-language';
export const PUBLIC_LANGUAGE_CHANGE_EVENT = 'public-language-change';
export const normalizeLocale = (value) => {
  if (!value || typeof value !== 'string') return 'zh-CN';
  const tag = value.trim().replaceAll('_', '-').toLowerCase();
  if (/^zh(?:-|$)/.test(tag)) {
    return /(?:^|-)(hant|tw|hk|mo)(?:-|$)/.test(tag) ? 'zh-TW' : 'zh-CN';
  }
  const base = tag.split('-')[0];
  return SUPPORTED_LOCALES.includes(base) ? base : 'en';
};
