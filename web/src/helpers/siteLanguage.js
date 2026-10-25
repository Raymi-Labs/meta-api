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
  normalizeLocale,
  SITE_LANGUAGE_KEY,
  SITE_LANGUAGES,
} from './localeNormalize.js';
export { SITE_LANGUAGE_KEY, SITE_LANGUAGES } from './localeNormalize.js';
export const normalizeSiteLanguage = normalizeLocale;
const legacyKeys = ['public-language', 'i18nextLng'];

// The canonical choice is independent of rendering libraries. Registered
// resource loaders prepare every mounted surface before a choice is committed.
export const createSiteLanguageStore = ({
  storage = () => globalThis.window?.localStorage,
  browserLanguage = () => globalThis.navigator?.language || 'zh-CN',
} = {}) => {
  const listeners = new Set();
  const loaders = new Set();
  let loaderVersion = 0;
  let prepared = null;
  let request = 0;
  let currentPromise = null;
  let pendingSource = null;
  let transient = null;
  let initialized = false;
  let migrationComplete = false;
  const read = (key) => {
    try {
      return storage()?.getItem(key) || null;
    } catch {
      return null;
    }
  };
  const preference = () => {
    if (transient) return transient;
    const canonical = read(SITE_LANGUAGE_KEY);
    if (canonical) return normalizeLocale(canonical);
    if (!migrationComplete) {
      for (const key of legacyKeys) {
        const value = read(key);
        if (value) return normalizeLocale(value);
      }
    }
    return normalizeLocale(browserLanguage());
  };
  let snapshot = {
    language: preference(),
    pending: null,
    error: false,
    storageError: false,
    revision: 0,
    userRevision: 0,
    source: 'initial',
    accountSaving: false,
    accountError: false,
  };
  const publish = (next) => {
    if (Object.keys(next).every((key) => snapshot[key] === next[key])) return;
    snapshot = { ...snapshot, ...next };
    for (const listener of [...listeners]) listener();
  };
  const persist = (language) => {
    transient = language;
    try {
      const target = storage();
      if (!target) throw new Error('storage unavailable');
      target.setItem(SITE_LANGUAGE_KEY, language);
      if (target.getItem(SITE_LANGUAGE_KEY) !== language)
        throw new Error('storage unavailable');
      migrationComplete = true;
      for (const key of legacyKeys) target.removeItem?.(key);
      transient = null;
      return false;
    } catch {
      return true;
    }
  };
  const prepare = async (language) => {
    let version;
    do {
      version = loaderVersion;
      await Promise.all([...loaders].map((load) => load(language)));
    } while (version !== loaderVersion);
    return version;
  };
  const change = (
    value,
    { source = 'user', persist: save = true, isCurrent = () => true } = {},
  ) => {
    const language = normalizeLocale(value);
    const id = ++request;
    pendingSource = source;
    const userRevision =
      snapshot.userRevision +
      (source === 'user' || source === 'storage' ? 1 : 0);
    publish({ pending: language, error: false, userRevision });
    const operation = (async () => {
      try {
        const version = await prepare(language);
        if (id !== request) return false;
        if (!isCurrent()) {
          publish({ pending: null, error: false });
          return false;
        }
        prepared = { language, version };
        const storageError = save ? persist(language) : snapshot.storageError;
        publish({
          language,
          pending: null,
          error: false,
          storageError,
          revision: snapshot.revision + 1,
          source,
        });
        return true;
      } catch {
        if (id === request) publish({ pending: null, error: isCurrent() });
        return false;
      } finally {
        if (id === request) {
          currentPromise = null;
          pendingSource = null;
        }
      }
    })();
    currentPromise = operation;
    return operation;
  };
  const isPrepared = () =>
    prepared?.language === snapshot.language &&
    prepared.version === loaderVersion;
  const whenReady = async () => {
    for (;;) {
      if (!currentPromise) {
        if (snapshot.error) return false;
        if (isPrepared()) return true;
        change(snapshot.language, { source: 'restore', persist: false });
      }
      // A superseded download may never settle. Wake on store changes too,
      // so readiness follows the newest request instead of that old promise.
      await new Promise((resolve) => {
        const done = () => {
          listeners.delete(done);
          resolve();
        };
        listeners.add(done);
        currentPromise?.then(done);
      });
    }
  };
  const restore = () => {
    if (currentPromise) return;
    const language = preference();
    if (language !== snapshot.language)
      return change(language, { source: 'storage', persist: false });
    if (!isPrepared())
      return change(language, { source: 'restore', persist: false });
  };
  const handleStorage = (event) => {
    if (event.storageArea) {
      try {
        if (event.storageArea !== storage()) return;
      } catch {
        return;
      }
    }
    if (
      event.key !== null &&
      event.key !== SITE_LANGUAGE_KEY &&
      !(
        legacyKeys.includes(event.key) &&
        !migrationComplete &&
        !read(SITE_LANGUAGE_KEY)
      )
    )
      return;
    transient = null;
    if (event.key === SITE_LANGUAGE_KEY || event.key === null)
      migrationComplete = true;
    const language = preference();
    // Queued old events read the latest value; unrelated updates never cancel a load.
    if (language === (snapshot.pending || snapshot.language)) return;
    return change(language, { source: 'storage', persist: false });
  };
  const initialize = (win = globalThis.window) => {
    if (initialized || !win) return;
    initialized = true;
    // Public surfaces on separate origins exchange only an allowlisted locale.
    // Consume it once so a later explicit choice survives a refresh.
    if (win.location?.href) {
      const url = new URL(win.location.href);
      const requested = url.searchParams.get('lang');
      if (SITE_LANGUAGES.some(([language]) => language === requested)) {
        url.searchParams.delete('lang');
        win.history.replaceState(win.history.state, '', url);
        change(requested);
      }
    }
    if (!read(SITE_LANGUAGE_KEY) && legacyKeys.some((key) => read(key))) {
      publish({ storageError: persist(snapshot.language) });
    } else if (read(SITE_LANGUAGE_KEY)) {
      migrationComplete = true;
      try {
        for (const key of legacyKeys) storage()?.removeItem?.(key);
      } catch {
        /* Canonical preference still wins. */
      }
    }
    const updateDocument = () => {
      win.document.documentElement.lang = snapshot.language;
    };
    updateDocument();
    listeners.add(updateDocument);
    win.addEventListener('storage', handleStorage);
    win.addEventListener('pageshow', restore);
    win.addEventListener('focus', restore);
    const visible = () => {
      if (win.document.visibilityState === 'visible') restore();
    };
    win.document.addEventListener('visibilitychange', visible);
  };
  return {
    getSnapshot: () => snapshot,
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    change,
    restore,
    whenReady,
    handleStorage,
    initialize,
    registerLoader: (loader) => {
      loaders.add(loader);
      loaderVersion++;
      return () => {
        loaders.delete(loader);
        loaderVersion++;
      };
    },
    adoptAccountLanguage: (
      language,
      expectedUserRevision,
      isCurrent = () => true,
    ) => {
      const canAdopt = () =>
        snapshot.userRevision === expectedUserRevision && isCurrent();
      if (!language || !canAdopt()) return Promise.resolve(false);
      return change(language, { source: 'account', isCurrent: canAdopt });
    },
    cancelAccountRestore: () => {
      if (pendingSource !== 'account') return;
      request++;
      currentPromise = null;
      pendingSource = null;
      publish({ pending: null, error: false });
    },
    setAccountStatus: (next) => publish(next),
  };
};

export const siteLanguageStore = createSiteLanguageStore();
export const getSiteLanguageSnapshot = siteLanguageStore.getSnapshot;
export const subscribeSiteLanguage = siteLanguageStore.subscribe;
export const changeSiteLanguage = (language) =>
  siteLanguageStore.change(language);
export const initializeSiteLanguage = () => siteLanguageStore.initialize();
