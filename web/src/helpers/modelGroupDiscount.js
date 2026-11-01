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

// Official reference FX is independent of internal credit and recharge rates.
export function usableOfficialExchangeRate(quote, now = Date.now()) {
  const rate = quote?.usd_cny;
  const date =
    typeof quote?.date === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(quote.date)
      ? Date.parse(quote.date + 'T00:00:00Z')
      : NaN;
  return typeof rate === 'number' &&
    Number.isFinite(rate) &&
    rate > 0 &&
    Number.isFinite(date) &&
    new Date(date).toISOString().slice(0, 10) === quote.date &&
    date <= now &&
    now - date < 7 * 86400000
    ? rate
    : null;
}

const fieldByKey = {
  input: 'input',
  completion: 'output',
  cache: 'cache_read',
  'create-cache': 'cache_write',
  'tiered-p': 'input',
  'tiered-c': 'output',
  'tiered-cr': 'cache_read',
  'tiered-cc': 'cache_write',
  fixed: 'price',
  'audio-duration': 'price',
};
const validPrice = (value) =>
  typeof value === 'number' && Number.isFinite(value) && value >= 0;

export function officialComparisonUnit(model) {
  if (model.billing_mode === 'tiered_expr') return 'million_tokens';
  return { 0: 'million_tokens', 1: 'request', 2: 'hour' }[model.quota_type];
}

// Compare the displayed site cost against a reference converted at the actual
// USD/CNY rate. Recharge/package credit rates must never discount the reference.
export function groupPriceComparison({
  item,
  pricing,
  model,
  officialTier,
  currency,
  displayPrice,
  exchangeQuote,
  tokenUnit,
  quotaDisplayType,
}) {
  const unavailable = (reason) => ({ comparison: null, reason });
  const field = fieldByKey[item.key];
  if (!field) return unavailable('该项暂不支持官方对比');
  if (quotaDisplayType === 'TOKENS') return unavailable('倍率视图不比较金额');
  if (!officialTier) return unavailable('尚未匹配到对应的官方档位');
  if (officialTier.unit !== officialComparisonUnit(model))
    return unavailable('官方与本站的计价单位不同');
  // No structured TTL in the official schema: do not guess retention equivalence.
  if (
    field === 'cache_write' &&
    (model.supports_cache_creation_ttl ||
      (pricing.isDynamicPricing &&
        model.supported_endpoint_types?.includes('anthropic')))
  )
    return unavailable('缓存时长未对应，暂不比较');
  const reference = officialTier[field];
  const usd = pricing.unitPricesUSD?.[item.key];
  if (!validPrice(reference)) return unavailable('官方未提供该项单价');
  if (!validPrice(usd) || typeof displayPrice?.toAmount !== 'function')
    return unavailable('本站单价暂不可比较');
  const sourceCurrency = model.official_pricing?.currency;
  if (
    !['USD', 'CNY'].includes(currency) ||
    !['USD', 'CNY'].includes(sourceCurrency)
  )
    return unavailable('当前币种无法换算官方价格');
  let rate = 1;
  if (sourceCurrency !== currency) {
    const fx = usableOfficialExchangeRate(exchangeQuote);
    if (fx == null) return unavailable('暂无有效汇率，暂不显示官方对比');
    rate = sourceCurrency === 'USD' ? fx : 1 / fx;
  }
  const official =
    (reference * rate) /
    (officialTier.unit === 'million_tokens' && tokenUnit === 'K' ? 1000 : 1);
  const group = displayPrice.toAmount(usd);
  if (!validPrice(official) || !validPrice(group))
    return unavailable('本站单价暂不可比较');
  if (Math.abs(official - group) <= Math.max(official, group) * 1e-10)
    return { comparison: { official, group, kind: 'equal', percent: null } };
  if (official === 0)
    return { comparison: { official, group, kind: 'higher', percent: null } };
  const percent = Math.abs((group - official) / official) * 100;
  // Underflow/overflow must not become a spurious zero/free or infinite badge.
  if (!Number.isFinite(percent)) return unavailable('本站单价暂不可比较');
  const kind = group < official ? 'lower' : 'higher';
  const percentText =
    percent < 0.1
      ? '<0.1'
      : (kind === 'lower' && group > 0
          ? Math.min(99.9, percent)
          : percent
        ).toLocaleString('en-US', { maximumFractionDigits: 1 });
  return { comparison: { official, group, kind, percent: percentText } };
}

// Keep the existing pure comparison API for other callers.
export function compareGroupPrice(options) {
  return groupPriceComparison(options).comparison;
}
