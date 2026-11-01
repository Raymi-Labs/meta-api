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

// Official reference prices are independent of site billing and display rates.
export const OFFICIAL_PRICE_COLUMNS = [
  ['input', '输入'],
  ['output', '输出'],
  ['cache_read', '缓存读取'],
  ['cache_write', '缓存创建'],
  ['price', '价格'],
];
export const OFFICIAL_PRICE_UNITS = {
  million_tokens: 'M tokens',
  request: 'request',
  image: 'image',
  second: 's',
  minute: 'min',
  hour: 'h',
};
export function officialPriceColumns(tiers = []) {
  return OFFICIAL_PRICE_COLUMNS.filter(([key]) =>
    tiers.some(
      (tier) =>
        typeof tier[key] === 'number' &&
        Number.isFinite(tier[key]) &&
        tier[key] >= 0,
    ),
  );
}
export function formatOfficialPrice(
  value,
  currency,
  maximumSignificantDigits = 15,
) {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0)
    return '—';
  const symbol = { USD: '$', CNY: '¥', EUR: '€' }[currency];
  return `${symbol || currency + ' '}${value.toLocaleString('en-US', { maximumSignificantDigits })}`;
}
export function officialPricingSource(value) {
  try {
    const url = new URL(value);
    return ['https:', 'http:'].includes(url.protocol) &&
      !url.username &&
      !url.password
      ? url.href
      : null;
  } catch {
    return null;
  }
}

// Retain raw source metadata; the catalog offers the default 30m cache rate.
export function visibleOfficialTiers(pricing) {
  return (Array.isArray(pricing?.tiers) ? pricing.tiers : [])
    .filter((tier) => !/ · cache 12h$/i.test(tier.label || ''))
    .map((tier) => ({
      ...tier,
      label: (tier.label || '').replace(/ · cache 30m$/i, ''),
    }));
}

export function visibleOfficialNotes(notes = '') {
  return notes
    .replace(
      /Cache-write prices shown use 30-minute retention\. Standard cache writes cost \$[\d.]+ per million tokens for 30-minute retention and \$[\d.]+ for 12-hour retention, before long-context adjustments\./g,
      '',
    )
    .trim();
}
