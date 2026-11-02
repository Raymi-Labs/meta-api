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
