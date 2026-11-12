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
