// Display the same partition as billing. The total includes the TTL splits.
// Legacy logs may omit the total; only then may it be derived from the splits.
export const getCacheCreationBreakdown = (record = {}) => {
  const count = (value) => {
    const n = Number(value);
    return Number.isFinite(n) && n >= 0 ? n : 0;
  };
  const fiveMinute = count(record.cache_creation_tokens_5m);
  const oneHour = count(record.cache_creation_tokens_1h);
  const split = fiveMinute + oneHour;
  const reported = record.cache_creation_tokens;
  const hasTotal = reported !== undefined && reported !== null;
  const total = hasTotal ? count(reported) : split;
  return {
    total,
    fiveMinute,
    oneHour,
    unclassified: Math.max(0, total - split),
    conflict: hasTotal && split > total,
  };
};

// This describes the stored prompt counter, not the client's request protocol.
// Old Messages -> OpenAI logs can carry stale Claude flags. Only a matching
// settlement snapshot proves that their prompt counter already includes cache;
// other legacy providers may have normalized it before storing the log.
export const promptTokensExcludeCache = (record = {}) => {
  if (record?.prompt_tokens_semantic === 'inclusive') return false;
  if (record?.prompt_tokens_semantic === 'non_cache') return true;

  const legacyNonCache =
    record?.claude === true ||
    (typeof record?.usage_semantic === 'string' &&
      record.usage_semantic.toLowerCase() === 'anthropic');
  const chain = record?.request_conversion;
  const finalFormat = Array.isArray(chain) ? chain[chain.length - 1] : '';
  const snapshotLength = record?.tiered_params?.len;
  const prompt = record?.prompt_tokens;
  const cacheRead = Number(record?.cache_tokens);
  const hasCache =
    (Number.isFinite(cacheRead) && cacheRead > 0) ||
    getCacheCreationBreakdown(record || {}).total > 0;
  if (
    legacyNonCache &&
    hasCache &&
    record?.billing_mode === 'tiered_expr' &&
    Array.isArray(chain) &&
    chain.length > 1 &&
    chain[0] === 'Claude Messages' &&
    ['OpenAI Compatible', 'OpenAI Responses'].includes(finalFormat) &&
    typeof snapshotLength === 'number' &&
    Number.isFinite(snapshotLength) &&
    snapshotLength >= 0 &&
    typeof prompt === 'number' &&
    Number.isFinite(prompt) &&
    prompt >= 0 &&
    snapshotLength === prompt
  ) {
    return false;
  }
  return legacyNonCache;
};

export const getNonCachePromptTokens = (record = {}) => {
  const prompt = Number(record?.prompt_tokens);
  const safePrompt = Number.isFinite(prompt) && prompt > 0 ? prompt : 0;
  if (promptTokensExcludeCache(record)) return safePrompt;
  const cacheRead = Number(record?.cache_tokens);
  return Math.max(
    0,
    safePrompt -
      (Number.isFinite(cacheRead) && cacheRead > 0 ? cacheRead : 0) -
      getCacheCreationBreakdown(record || {}).total,
  );
};
