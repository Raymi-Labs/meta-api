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

import { getModelCatalogPricing } from './modelCatalogPricing';

export function getComparePricing(model, data, group, t) {
  if (
    !group ||
    !Object.hasOwn(data.usableGroup || {}, group) ||
    !model.enable_groups?.includes(group) ||
    !Number.isFinite(data.groupRatio?.[group])
  )
    return null;
  return getModelCatalogPricing(
    {
      record: model,
      selectedGroup: group,
      groupRatio: data.groupRatio,
      timeRatioMap: data.timeRatioMap,
      currency: 'USD',
      quotaDisplayType: 'USD',
      tokenUnit: 'M',
      displayPrice: (value) => `$${value}`,
    },
    t,
  );
}

const priceAliases = {
  'tiered-p': 'input',
  'tiered-c': 'completion',
  'tiered-cr': 'cache',
  'tiered-cc': 'create-cache',
  'tiered-cc1h': 'create-cache-1h',
  'tiered-img': 'image',
  'tiered-ai': 'audio-input',
  'tiered-ao': 'audio-output',
};
export function comparePriceItems(pricing) {
  return (pricing?.items || [])
    .filter((item) => item.key !== 'time-ratio')
    .map((item) => ({ ...item, key: priceAliases[item.key] || item.key }));
}
