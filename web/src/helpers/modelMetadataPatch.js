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
