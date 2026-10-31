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

import { descriptionOverridePatch } from './modelDescription.js';
// Only edited display fields become manual overrides. Untouched initial values
// may have been refreshed by the background worker since the form opened.
const metadataFields = [
  'description',
  'tags',
  'vendor_id',
  'official_pricing',
  'release_date',
  'context_length',
  'max_output_tokens',
  'capabilities',
  'input_modalities',
  'output_modalities',
  'display_name',
  'faq',
];
export function omitUnchangedModelMetadata(payload, values, initial) {
  const patch = { ...payload };
  delete patch.description_translations;
  delete patch.description_automatic;
  const descriptions = descriptionOverridePatch(
    values?.description_overrides,
    initial?.description_overrides,
  );
  delete patch.description_overrides;
  if (Object.keys(descriptions).length)
    patch.description_overrides = descriptions;
  if (!initial) return patch;
  for (const field of metadataFields) {
    if (
      values.name_rule !== initial.name_rule &&
      ['release_date', 'official_pricing'].includes(field)
    )
      continue;
    if (JSON.stringify(values[field]) === JSON.stringify(initial[field])) {
      delete patch[field];
    }
  }
  const before = initial.capabilities || {};
  const after = values.capabilities || {};
  const overrides = {};
  for (const key of new Set([...Object.keys(before), ...Object.keys(after)])) {
    if (before[key] !== after[key]) overrides[key] = after[key] ?? null;
  }
  delete patch.capabilities;
  if (Object.keys(overrides).length) patch.capability_overrides = overrides;
  return patch;
}
