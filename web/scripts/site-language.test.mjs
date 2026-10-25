import assert from 'node:assert/strict';
import test from 'node:test';
import {
  createSiteLanguageStore,
  SITE_LANGUAGE_KEY,
  SITE_LANGUAGES,
} from '../src/helpers/siteLanguage.js';
import {
  createLanguageSaveQueue,
  createLanguageAccountSync,
  createLanguageIdentityGuard,
  createPendingAccountLanguage,
} from '../src/helpers/languageAccount.js';
const deferred = () => {
  let resolve, reject;
  const promise = new Promise((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
};
const tick = async () => {
  for (let i = 0; i < 12; i++) await Promise.resolve();
};
const memory = (initial = {}) => {
  const values = new Map(Object.entries(initial));
  const writes = [];
  return {
    values,
    writes,
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => {
      writes.push([key, value]);
      values.set(key, value);
    },
    removeItem: (key) => values.delete(key),
  };
};
const fixture = (initial = {}) => {
  const storage = memory(initial);
  const store = createSiteLanguageStore({
    storage: () => storage,
    browserLanguage: () => 'en-US',
  });
  return { storage, store };
};
const windowFixture = () => {
  const listeners = new Map();
  const add = (name, callback) => {
    if (!listeners.has(name)) listeners.set(name, []);
    listeners.get(name).push(callback);
  };
  return {
    addEventListener: add,
    document: {
      documentElement: {},
      visibilityState: 'visible',
      addEventListener: add,
    },
    listeners,
    fire: (name) => listeners.get(name)?.forEach((fn) => fn()),
  };
};

test('canonical storage wins; legacy values migrate once without losing any of 17 languages', async () => {
  for (const [language] of SITE_LANGUAGES) {
    const { store, storage } = fixture({
      'public-language': language,
      i18nextLng: 'en',
    });
    store.initialize(windowFixture());
    assert.equal(store.getSnapshot().language, language);
    assert.equal(storage.getItem(SITE_LANGUAGE_KEY), language);
    assert.equal(storage.getItem('public-language'), null);
    assert.equal(storage.getItem('i18nextLng'), null);
  }
  const { store, storage } = fixture({
    [SITE_LANGUAGE_KEY]: 'ko',
    'public-language': 'fr',
  });
  store.initialize(windowFixture());
  assert.equal(store.getSnapshot().language, 'ko');
  assert.equal(storage.getItem('public-language'), null);
});

test('language commits only after every registered surface is ready', async () => {
  const { store, storage } = fixture();
  const publicLoad = deferred();
  const consoleLoad = deferred();
  store.registerLoader(() => publicLoad.promise);
  const choice = store.change('uk');
  store.registerLoader(() => consoleLoad.promise);
  publicLoad.resolve();
  await tick();
  assert.equal(store.getSnapshot().language, 'en');
  assert.equal(storage.writes.length, 0);
  consoleLoad.resolve();
  assert.equal(await choice, true);
  assert.equal(store.getSnapshot().language, 'uk');
});

test('initial readiness follows newest choice even when superseded download never settles', async () => {
  const { store } = fixture();
  const french = deferred();
  const german = deferred();
  store.registerLoader((language) =>
    language === 'fr' ? french.promise : german.promise,
  );
  store.change('fr');
  let ready = false;
  const wait = store.whenReady().then((value) => {
    ready = value;
  });
  const newest = store.change('de');
  german.resolve();
  await newest;
  await tick();
  assert.equal(ready, true);
  await wait;
  assert.equal(store.getSnapshot().language, 'de');
});

test('late failures cannot replace latest success; failed latest request can be retried', async () => {
  const { store } = fixture();
  const french = deferred();
  let failures = 0;
  store.registerLoader((language) =>
    language === 'fr'
      ? french.promise
      : ++failures === 1
        ? Promise.reject(new Error('offline'))
        : undefined,
  );
  const old = store.change('fr');
  assert.equal(await store.change('bg'), false);
  assert.equal(await store.whenReady(), false);
  assert.equal(await store.change('bg'), true);
  french.reject(new Error('late failure'));
  assert.equal(await old, false);
  assert.equal(store.getSnapshot().language, 'bg');
  assert.equal(store.getSnapshot().error, false);
});

test('storage failures keep in-memory choice across restoration and report persistence failure', async () => {
  const store = createSiteLanguageStore({
    storage: () => {
      throw new Error('blocked');
    },
    browserLanguage: () => 'en',
  });
  await store.change('tr');
  assert.equal(store.getSnapshot().storageError, true);
  assert.equal(store.restore(), undefined);
  assert.equal(store.getSnapshot().language, 'tr');
});

test('initialization is idempotent and document, focus, pageshow and cross-tab restoration agree', async () => {
  const { store, storage } = fixture();
  const win = windowFixture();
  store.initialize(win);
  store.initialize(win);
  assert.equal(win.listeners.get('storage').length, 1);
  await store.change('pl');
  assert.equal(win.document.documentElement.lang, 'pl');
  storage.values.set(SITE_LANGUAGE_KEY, 'it');
  win.fire('focus');
  await tick();
  assert.equal(store.getSnapshot().language, 'it');
  storage.values.set(SITE_LANGUAGE_KEY, 'nl');
  win.fire('pageshow');
  await tick();
  assert.equal(store.getSnapshot().language, 'nl');
  storage.values.set(SITE_LANGUAGE_KEY, 'pt');
  await store.handleStorage({
    key: SITE_LANGUAGE_KEY,
    newValue: 'stale-fr',
    storageArea: storage,
  });
  assert.equal(store.getSnapshot().language, 'pt');
  assert.equal(win.document.documentElement.lang, 'pt');
  assert.equal(
    storage.writes.length,
    1,
    'external choices do not echo back to storage',
  );
});

test('page restoration cannot cancel a pending explicit choice', async () => {
  const { store } = fixture();
  const load = deferred();
  store.registerLoader(() => load.promise);
  const choice = store.change('es');
  store.restore();
  load.resolve();
  await choice;
  assert.equal(store.getSnapshot().language, 'es');
});

test('account restore cannot override a later explicit intent, even while it loads', async () => {
  const { store } = fixture();
  const load = deferred();
  store.registerLoader(() => load.promise);
  const revision = store.getSnapshot().userRevision;
  const choice = store.change('ja');
  assert.equal(await store.adoptAccountLanguage('fr', revision), false);
  load.resolve();
  await choice;
  assert.equal(store.getSnapshot().language, 'ja');
});

for (const action of ['logout', 'dispose']) {
  test(`${action} cancels an account restore already loading without blocking initial readiness`, async () => {
    const { store, storage } = fixture();
    const load = deferred();
    store.registerLoader((language) =>
      language === 'fr' ? load.promise : undefined,
    );
    const sync = createLanguageAccountSync({
      store,
      readAccount: async (id) => ({ id, setting: '{"language":"fr"}' }),
      save: () => assert.fail('restoration must not save'),
    });
    sync.setIdentity(1);
    await tick();
    assert.equal(store.getSnapshot().pending, 'fr');
    let ready = false;
    const waiting = store.whenReady().then((value) => {
      ready = value;
    });
    if (action === 'logout') sync.setIdentity(null);
    else sync.dispose();
    assert.equal(store.getSnapshot().pending, null);
    await tick();
    assert.equal(ready, true, 'readiness cannot wait for the old download');
    await waiting;
    assert.equal(store.getSnapshot().language, 'en');
    assert.deepEqual(storage.writes, []);
    load.resolve();
    await tick();
    assert.equal(store.getSnapshot().language, 'en');
    assert.deepEqual(storage.writes, []);
    sync.dispose();
  });
}

test('replacement account lookup failure cannot leave an old account restore active', async () => {
  const { store, storage } = fixture();
  const load = deferred();
  store.registerLoader((language) =>
    language === 'fr' ? load.promise : undefined,
  );
  const sync = createLanguageAccountSync({
    store,
    readAccount: async (id) => {
      if (id === '2') throw new Error('offline');
      return { id, setting: '{"language":"fr"}' };
    },
    save: () => assert.fail('restoration must not save'),
  });
  sync.setIdentity(1);
  await tick();
  assert.equal(store.getSnapshot().pending, 'fr');
  sync.setIdentity(2);
  await tick();
  load.resolve();
  await tick();
  assert.equal(store.getSnapshot().language, 'en');
  assert.equal(store.getSnapshot().pending, null);
  assert.deepEqual(storage.writes, []);
  sync.dispose();
});

test('logout preserves an explicit choice that is still loading', async () => {
  const { store } = fixture();
  const load = deferred();
  const saves = [];
  store.registerLoader((language) =>
    language === 'fr' ? load.promise : undefined,
  );
  const sync = createLanguageAccountSync({
    store,
    readAccount: async (id) => ({ id }),
    save: (...args) => saves.push(args),
  });
  sync.setIdentity(1);
  await tick();
  const choice = store.change('fr');
  sync.setIdentity(null);
  assert.equal(store.getSnapshot().pending, 'fr');
  load.resolve();
  assert.equal(await choice, true);
  assert.equal(store.getSnapshot().language, 'fr');
  assert.deepEqual(saves, []);
  sync.dispose();
});

test('account adoption rechecks its identity guard immediately before committing', async () => {
  const { store, storage } = fixture();
  const load = deferred();
  store.registerLoader(() => load.promise);
  let current = true;
  const choice = store.adoptAccountLanguage('fr', 0, () => current);
  current = false;
  load.resolve();
  assert.equal(await choice, false);
  assert.equal(store.getSnapshot().language, 'en');
  assert.equal(store.getSnapshot().pending, null);
  assert.equal(store.getSnapshot().error, false);
  assert.deepEqual(storage.writes, []);
});

test('save queue serializes and coalesces rapid choices, ignoring superseded callbacks', async () => {
  const calls = [];
  const saved = [];
  const first = deferred();
  const queue = createLanguageSaveQueue({
    save: (id, language) => {
      calls.push([id, language]);
      return calls.length === 1 ? first.promise : undefined;
    },
    onSaved: (...args) => saved.push(args),
  });
  queue.setIdentity(7);
  queue.enqueue('fr');
  await tick();
  queue.enqueue('de');
  queue.enqueue('ko');
  assert.deepEqual(calls, [['7', 'fr']]);
  first.resolve();
  await tick();
  assert.deepEqual(calls, [
    ['7', 'fr'],
    ['7', 'ko'],
  ]);
  assert.deepEqual(saved, [['7', 'ko']]);
});

test('logout and account replacement invalidate old save responses and queued requests', async () => {
  const first = deferred();
  const calls = [];
  const saved = [];
  const states = [];
  const queue = createLanguageSaveQueue({
    save: (id, language) => {
      calls.push([id, language]);
      return calls.length === 1 ? first.promise : undefined;
    },
    onSaved: (...args) => saved.push(args),
    onState: (value) => states.push(value),
  });
  queue.setIdentity(7);
  queue.enqueue('fr');
  await tick();
  queue.enqueue('de');
  queue.setIdentity(null);
  queue.setIdentity(8);
  queue.enqueue('it');
  first.reject(new Error('old login failed'));
  await tick();
  assert.deepEqual(calls, [
    ['7', 'fr'],
    ['8', 'it'],
  ]);
  assert.deepEqual(saved, [['8', 'it']]);
  assert.equal(
    states.some((state) => state.accountError === true),
    false,
  );
});

test('failed account save is explicit, never rolls back local choice and can retry same language', async () => {
  const { store } = fixture();
  let attempts = 0;
  const sync = createLanguageAccountSync({
    store,
    readAccount: async () => ({ id: 1 }),
    save: () => {
      if (++attempts === 1) throw new Error('offline');
    },
  });
  sync.setIdentity(1);
  await tick();
  await store.change('bg');
  await tick();
  assert.equal(store.getSnapshot().language, 'bg');
  assert.equal(store.getSnapshot().accountError, true);
  await store.change('bg');
  await tick();
  assert.equal(attempts, 2);
  assert.equal(store.getSnapshot().accountError, false);
  sync.dispose();
});

test('fresh account preference restores once; late reads and disposed bridge cannot overwrite selection', async () => {
  const { store } = fixture();
  const account = deferred();
  let reads = 0;
  const saves = [];
  const sync = createLanguageAccountSync({
    store,
    readAccount: () => {
      reads++;
      return account.promise;
    },
    save: (...args) => saves.push(args),
  });
  sync.setIdentity(1);
  await tick();
  await store.change('ja');
  account.resolve({ id: 1, setting: JSON.stringify({ language: 'fr' }) });
  await tick();
  sync.setIdentity(1);
  assert.equal(reads, 1);
  assert.equal(store.getSnapshot().language, 'ja');
  assert.deepEqual(saves, [['1', 'ja']]);
  sync.dispose();
  const late = deferred();
  const second = createLanguageAccountSync({
    store,
    readAccount: () => late.promise,
    save: () => {},
  });
  second.setIdentity(2);
  second.dispose();
  late.resolve({ id: 2, setting: '{"language":"ru"}' });
  await tick();
  assert.equal(store.getSnapshot().language, 'ja');
});

test('a guest choice survives login; a different account restores its own preference', async () => {
  const { store } = fixture();
  const saves = [];
  const sync = createLanguageAccountSync({
    store,
    readAccount: async (id) => ({ id, setting: '{"language":"ru"}' }),
    save: (...args) => saves.push(args),
  });
  await store.change('de');
  sync.setIdentity(1);
  await tick();
  assert.equal(store.getSnapshot().language, 'de');
  assert.deepEqual(saves, [['1', 'de']]);
  sync.setIdentity(2);
  await tick();
  assert.equal(store.getSnapshot().language, 'ru');
  sync.dispose();
});

test('selection started under a previous identity cannot save into a replacement account', async () => {
  const { store } = fixture();
  const load = deferred();
  const account = deferred();
  const saves = [];
  store.registerLoader((language) =>
    language === 'fr' ? load.promise : undefined,
  );
  const sync = createLanguageAccountSync({
    store,
    readAccount: (id) =>
      id === '2' ? account.promise : Promise.resolve({ id: 1 }),
    save: (...args) => saves.push(args),
  });
  sync.setIdentity(1);
  await tick();
  const selection = store.change('fr');
  sync.setIdentity(2);
  load.resolve();
  await selection;
  await tick();
  assert.deepEqual(saves, []);
  account.resolve({ id: 2, setting: '{"language":"ko"}' });
  await tick();
  assert.equal(store.getSnapshot().language, 'ko');
  sync.dispose();
});

test('an unsaved account choice survives refresh and retries without leaking into another identity', async () => {
  const { createPendingAccountLanguage, PENDING_ACCOUNT_LANGUAGE_KEY } =
    await import('../src/helpers/languageAccount.js');
  const storage = memory();
  const pendingAccount = createPendingAccountLanguage(() => storage);
  const makeStore = () =>
    createSiteLanguageStore({
      storage: () => storage,
      browserLanguage: () => 'en',
    });
  const first = makeStore();
  const sync = createLanguageAccountSync({
    store: first,
    pendingAccount,
    readAccount: async () => ({ id: 1, setting: '{"language":"ru"}' }),
    save: () => {
      throw new Error('offline');
    },
  });
  sync.setIdentity(1);
  await tick();
  await first.change('de');
  await tick();
  sync.dispose();
  assert.equal(pendingAccount.read(1), 'de');
  assert.equal(pendingAccount.read(2), null);
  const refreshed = makeStore();
  let reads = 0;
  const saves = [];
  const restored = createLanguageAccountSync({
    store: refreshed,
    pendingAccount,
    readAccount: async (id) => {
      reads++;
      return { id, setting: '{"language":"ja"}' };
    },
    save: (...args) => saves.push(args),
  });
  restored.setIdentity(1);
  await tick();
  assert.equal(refreshed.getSnapshot().language, 'de');
  assert.equal(reads, 0);
  assert.deepEqual(saves, [['1', 'de']]);
  assert.equal(storage.getItem(PENDING_ACCOUNT_LANGUAGE_KEY), null);
  pendingAccount.write(1, 'fr');
  restored.setIdentity(2);
  await tick();
  assert.equal(refreshed.getSnapshot().language, 'ja');
  assert.equal(pendingAccount.read(1), 'fr');
  restored.dispose();
});

test('late success cannot erase a newer pending account marker', async () => {
  const { createPendingAccountLanguage } = await import(
    '../src/helpers/languageAccount.js'
  );
  const storage = memory();
  const pending = createPendingAccountLanguage(() => storage);
  pending.write(1, 'de');
  pending.clear(1, 'fr');
  assert.equal(pending.read(1), 'de');
  pending.clear(2, 'de');
  assert.equal(pending.read(1), 'de');
  pending.clear(1, 'de');
  assert.equal(pending.read(1), null);
});

test('anonymous explicit choice survives an OAuth-style reload, is claimed once and never follows another account', async () => {
  const { createPendingAccountLanguage, recordExplicitLanguageChoice } =
    await import('../src/helpers/languageAccount.js');
  const storage = memory({ 'site-language': 'pl' });
  assert.equal(
    recordExplicitLanguageChoice('pl', () => storage),
    true,
  );
  const pendingAccount = createPendingAccountLanguage(() => storage);
  assert.equal(pendingAccount.read(null), 'pl');
  const store = createSiteLanguageStore({
    storage: () => storage,
    browserLanguage: () => 'en',
  });
  const saves = [];
  let reads = 0;
  const request = deferred();
  const sync = createLanguageAccountSync({
    store,
    pendingAccount,
    readAccount: async (id) => {
      reads++;
      return { id, setting: '{"language":"ja"}' };
    },
    save: (...args) => {
      saves.push(args);
      return request.promise;
    },
  });
  sync.setIdentity(1);
  await tick();
  assert.equal(store.getSnapshot().language, 'pl');
  assert.equal(pendingAccount.read(null), null);
  assert.equal(pendingAccount.read(1), 'pl');
  assert.equal(reads, 0);
  sync.setIdentity(null);
  sync.setIdentity(2);
  await tick();
  assert.equal(store.getSnapshot().language, 'ja');
  assert.deepEqual(saves, [['1', 'pl']]);
  request.resolve();
  await tick();
  assert.equal(store.getSnapshot().language, 'ja');
  assert.equal(pendingAccount.read(1), 'pl');
  sync.dispose();
});

test('anonymous pending download is bound to the first login and cannot save into a subsequent account', async () => {
  const { store } = fixture();
  const load = deferred();
  const account = deferred();
  const saves = [];
  store.registerLoader((language) =>
    language === 'fr' ? load.promise : undefined,
  );
  const sync = createLanguageAccountSync({
    store,
    readAccount: () => account.promise,
    save: (...args) => saves.push(args),
  });
  const selection = store.change('fr');
  sync.setIdentity(1);
  sync.setIdentity(2);
  load.resolve();
  await selection;
  await tick();
  assert.deepEqual(saves, []);
  account.resolve({ id: 2, setting: '{"language":"de"}' });
  await tick();
  assert.equal(store.getSnapshot().language, 'de');
  sync.dispose();
});

test('shared external helper binds an authenticated choice to only the cached account and reports storage errors', async () => {
  const { createPendingAccountLanguage, recordExplicitLanguageChoice } =
    await import('../src/helpers/languageAccount.js');
  const storage = memory({ user: '{"id":7}' });
  assert.equal(
    recordExplicitLanguageChoice('ko', () => storage),
    true,
  );
  const pending = createPendingAccountLanguage(() => storage);
  assert.equal(pending.read(7), 'ko');
  assert.equal(pending.read(8), null);
  assert.equal(pending.read(null), null);
  assert.equal(
    recordExplicitLanguageChoice('fr', () => {
      throw new Error('blocked');
    }),
    false,
  );
});

test('a stale tab cannot overwrite another account pending choice after a cross-tab login', async () => {
  const storage = memory({ user: '{"id":1}', 'site-language': 'en' });
  const pendingAccount = createPendingAccountLanguage(() => storage);
  const makeStore = () =>
    createSiteLanguageStore({
      storage: () => storage,
      browserLanguage: () => 'en',
    });
  const oldStore = makeStore();
  const oldSaves = [];
  const oldSync = createLanguageAccountSync({
    store: oldStore,
    pendingAccount,
    isIdentityCurrent: createLanguageIdentityGuard({
      getIdentity: () => 1,
      storage: () => storage,
    }),
    readAccount: async (id) => ({ id }),
    save: (...args) => oldSaves.push(args),
  });
  oldSync.setIdentity(1);
  await tick();

  // Login in a second tab changes shared storage before the old UserContext.
  storage.setItem('user', '{"id":2}');
  const activeStore = makeStore();
  const activeSync = createLanguageAccountSync({
    store: activeStore,
    pendingAccount,
    isIdentityCurrent: createLanguageIdentityGuard({
      getIdentity: () => 2,
      storage: () => storage,
    }),
    readAccount: async (id) => ({ id }),
    save: async () => {
      throw new Error('offline');
    },
  });
  activeSync.setIdentity(2);
  await tick();
  await activeStore.change('de');
  await tick();
  assert.equal(pendingAccount.read(2), 'de');
  assert.equal(activeStore.getSnapshot().accountError, true);
  await oldStore.handleStorage({
    key: SITE_LANGUAGE_KEY,
    storageArea: storage,
  });
  await tick();
  assert.equal(
    oldStore.getSnapshot().language,
    'de',
    'display still follows the browser choice',
  );
  assert.deepEqual(oldSaves, []);
  assert.equal(pendingAccount.read(2), 'de');
  activeSync.dispose();

  const refreshed = makeStore();
  const retrySaves = [];
  const retry = createLanguageAccountSync({
    store: refreshed,
    pendingAccount,
    isIdentityCurrent: createLanguageIdentityGuard({
      getIdentity: () => 2,
      storage: () => storage,
    }),
    readAccount: async () => assert.fail('pending choice must survive refresh'),
    save: (...args) => retrySaves.push(args),
  });
  retry.setIdentity(2);
  await tick();
  assert.equal(refreshed.getSnapshot().language, 'de');
  assert.deepEqual(retrySaves, [['2', 'de']]);
  oldSync.dispose();
  retry.dispose();
});

test('cross-tab logout invalidates a queued save and late success without clearing another account marker', async () => {
  const storage = memory({ user: '{"id":1}' });
  const pendingAccount = createPendingAccountLanguage(() => storage);
  const store = createSiteLanguageStore({
    storage: () => storage,
    browserLanguage: () => 'en',
  });
  const request = deferred();
  const saves = [];
  const saved = [];
  const sync = createLanguageAccountSync({
    store,
    pendingAccount,
    isIdentityCurrent: createLanguageIdentityGuard({
      getIdentity: () => 1,
      storage: () => storage,
    }),
    readAccount: async (id) => ({ id }),
    save: (...args) => {
      saves.push(args);
      return request.promise;
    },
    onSaved: (...args) => saved.push(args),
  });
  sync.setIdentity(1);
  await tick();
  await store.change('fr');
  await tick();
  await store.change('de');
  await tick();
  storage.removeItem('user');
  pendingAccount.write(null, 'pl');
  request.resolve();
  await tick();
  assert.deepEqual(
    saves,
    [['1', 'fr']],
    'queued choice must not run after external logout',
  );
  assert.deepEqual(saved, []);
  assert.equal(pendingAccount.read(null), 'pl');
  assert.equal(store.getSnapshot().accountSaving, false);
  sync.dispose();
});

test('an account restore cannot commit after a different tab replaces the cached identity', async () => {
  const storage = memory({ user: '{"id":1}', 'site-language': 'en' });
  const store = createSiteLanguageStore({
    storage: () => storage,
    browserLanguage: () => 'en',
  });
  const language = deferred();
  store.registerLoader((value) =>
    value === 'fr' ? language.promise : undefined,
  );
  const sync = createLanguageAccountSync({
    store,
    pendingAccount: createPendingAccountLanguage(() => storage),
    isIdentityCurrent: createLanguageIdentityGuard({
      getIdentity: () => 1,
      storage: () => storage,
    }),
    readAccount: async (id) => ({ id, setting: '{"language":"fr"}' }),
    save: () => assert.fail('stale restoration must not save'),
  });
  sync.setIdentity(1);
  await tick();
  assert.equal(store.getSnapshot().pending, 'fr');
  storage.setItem('user', '{"id":2}');
  language.resolve();
  await tick();
  assert.equal(store.getSnapshot().language, 'en');
  assert.equal(storage.getItem(SITE_LANGUAGE_KEY), 'en');
  sync.dispose();
});

test('identity guard distinguishes unavailable storage from explicit logout, changed or malformed cache', () => {
  let identity = 1;
  const storage = memory({ user: '{"id":1}' });
  const check = createLanguageIdentityGuard({
    getIdentity: () => identity,
    storage: () => storage,
  });
  assert.equal(check('1'), true);
  storage.setItem('user', '{"id":2}');
  assert.equal(check('1'), false);
  storage.removeItem('user');
  assert.equal(check('1'), false);
  identity = null;
  assert.equal(check(null), true);
  storage.setItem('user', 'invalid json');
  assert.equal(check(null), false);
  const unavailable = createLanguageIdentityGuard({
    getIdentity: () => 1,
    storage: () => {
      throw new Error('blocked');
    },
  });
  assert.equal(unavailable('1'), true);
  assert.equal(unavailable('2'), false);
});
