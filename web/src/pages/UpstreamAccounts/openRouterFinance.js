export const openRouterUSD = (amount) =>
  typeof amount === 'number' && Number.isFinite(amount)
    ? `$${amount.toFixed(6)}`
    : '--';

export function filterOpenRouterActivity(activity, search = '', date = '') {
  const query = search.trim().toLowerCase();
  return (activity || []).filter(
    (row) =>
      (!date || row.date === date) &&
      (!query ||
        [row.model, row.model_permaslug, row.provider_name].some((value) =>
          String(value || '')
            .toLowerCase()
            .includes(query),
        )),
  );
}

// Union both sources: local-only models must remain visible, and missing sides
// stay unknown instead of implying a measured zero.
export function summarizeOpenRouterConsumption(
  data,
  { start = '', end = '', search = '', daily = false } = {},
) {
  const groups = new Map();
  const query = search.trim().toLowerCase();
  const include = (r) =>
    (!start || r.date >= start) &&
    (!end || r.date <= end) &&
    (!query || r.model.toLowerCase().includes(query));
  const group = (r) => {
    const key = daily ? r.date : r.model;
    if (!groups.has(key))
      groups.set(key, {
        key,
        model: r.model,
        date: r.date,
        usage: null,
        quota: null,
        upstream_requests: null,
        site_requests: null,
        byok: null,
      });
    return groups.get(key);
  };
  for (const r of data?.activity || []) {
    if (!include(r)) continue;
    const g = group(r);
    g.usage = (g.usage ?? 0) + r.usage;
    g.upstream_requests = (g.upstream_requests ?? 0) + r.requests;
    g.byok = (g.byok ?? 0) + (r.byok_usage_inference || 0);
  }
  for (const r of data?.site_activity || []) {
    if (!include(r)) continue;
    const g = group(r);
    g.quota = (g.quota ?? 0) + r.quota;
    g.site_requests = (g.site_requests ?? 0) + r.requests;
  }
  return [...groups.values()].sort((a, b) =>
    daily
      ? b.date.localeCompare(a.date)
      : (b.usage ?? -1) - (a.usage ?? -1) || a.model.localeCompare(b.model),
  );
}

export function summarizeOpenRouterChannels(
  data,
  { start = '', end = '', search = '' } = {},
) {
  const groups = new Map();
  const query = search.trim().toLowerCase();
  for (const row of data?.channel_activity || []) {
    const name = `${row.channel_name} (#${row.channel_id})`;
    if (
      (start && row.date < start) ||
      (end && row.date > end) ||
      !name.toLowerCase().includes(query)
    )
      continue;
    if (!groups.has(row.channel_id))
      groups.set(row.channel_id, {
        key: row.channel_id,
        name,
        quota: 0,
        site_requests: 0,
      });
    const group = groups.get(row.channel_id);
    group.quota += row.quota;
    group.site_requests += row.requests;
  }
  return [...groups.values()].sort(
    (a, b) => b.quota - a.quota || a.key - b.key,
  );
}

// Fixed comparison rates requested for this report, independent of global FX.
const comparisonUSDToCNY = 6.7;
const lightningPerCNY = 5;
const cny = (amount) =>
  Number.isFinite(amount) ? `¥${amount.toFixed(6)}` : '--';
export const openRouterUsageCNY = (usd) =>
  typeof usd === 'number' ? cny(usd * comparisonUSDToCNY) : '--';

export function openRouterQuotaCNY(quota, quotaPerUnit, lightningPerUnit) {
  if (
    typeof quota !== 'number' ||
    !Number.isFinite(quotaPerUnit) ||
    quotaPerUnit <= 0 ||
    !Number.isFinite(lightningPerUnit) ||
    lightningPerUnit <= 0
  )
    return '--';
  return cny(((quota / quotaPerUnit) * lightningPerUnit) / lightningPerCNY);
}

export function readOpenRouterQuotaScale() {
  try {
    const status = JSON.parse(localStorage.getItem('status') || '{}');
    return {
      quotaPerUnit: Number(localStorage.getItem('quota_per_unit')),
      lightningPerUnit: Number(status?.custom_currency_exchange_rate ?? 1),
    };
  } catch {
    return { quotaPerUnit: NaN, lightningPerUnit: NaN };
  }
}
