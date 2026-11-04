export const normalizeVMCapacitySettings = (value) => ({
  enabled: value?.enabled === true,
  overflow_policy: value?.overflow_policy === 'reroute' ? 'reroute' : 'wait',
  max_wait_ms:
    Number.isInteger(value?.max_wait_ms) && value.max_wait_ms > 0
      ? value.max_wait_ms
      : 30000,
});

export const readChannelCapacityUsage = (item, defaultMaxConcurrency) => ({
  inflight: Math.max(0, Number(item.inflight) || 0),
  maxConcurrency: item.vm_capacity
    ? Math.max(0, Number(item.max_concurrency) || 0)
    : Number(item.max_concurrency) > 0
      ? Number(item.max_concurrency)
      : defaultMaxConcurrency,
  vmCapacity: item.vm_capacity || null,
  staticMaxConcurrency: item.static_max_concurrency,
});
