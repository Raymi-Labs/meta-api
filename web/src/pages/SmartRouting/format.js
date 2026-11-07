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
