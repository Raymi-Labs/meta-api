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

import { filterValues, catalogVendor } from './modelCatalogFacets.js';

export const matchesPricingVendor = (model, vendor) =>
  !filterValues(vendor).length ||
  filterValues(vendor).some(
    (v) => catalogVendor(v) === catalogVendor(model.vendor_name),
  );

export const groupForPricingVendor = ({
  models,
  usableGroup,
  group,
  vendor,
}) => {
  if (group === 'all') return group;
  return Object.hasOwn(usableGroup, group) &&
    models.some(
      (model) =>
        matchesPricingVendor(model, vendor) &&
        model.enable_groups?.includes(group),
    )
    ? group
    : 'all';
};

const priority = [
  ['anthropic', 'claude'],
  ['openai'],
  ['deepseek'],
  ['google', 'gemini'],
  ['moonshot', 'kimi'],
  ['xai', 'grok'],
];
export const sortPricingVendors = (vendors) => {
  const rank = (value) => {
    const index = priority.findIndex((names) =>
      names.includes(value.toLowerCase()),
    );
    return index < 0 ? priority.length : index;
  };
  return [...vendors].sort(
    (a, b) =>
      rank(a) - rank(b) || a.localeCompare(b, 'zh-CN', { numeric: true }),
  );
};
