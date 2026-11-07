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

// Toggle one group in an ordered selection. New groups go to the end, so the
// first checked group stays preferred. "auto" is a platform-ordered group and
// cannot be combined with explicit groups; single mode keeps one group.
export const nextTokenGroupSelection = (selected, name, multiple = true) => {
  const current = Array.isArray(selected) ? selected : [];
  if (current.includes(name)) return current.filter((item) => item !== name);
  if (!multiple || name === 'auto') return [name];
  return [...current.filter((item) => item !== 'auto'), name];
};

export const moveTokenGroup = (selected, index, delta) => {
  const target = index + delta;
  if (index < 0 || index >= selected.length) return selected;
  if (target < 0 || target >= selected.length) return selected;
  const next = [...selected];
  [next[index], next[target]] = [next[target], next[index]];
  return next;
};

// Stable sort by the user's applicable ratio; unknown ratios go last so an
// unpriced group is never promoted to preferred.
export const sortSelectedGroupsByRatio = (selected, byName) => {
  const ratio = (name) => {
    const value = byName.get(name)?.ratio;
    return typeof value === 'number' && Number.isFinite(value) && value >= 0
      ? value
      : Number.POSITIVE_INFINITY;
  };
  return selected
    .map((name, index) => ({ name, index }))
    .sort((a, b) => ratio(a.name) - ratio(b.name) || a.index - b.index)
    .map(({ name }) => name);
};

// Vendor column: one entry per vendor with the groups that serve any of its
// models. The search matches a vendor name or a model name; a model match
// still yields its vendor, with the matching models listed as a hint.
// Models without a known vendor go to the "other" entry at the end.
export const buildVendorSections = (catalog, vendors, search, otherLabel) => {
  const query = String(search || '')
    .trim()
    .toLocaleLowerCase();
  const byVendor = new Map();
  for (const name of Object.keys(catalog || {}).sort()) {
    const vendor = vendors?.[name] || otherLabel;
    if (!byVendor.has(vendor))
      byVendor.set(vendor, { vendor, models: [], groups: new Set() });
    const entry = byVendor.get(vendor);
    entry.models.push(name);
    for (const group of catalog[name] || []) entry.groups.add(group);
  }
  const sections = [];
  for (const entry of byVendor.values()) {
    const vendorMatch =
      !query || entry.vendor.toLocaleLowerCase().includes(query);
    const matchedModels = query
      ? entry.models.filter((name) => name.toLocaleLowerCase().includes(query))
      : [];
    if (!vendorMatch && !matchedModels.length) continue;
    sections.push({
      ...entry,
      matchedModels: vendorMatch ? [] : matchedModels,
    });
  }
  return sections.sort((a, b) => {
    if (a.vendor === otherLabel) return 1;
    if (b.vendor === otherLabel) return -1;
    return a.vendor.localeCompare(b.vendor);
  });
};

// Read and write the form's group fields from one ordered selection: one group
// is a plain group token; two or more become a routing config whose first
// group is also the token group (the server requires that pairing).
export const selectionFromForm = (values, readTokenRouting) => {
  if (values?.routing_enabled)
    return readTokenRouting(values.routing_config).groups;
  return values?.group ? [values.group] : [];
};

export const formPatchForSelection = (next, values, readTokenRouting) => {
  if (next.length < 2) {
    return { group: next[0] || '', routing_enabled: false, routing_config: '' };
  }
  const config = readTokenRouting(values?.routing_config);
  const modelGroups = Object.fromEntries(
    Object.entries(config.model_groups || {})
      // An emptied rule stays empty so the editor asks for a new choice
      // instead of silently widening the model to every group.
      .map(([name, assigned]) => [
        name,
        assigned.filter((group) => next.includes(group)),
      ]),
  );
  return {
    group: next[0],
    routing_enabled: true,
    routing_config: JSON.stringify({
      groups: next,
      model_groups: modelGroups,
      ...(values?.auto_group_fallback ? { auto_fallback: true } : {}),
    }),
  };
};
