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

import { normalizeSiteLanguage } from './siteLanguage.js';

export const connectSiteLanguageI18n = ({
  i18n,
  store,
  loaders,
  initPromise,
}) => {
  const applyLanguage = i18n.changeLanguage.bind(i18n);
  const inflight = new Map();
  const loadLocale = async (language) => {
    await initPromise;
    if (i18n.hasResourceBundle(language, 'translation')) return;
    if (!inflight.has(language)) {
      inflight.set(
        language,
        loaders[language]()
          .then((module) => {
            const resources = module.default || module;
            i18n.addResourceBundle(
              language,
              'translation',
              resources.translation || resources,
              true,
              true,
            );
          })
          .finally(() => inflight.delete(language)),
      );
    }
    await inflight.get(language);
  };
  const unregister = store.registerLoader(loadLocale);

  let appliedRevision = -1;
  let applied = Promise.resolve();
  const synchronize = () => {
    const state = store.getSnapshot();
    if (
      state.revision === appliedRevision ||
      !i18n.hasResourceBundle(state.language, 'translation')
    )
      return applied;
    appliedRevision = state.revision;
    // All resources already exist, so i18next changes synchronously and never
    // waits on a stale network request after the store commits a newer choice.
    applied = applyLanguage(state.language);
    return applied;
  };
  const unsubscribe = store.subscribe(synchronize);

  // Compatibility for existing i18next callers; new UI uses useSiteLanguage.
  const requestLanguage = async (language, callback) => {
    const success = await store.change(normalizeSiteLanguage(language));
    if (!success) {
      const error = new Error(
        'Language resources could not be loaded or selection was superseded',
      );
      if (callback) callback(error);
      return i18n.t.bind(i18n);
    }
    await synchronize();
    const translate = i18n.t.bind(i18n);
    if (callback) callback(null, translate);
    return translate;
  };

  i18n.changeLanguage = requestLanguage;

  // Wait for the latest selection, even if it supersedes the first request while
  // AppShell is loading. Failed downloads retain the existing readable language.
  const ready = initPromise.then(async () => {
    store.initialize();
    for (;;) {
      const success = await store.whenReady();
      await synchronize();
      if (!success || !store.getSnapshot().pending) return;
    }
  });
  return {
    ready,
    dispose: () => {
      unsubscribe();
      unregister();
      if (i18n.changeLanguage === requestLanguage)
        i18n.changeLanguage = applyLanguage;
    },
  };
};
