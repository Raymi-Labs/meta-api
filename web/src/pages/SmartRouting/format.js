export const formatPercent = (value) =>
  value == null || Number.isNaN(value) ? '-' : `${(value * 100).toFixed(1)}%`;

// Histogram percentiles are bin upper bounds; -1 is the open overflow bin.
export const formatLatency = (value) => {
  if (value == null) return '-';
  if (value === -1) return '>120s';
  if (value >= 1000) return `≤${(value / 1000).toFixed(value % 1000 ? 1 : 0)}s`;
  return `≤${value}ms`;
};

export const statusColor = (status) =>
  ({ healthy: 'green', unhealthy: 'red', unknown: 'grey' })[status] || 'grey';

const parseJSON = (raw, fallback) => {
  if (!raw) return fallback;
  try {
    const value = JSON.parse(raw);
    return value ?? fallback;
  } catch {
    return fallback;
  }
};

// The API stores plan lists as JSON strings; the editor works with arrays.
export const planToForm = (plan) => ({
  id: plan?.id || 0,
  name: plan?.name || '',
  description: plan?.description || '',
  status: plan?.status || 1,
  groups: parseJSON(plan?.groups, []),
  sale_ratio: plan?.sale_ratio ?? 1,
  model_ratios: JSON.stringify(parseJSON(plan?.model_ratios, {}), null, 2),
  user_ids: parseJSON(plan?.user_ids, []).map(String),
  user_groups: parseJSON(plan?.user_groups, []),
});

export const formToPlan = (form) => {
  let modelRatios = {};
  const raw = (form.model_ratios || '').trim();
  if (raw) {
    modelRatios = JSON.parse(raw);
    if (
      typeof modelRatios !== 'object' ||
      Array.isArray(modelRatios) ||
      Object.values(modelRatios).some(
        (value) => typeof value !== 'number' || value < 0,
      )
    ) {
      throw new Error('model_ratios');
    }
  }
  const userIds = (form.user_ids || [])
    .map((value) => Number(String(value).trim()))
    .filter((value) => Number.isInteger(value) && value > 0);
  return {
    id: form.id || 0,
    name: form.name.trim(),
    description: form.description || '',
    status: form.status,
    groups: JSON.stringify(form.groups || []),
    sale_ratio: Number(form.sale_ratio) || 0,
    model_ratios: JSON.stringify(modelRatios),
    user_ids: JSON.stringify(userIds),
    user_groups: JSON.stringify(form.user_groups || []),
  };
};
