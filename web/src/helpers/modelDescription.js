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

import { normalizeLocale } from './localeNormalize.js';

// Project cached API data on every language change; never mutate its source.
export function localizeModelDescription(model, language) {
  if (!model) return model;
  const original = model.description_original ?? model.description ?? '';
  const translated =
    model.description_translations?.[normalizeLocale(language)];
  return {
    ...model,
    description_original: original,
    description:
      typeof translated === 'string' && translated.trim()
        ? translated
        : original,
  };
}

export function descriptionOverridePatch(values = {}, initial = {}) {
  const patch = {};
  for (const locale of new Set([
    ...Object.keys(values),
    ...Object.keys(initial),
  ])) {
    const next = values[locale]?.trim() || '';
    if (next !== (initial[locale]?.trim() || '')) patch[locale] = next || null;
  }
  return patch;
}
