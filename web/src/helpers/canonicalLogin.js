// Authentication lives on one origin; model/API routes stay independently selectable.
import { getSafeInternalRedirectPath } from './authRedirect.js';

export const META_LOGIN_ORIGIN = 'https://meta-api.com';
const ORIGINS = new Set([
  META_LOGIN_ORIGIN,
  'https://meta-api.vip',
  'https://cn.meta-api.vip',
  'https://nan.meta-api.vip',
]);
const validRequest = (value) =>
  typeof value === 'string' && /^[A-Za-z0-9_-]{43}$/.test(value);
export function loginOrigin(origin) {
  return ORIGINS.has(origin) ? META_LOGIN_ORIGIN : origin;
}
function safeNext(value) {
  if (typeof value !== 'string' || value.length > 4096) return '';
  const safe = getSafeInternalRedirectPath(value);
  if (!safe) return '';
  const parsed = new URL(safe, META_LOGIN_ORIGIN);
  if (parsed.pathname === '/desktop-login') {
    const ids = parsed.searchParams.getAll('request_id');
    return ids.length === 1 && validRequest(ids[0])
      ? `/desktop-login?request_id=${ids[0]}`
      : '';
  }
  if (parsed.pathname === '/api/forum/sso/start') return parsed.pathname;
  if (parsed.pathname === '/console' || parsed.pathname.startsWith('/console/'))
    return parsed.pathname;
  return '';
}
export function canonicalLoginPage(origin, pathname, search = '') {
  if (!ORIGINS.has(origin) || origin === META_LOGIN_ORIGIN) return '';
  const path = pathname.replace(/\/$/, '');
  if (!['/login', '/register', '/desktop-login'].includes(path)) return '';
  const input = new URLSearchParams(search);
  const target = new URL(path, META_LOGIN_ORIGIN);
  if (path === '/desktop-login') {
    const ids = input.getAll('request_id');
    if (ids.length === 1 && validRequest(ids[0]))
      target.searchParams.set('request_id', ids[0]);
  } else {
    const values = input.getAll('next');
    const next = values.length === 1 ? safeNext(values[0]) : '';
    if (next) target.searchParams.set('next', next);
    const aff = input.getAll('aff');
    if (aff.length === 1 && /^[A-Za-z0-9_-]{1,64}$/.test(aff[0]))
      target.searchParams.set('aff', aff[0]);
  }
  return target.toString();
}
// Existing alternative-domain personal pages may still have bind buttons.
// Move to the primary personal page for an explicit second confirmation; never
// transfer the old account cookie or silently bind the primary site's account.
export function canonicalOAuthEntry(origin, options = {}) {
  if (!ORIGINS.has(origin) || origin === META_LOGIN_ORIGIN) return '';
  if (!options.shouldLogout) return `${META_LOGIN_ORIGIN}/console/personal`;
  const query = new URLSearchParams();
  if (options.postLoginPath) query.set('next', options.postLoginPath);
  return canonicalLoginPage(origin, '/login', query.toString());
}
