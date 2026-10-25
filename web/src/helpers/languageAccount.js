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

import { normalizeSiteLanguage, SITE_LANGUAGES } from './siteLanguage.js';

export const PENDING_ACCOUNT_LANGUAGE_KEY = 'site-language-pending-account';
const preferenceIdentity = (identity) =>
  identity == null ? null : String(identity);

export const createLanguageIdentityGuard =
  ({ getIdentity, storage = () => globalThis.window?.localStorage }) =>
  (identity) => {
    const expected = preferenceIdentity(identity);
    if (preferenceIdentity(getIdentity()) !== expected) return false;
    let cached;
    try {
      const target = storage();
      if (!target) return true;
      cached = target.getItem('user');
    } catch {
      // A blocked cache cannot invalidate an otherwise active session.
      return true;
    }
    try {
      return preferenceIdentity(JSON.parse(cached || 'null')?.id) === expected;
    } catch {
      return false;
    }
  };

export const createPendingAccountLanguage = (
  storage = () => globalThis.window?.localStorage,
) => ({
  read: (identity) => {
    try {
      const value = JSON.parse(
        storage()?.getItem(PENDING_ACCOUNT_LANGUAGE_KEY) || 'null',
      );
      return value?.accountId === preferenceIdentity(identity) &&
        SITE_LANGUAGES.some(([language]) => language === value.language)
        ? value.language
        : null;
    } catch {
      return null;
    }
  },
  write: (identity, language) => {
    try {
      const value = JSON.stringify({
        accountId: preferenceIdentity(identity),
        language: normalizeSiteLanguage(language),
      });
      const target = storage();
      target.setItem(PENDING_ACCOUNT_LANGUAGE_KEY, value);
      return target.getItem(PENDING_ACCOUNT_LANGUAGE_KEY) === value;
    } catch {
      return false;
    }
  },
  clear: (identity, language) => {
    try {
      const target = storage();
      const value = JSON.parse(
        target?.getItem(PENDING_ACCOUNT_LANGUAGE_KEY) || 'null',
      );
      if (
        value?.accountId === preferenceIdentity(identity) &&
        value.language === language
      )
        target.removeItem(PENDING_ACCOUNT_LANGUAGE_KEY);
    } catch {
      /* Keep an unsaved choice rather than erase another account's marker. */
    }
  },
});

// Independent applications record intent only. The main site's single bridge
// claims an anonymous choice once after login and performs the account write.
export const recordExplicitLanguageChoice = (
  language,
  storage = () => globalThis.window?.localStorage,
) => {
  let identity = null;
  try {
    const user = JSON.parse(storage()?.getItem('user') || 'null');
    if (typeof user?.id === 'string' || typeof user?.id === 'number')
      identity = user.id;
  } catch {
    /* An anonymous intent is still useful when no account is cached. */
  }
  return createPendingAccountLanguage(storage).write(identity, language);
};

// One serial queue per mounted account bridge. A queued choice supersedes older
// choices, while identity generations isolate responses from a previous login.
export const createLanguageSaveQueue = ({
  save,
  onSaved = () => {},
  onState = () => {},
  isIdentityCurrent = () => true,
}) => {
  let identity = null;
  let generation = 0;
  let sequence = 0;
  let queued = null;
  let running = null;
  const sameIdentity = (job) =>
    job.generation === generation && job.identity === identity;
  const current = (job) => sameIdentity(job) && isIdentityCurrent(job.identity);
  const drain = () => {
    if (running || !queued) return;
    const job = queued;
    queued = null;
    running = job;
    Promise.resolve()
      .then(() => {
        if (!current(job)) return;
        return save(job.identity, job.language);
      })
      .then(
        () => {
          if (current(job) && job.sequence === sequence)
            onSaved(job.identity, job.language);
        },
        () => {
          if (current(job) && job.sequence === sequence)
            onState({ accountSaving: false, accountError: true });
        },
      )
      .finally(() => {
        if (running === job) running = null;
        if (sameIdentity(job) && job.sequence === sequence)
          onState({ accountSaving: false });
        drain();
      });
  };
  return {
    setIdentity: (value) => {
      const next = value == null ? null : String(value);
      if (next === identity) return;
      identity = next;
      generation++;
      queued = null;
      onState({ accountSaving: false, accountError: false });
    },
    enqueue: (language) => {
      if (identity === null) return;
      queued = { identity, generation, language, sequence: ++sequence };
      onState({ accountSaving: true, accountError: false });
      drain();
    },
  };
};

