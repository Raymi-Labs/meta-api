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

const nonnegative = (value) =>
  value !== null &&
  value !== undefined &&
  value !== '' &&
  Number.isFinite(Number(value)) &&
  Number(value) >= 0;

// USD per million tokens / per request, before formatting and rounding.
// Dynamic expressions and hourly pricing have no comparable single unit here.
export const getModelSortPrices = (record, effectiveRatio) => {
  const prices = { input: null, output: null, request: null };
  if (!nonnegative(effectiveRatio) || record.billing_mode === 'tiered_expr')
    return prices;
  if (record.quota_type === 0 && nonnegative(record.model_ratio)) {
    prices.input = Number(record.model_ratio) * 2 * effectiveRatio;
    if (nonnegative(record.completion_ratio))
      prices.output = prices.input * Number(record.completion_ratio);
  } else if (record.quota_type === 1 && nonnegative(record.model_price)) {
    prices.request = Number(record.model_price) * effectiveRatio;
  }
  for (const key of Object.keys(prices))
    if (!Number.isFinite(prices[key])) prices[key] = null;
  return prices;
};

export const sortPricingModels = (models, order, getPrices) => {
  if (order === 'version-desc') {
    const collator = new Intl.Collator('en', {
      numeric: true,
      sensitivity: 'base',
    });
    const parts = (name = '') => {
      const match = name.match(/\d/);
      return {
        family: match
          ? name.slice(0, match.index).toLowerCase()
          : name.toLowerCase(),
        version: match ? name.slice(match.index) : '',
      };
    };
    return models
      .map((model, index) => ({ model, index, ...parts(model.model_name) }))
      .sort(
        (a, b) =>
          collator.compare(a.family, b.family) ||
          collator.compare(b.version, a.version) ||
          a.index - b.index,
      )
      .map(({ model }) => model);
  }
  if (!/^(input|output|request)-(asc|desc)$/.test(order)) return models;
  const [field, direction] = order.split('-');
  return models
    .map((model, index) => ({
      model,
      index,
      value: getPrices(model)?.[field] ?? null,
    }))
    .sort((a, b) => {
      if (a.value === null) return b.value === null ? a.index - b.index : 1;
      if (b.value === null) return -1;
      return (
        (direction === 'asc' ? a.value - b.value : b.value - a.value) ||
        a.index - b.index
      );
    })
    .map(({ model }) => model);
};

export const pricingSortOptions = (t) => [
  { value: 'version-desc', label: t('版本从新到旧') },
  { value: 'default', label: t('默认排序') },
  { value: 'input-asc', label: t('输入价格从低到高') },
  { value: 'input-desc', label: t('输入价格从高到低') },
  { value: 'output-asc', label: t('输出价格从低到高') },
  { value: 'output-desc', label: t('输出价格从高到低') },
  { value: 'request-asc', label: t('按次价格从低到高') },
  { value: 'request-desc', label: t('按次价格从高到低') },
];
