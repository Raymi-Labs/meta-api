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

export const MAX_COMPARE_MODELS = 4;

export function getCompareModels(search) {
  return [...new Set(new URLSearchParams(search).getAll('model'))]
    .filter((name) => name.length > 0 && name.length <= 255)
    .slice(0, MAX_COMPARE_MODELS);
}

export function buildModelComparePath(names = []) {
  const query = new URLSearchParams();
  [...new Set(names)]
    .filter(
      (name) =>
        typeof name === 'string' && name.length > 0 && name.length <= 255,
    )
    .slice(0, MAX_COMPARE_MODELS)
    .forEach((name) => query.append('model', name));
  return `/compare${query.size ? `?${query}` : ''}`;
}

export function buildModelPlaygroundPath(model, group) {
  const query = new URLSearchParams({ model });
  if (group && group !== 'all' && group !== 'auto') query.set('group', group);
  return `/playground?${query}`;
}

export function getCompareGroups(models, usableGroup) {
  const groups = Object.keys(usableGroup || {}).filter(
    (group) =>
      group !== 'auto' &&
      models.some((model) => model.enable_groups?.includes(group)),
  );
  return groups.sort(
    (a, b) =>
      Number(models.every((model) => model.enable_groups?.includes(b))) -
      Number(models.every((model) => model.enable_groups?.includes(a))),
  );
}

// Monthly token estimates only apply to ordinary text token pricing.
export function estimateModelMonthlyCost(model, price, workload) {
  if (!price || model.quota_type !== 0 || price.isDynamicPricing) return null;
  const input = price.unitPricesUSD?.input;
  const output = price.unitPricesUSD?.completion;
  const values = [
    input,
    output,
    workload.requests,
    workload.input,
    workload.output,
  ];
  if (
    !values.every(
      (value) =>
        typeof value === 'number' && Number.isFinite(value) && value >= 0,
    )
  )
    return null;
  const total =
    (workload.requests *
      30 *
      (workload.input * input + workload.output * output)) /
    1e6;
  return Number.isFinite(total) ? total : null;
}
