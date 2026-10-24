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

// 控制台支持的语言归一化；公开页面可提供更多语言，进入控制台时回退英文。
// 保持轻量，不引入 i18next 或翻译资源。
export const PUBLIC_LANGUAGE_CHANGE_EVENT = 'public-language-change';

export const SUPPORTED_LOCALES = [
  'zh-CN',
  'zh-TW',
  'en',
  'fr',
  'ru',
  'ja',
  'vi',
];

export const normalizeLocale = (lng) => {
  if (!lng) return 'zh-CN';
  if (lng.startsWith('zh-TW') || lng.startsWith('zh-HK')) return 'zh-TW';
  if (lng.startsWith('zh')) return 'zh-CN';
  if (SUPPORTED_LOCALES.includes(lng)) return lng;
  const shortLng = lng.split('-')[0];
  if (SUPPORTED_LOCALES.includes(shortLng)) return shortLng;
  // 未知的非中文语言统一回退英文（而非 zh-CN），与公开首页行为一致
  return 'en';
};