export const readAccountLanguage = (user) => {
  try {
    const settings =
      typeof user?.setting === 'string'
        ? JSON.parse(user.setting)
        : user?.setting;
    return typeof settings?.language === 'string' ? settings.language : null;
  } catch {
    return null;
  }
};

// Fresh account preferences are restored once per identity, only if no newer
// user intent (including a still-loading selection) has appeared in this tab.
export const createLanguageAccountSync = ({
  store,
  readAccount,
  save,
  onSaved = () => {},
  pendingAccount = createPendingAccountLanguage(),
  isIdentityCurrent = () => true,
}) => {
  let identity = null;
  let generation = 0;
  let disposed = false;
  let guestRevision = store.getSnapshot().userRevision;
  let observedRevision = store.getSnapshot().revision;
  let observedIntent = store.getSnapshot().userRevision;
  let intentIdentity = null;
  const queue = createLanguageSaveQueue({
    save,
    isIdentityCurrent,
    onSaved: (id, language) => {
      pendingAccount.clear(id, language);
      onSaved(id, language);
    },
    onState: store.setAccountStatus,
  });
  const enqueue = (language) => {
    // Another tab may have logged out or replaced the cached account before
    // this tab's UserContext catches up. Never claim its pending preference.
    if (!isIdentityCurrent(identity)) return;
    if (!pendingAccount.write(identity, language))
      store.setAccountStatus({ storageError: true });
    queue.enqueue(language);
  };
  const unsubscribe = store.subscribe(() => {
    const state = store.getSnapshot();
    if (state.userRevision !== observedIntent) {
      observedIntent = state.userRevision;
      intentIdentity = identity;
    }
    if (state.revision === observedRevision) return;
    observedRevision = state.revision;
    if (
      (identity === null &&
        intentIdentity === null &&
        state.source === 'user') ||
      (identity !== null &&
        (intentIdentity === null || intentIdentity === identity) &&
        (state.source === 'user' || state.source === 'storage'))
    )
      enqueue(state.language);
  });
  return {
    setIdentity: (value) => {
      const next = value == null ? null : String(value);
      if (next === identity || disposed) return;
      const previous = identity;
      identity = next;
      const id = ++generation;
      store.cancelAccountRestore();
      queue.setIdentity(next);
      const isCurrent = () =>
        !disposed &&
        generation === id &&
        identity === next &&
        isIdentityCurrent(next);
      const state = store.getSnapshot();
      if (next === null) {
        guestRevision = state.userRevision;
        return;
      }
      if (!isCurrent()) return;
      if (previous === null && state.userRevision > guestRevision) {
        // Claim an anonymous intent on this first login, even if its resource
        // download is pending. A later account cannot inherit that request.
        if (intentIdentity === null) intentIdentity = next;
        if (!state.pending && !state.error) enqueue(state.language);
        guestRevision = state.userRevision;
        return;
      }
      const expectedRevision = state.userRevision;
      let unsaved = pendingAccount.read(next);
      if (!unsaved && previous === null) {
        unsaved = pendingAccount.read(null);
        if (unsaved && !pendingAccount.write(next, unsaved))
          store.setAccountStatus({ storageError: true });
      }
      if (unsaved) {
        store
          .adoptAccountLanguage(unsaved, expectedRevision, isCurrent)
          .then((success) => {
            if (
              success &&
              !disposed &&
              generation === id &&
              identity === next &&
              store.getSnapshot().userRevision === expectedRevision
            )
              enqueue(unsaved);
          });
        return;
      }
      Promise.resolve()
        .then(() => {
          if (!isCurrent()) return null;
          return readAccount(next);
        })
        .then((account) => {
          if (!isCurrent() || String(account?.id) !== next) return;
          return store.adoptAccountLanguage(
            readAccountLanguage(account),
            expectedRevision,
            isCurrent,
          );
        })
        .catch(() => {
          // Account lookup failure must not overwrite a valid local preference.
        });
    },
    dispose: () => {
      disposed = true;
      generation++;
      unsubscribe();
      store.cancelAccountRestore();
      queue.setIdentity(null);
    },
  };
};
