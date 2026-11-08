import { API } from '../../../helpers/apiCore';

// Model -> groups catalog (with each model's vendor) for the token group
// picker. The tokens page prefetches it so the create/edit dialog opens with
// vendors ready; the dialog shows a cached copy at once and revalidates it.
// It only drives filtering hints: the server rechecks groups on save.
export const TOKEN_GROUP_CATALOG_FRESH_MS = 5 * 60 * 1000;
const MAX_GROUPS = 100;

let entry = null; // { key, at, value, promise }

export function catalogGroupNames(groups) {
  return (Array.isArray(groups) ? groups : [])
    .map((group) => (typeof group === 'string' ? group : group?.value))
    .filter((name) => name && name !== 'auto')
    .slice(0, MAX_GROUPS);
}

const cacheKey = (userId, names) => `${userId}\u0000${names.join('\u0000')}`;

export function parseTokenGroupCatalog(items) {
  const catalog = {};
  const vendors = {};
  for (const item of Array.isArray(items) ? items : []) {
    if (!item?.name || !Array.isArray(item.groups)) continue;
    catalog[item.name] = item.groups;
    if (item.vendor) vendors[item.name] = item.vendor;
  }
  return { catalog, vendors };
}

async function fetchTokenGroupCatalog(names) {
  const res = await API.get('/api/token/models', {
    disableDuplicate: true,
    params: {
      detail: true,
      // The server requires the draft's group to be the first listed one.
      group: names[0],
      routing_config: JSON.stringify({ groups: names }),
    },
  });
  if (!res.data?.success) throw new Error(res.data?.message);
  return parseTokenGroupCatalog(res.data.data);
}

// Returns the cached catalog for exactly this user and group list, with
// `fresh` telling whether it is recent enough to skip a refresh.
export function peekTokenGroupCatalog(userId, names, now = Date.now()) {
  if (!userId || !entry?.value || entry.key !== cacheKey(userId, names))
    return null;
  return {
    ...entry.value,
    fresh: now - entry.at < TOKEN_GROUP_CATALOG_FRESH_MS,
  };
}

export function loadTokenGroupCatalog(
  userId,
  names,
  { force = false, fetcher = fetchTokenGroupCatalog } = {},
) {
  if (!userId || !names.length)
    return Promise.resolve({ catalog: {}, vendors: {} });
  const key = cacheKey(userId, names);
  const same = entry?.key === key ? entry : null;
  if (same?.promise) return same.promise;
  if (
    !force &&
    same?.value &&
    Date.now() - same.at < TOKEN_GROUP_CATALOG_FRESH_MS
  )
    return Promise.resolve(same.value);
  const promise = fetcher(names).then(
    (value) => {
      if (entry?.promise === promise)
        entry = { key, at: Date.now(), value, promise: null };
      return value;
    },
    (error) => {
      // Keep the last good copy for display; the next load retries.
      if (entry?.promise === promise) entry = { ...entry, promise: null };
      throw error;
    },
  );
  entry = { key, at: same?.at || 0, value: same?.value || null, promise };
  return promise;
}

export async function prefetchTokenGroupCatalog(userId) {
  if (!userId) return;
  const res = await API.get('/api/user/self/groups', {
    disableDuplicate: true,
  });
  if (!res.data?.success) return;
  await loadTokenGroupCatalog(
    userId,
    catalogGroupNames(Object.keys(res.data.data || {})),
  );
}

export function resetTokenGroupCatalogForTest() {
  entry = null;
}
