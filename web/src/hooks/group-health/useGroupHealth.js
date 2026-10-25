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

import {
  useCallback,
  useContext,
  useEffect,
  useSyncExternalStore,
} from 'react';
import { UserContext } from '../../context/User';
import { API } from '../../helpers/apiCore';

const REFRESH_MS = 60000;
const EMPTY = { data: null, loading: false, error: null };
const sessions = new WeakMap();
const entries = new Map();
let nextSession = 0;

function sessionKey(user) {
  if (!user?.id) return '';
  if (!sessions.has(user)) sessions.set(user, ++nextSession);
  return `${user.id}:${user.group || ''}:${sessions.get(user)}`;
}

function getEntry(key) {
  if (!entries.has(key)) {
    // No persisted health data: each login/user object has an isolated cache.
    for (const [oldKey, old] of entries) {
      if (entries.size < 64) break;
      if (!old.listeners.size && !old.promise) entries.delete(oldKey);
    }
    entries.set(key, { snapshot: EMPTY, listeners: new Set(), fetchedAt: 0 });
  }
  return entries.get(key);
}

function publish(entry, snapshot) {
  entry.snapshot = snapshot;
  entry.listeners.forEach((notify) => notify());
}

function requestHealth(key, params, force = false) {
  const entry = getEntry(key);
  if (entry.promise) return entry.promise;
  if (!force && Date.now() - entry.fetchedAt < REFRESH_MS)
    return Promise.resolve();
  publish(entry, { ...entry.snapshot, loading: true });
  entry.promise = API.get('/api/health/groups', {
    params,
    skipErrorHandler: true,
    skipGlobalLoading: true,
    // API's generic dedupe omits account identity; this cache handles it instead.
    disableDuplicate: true,
    timeout: 15000,
  })
    .then(({ data: response }) => {
      if (!response?.success || !Array.isArray(response?.data?.groups)) {
        throw new Error(response?.message || 'Health data unavailable');
      }
      entry.fetchedAt = Date.now();
      publish(entry, { data: response.data, loading: false, error: null });
    })
    .catch((error) => {
      entry.fetchedAt = Date.now();
      publish(entry, { ...entry.snapshot, loading: false, error });
    })
    .finally(() => {
      entry.promise = null;
    });
  return entry.promise;
}

/** Authenticated, session-scoped health data shared across active surfaces. */
export function useGroupHealth({
  model,
  group,
  hours = 24,
  enabled = true,
} = {}) {
  const [userState] = useContext(UserContext);
  const identity = sessionKey(userState?.user);
  const active = enabled && Boolean(identity);
  const modelValue = model || '';
  const groupValue = group || '';
  const key = active
    ? JSON.stringify([identity, modelValue, groupValue, hours])
    : '';
  const subscribe = useCallback(
    (notify) => {
      if (!key) return () => {};
      const entry = getEntry(key);
      entry.listeners.add(notify);
      return () => entry.listeners.delete(notify);
    },
    [key],
  );
  const getSnapshot = useCallback(
    () => (key ? entries.get(key)?.snapshot || EMPTY : EMPTY),
    [key],
  );
  const snapshot = useSyncExternalStore(subscribe, getSnapshot, () => EMPTY);
  const refresh = useCallback(
    (force = true) => {
      if (!key) return Promise.resolve();
      return requestHealth(
        key,
        {
          ...(hours !== 24 ? { hours } : {}),
          ...(modelValue ? { model: modelValue } : {}),
          ...(groupValue ? { group: groupValue } : {}),
        },
        force,
      );
    },
    [key, modelValue, groupValue, hours],
  );

  useEffect(() => {
    if (!key) return;
    if (!document.hidden) refresh(false);
    const onVisible = () => {
      if (!document.hidden) refresh(false);
    };
    const timer = window.setInterval(onVisible, REFRESH_MS);
    document.addEventListener('visibilitychange', onVisible);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener('visibilitychange', onVisible);
    };
  }, [key, refresh]);

  return {
    ...snapshot,
    loading: active && (snapshot === EMPTY || snapshot.loading),
    refresh,
    isAuthenticated: Boolean(identity),
  };
}
