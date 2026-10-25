import assert from 'node:assert/strict';
import test from 'node:test';
import {
  ANALYTICS_CONFIG_IDS,
  ANALYTICS_CONSENT_KEY,
  createAnalyticsConsentStore,
} from '../src/helpers/analyticsConsent.js';

const events = () => {
  const listeners = new Map();
  return {
    addEventListener(name, listener) {
      if (!listeners.has(name)) listeners.set(name, new Set());
      listeners.get(name).add(listener);
    },
    removeEventListener(name, listener) {
      listeners.get(name)?.delete(listener);
    },
    dispatch(name, event = {}) {
      for (const listener of [...(listeners.get(name) || [])]) listener(event);
    },
    count(name) {
      return listeners.get(name)?.size || 0;
    },
  };
};
const fixture = ({ choice, google = true, umami = true } = {}) => {
  const values = new Map(choice ? [[ANALYTICS_CONSENT_KEY, choice]] : []);
  const storage = {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  };
  const nodes = new Map();
  if (google) {
    nodes.set(ANALYTICS_CONFIG_IDS.google, {
      type: 'application/json',
      textContent: JSON.stringify({ measurementId: 'G-TEST123' }),
    });
  }
  if (umami) {
    nodes.set(ANALYTICS_CONFIG_IDS.umami, {
      type: 'application/json',
      textContent: JSON.stringify({
        websiteId: 'test-site',
        scriptUrl: 'https://stats.example.test/script.js',
      }),
    });
  }
  const appended = [];
  const cookieWrites = [];
  const document = {
    ...events(),
    visibilityState: 'visible',
    getElementById: (id) => nodes.get(id),
    createElement: () => ({
      setAttribute(name, value) {
        this[name] = value;
      },
      remove() {
        nodes.delete(this.id);
        this.removed = true;
      },
    }),
    head: {
      appendChild(node) {
        nodes.set(node.id, node);
        appended.push(node);
      },
    },
    get cookie() {
      return '_ga=analytics; _ga_TEST123=session; _gid=visitor; session=login; theme=dark';
    },
    set cookie(value) {
      cookieWrites.push(value);
    },
  };
  let reloads = 0;
  const window = {
    ...events(),
    document,
    localStorage: storage,
    location: {
      hostname: 'www.example.test',
      pathname: '/privacy-choices',
      reload: () => {
        reloads += 1;
      },
    },
  };
  const store = createAnalyticsConsentStore({ window, document });
  return {
    store,
    window,
    document,
    storage,
    values,
    nodes,
    appended,
    cookieWrites,
    get reloads() {
      return reloads;
    },
    changeStorage(value, event = {}) {
      if (value === null) values.delete(ANALYTICS_CONSENT_KEY);
      else values.set(ANALYTICS_CONSENT_KEY, value);
      window.dispatch('storage', {
        key: ANALYTICS_CONSENT_KEY,
        newValue: value,
        storageArea: storage,
        ...event,
      });
    },
  };
};

test('unset, denied and corrupted choices never load optional scripts', () => {
  for (const choice of [
    undefined,
    'denied',
    'true',
    'allow',
    '{"analytics":true}',
  ]) {
    const f = fixture({ choice });
    f.store.initialize();
    assert.equal(f.appended.length, 0);
    assert.equal(f.window.gtag, undefined);
    assert.equal(f.reloads, 0);
    assert.equal(f.store.getSnapshot().analyticsEnabled, false);
    assert.equal(
      f.store.getSnapshot().choice,
      choice === 'denied' ? 'denied' : 'unset',
    );
  }
});

test('an explicit saved grant loads each configured provider exactly once', () => {
  const f = fixture();
  f.store.initialize();
  assert.deepEqual(f.store.save(true), {
    success: true,
    reloadRequired: false,
  });
  assert.equal(f.values.get(ANALYTICS_CONSENT_KEY), 'granted');
  assert.equal(f.appended.length, 2);
  const google = f.appended.find((node) => node.id.includes('google'));
  const umami = f.appended.find((node) => node.id.includes('umami'));
  assert.match(
    google.src,
    /^https:\/\/www\.googletagmanager\.com\/gtag\/js\?id=G-TEST123$/,
  );
  assert.equal(umami['data-website-id'], 'test-site');
  assert.equal(f.window['ga-disable-G-TEST123'], false);
  const consent = Array.from(f.window.dataLayer[0]);
  assert.equal(consent[2].analytics_storage, 'granted');
  assert.equal(consent[2].ad_storage, 'denied');
  assert.equal(consent[2].ad_personalization, 'denied');
  f.store.initialize();
  f.store.initialize();
  f.store.save(true);
  assert.equal(f.appended.length, 2);
  assert.equal(f.window.count('storage'), 1);
  assert.equal(f.document.count('visibilitychange'), 1);
});

test('a prior grant loads on initialization, but no configuration makes no requests', () => {
  const f = fixture({ choice: 'granted' });
  f.store.initialize();
  assert.equal(f.appended.length, 2);
  const empty = fixture({ choice: 'granted', google: false, umami: false });
  empty.store.initialize();
  assert.equal(empty.appended.length, 0);
  assert.equal(empty.store.getSnapshot().configured, false);
  assert.equal(empty.store.getSnapshot().analyticsEnabled, false);
});

test('denying before any grant persists without an unnecessary reload', () => {
  const f = fixture();
  assert.deepEqual(f.store.save(false), {
    success: true,
    reloadRequired: false,
  });
  assert.equal(f.store.getSnapshot().choice, 'denied');
  assert.equal(f.appended.length, 0);
  assert.equal(f.reloads, 0);
});

test('revocation stops pending/loaded providers, disables GA, clears only analytics cookies and reloads once', () => {
  const f = fixture({ choice: 'granted' });
  f.store.initialize();
  assert.deepEqual(f.store.save(false), {
    success: true,
    reloadRequired: true,
  });
  assert.equal(f.window['ga-disable-G-TEST123'], true);
  assert.equal(f.store.getSnapshot().analyticsEnabled, false);
  assert.equal(f.reloads, 1);
  assert.ok(f.appended.every((node) => node.removed));
  assert.ok(f.cookieWrites.some((cookie) => cookie.startsWith('_ga=')));
  assert.ok(
    f.cookieWrites.some((cookie) => cookie.includes('Domain=.example.test')),
  );
  assert.ok(
    f.cookieWrites.every(
      (cookie) =>
        !cookie.startsWith('session=') && !cookie.startsWith('theme='),
    ),
  );
  f.changeStorage('granted');
  assert.equal(
    f.appended.length,
    2,
    'no tracker may restart while reload is pending',
  );
  assert.equal(f.reloads, 1);
});

test('Umami alone also reloads on revocation, including while its download is pending', () => {
  const f = fixture({ choice: 'granted', google: false });
  f.store.initialize();
  assert.equal(f.appended.length, 1);
  assert.equal(f.store.save(false).reloadRequired, true);
  assert.equal(f.reloads, 1);
});

test('denial during script append prevents another provider from starting', () => {
  const f = fixture();
  const originalAppend = f.document.head.appendChild;
  f.document.head.appendChild = (node) => {
    originalAppend(node);
    f.store.save(false);
  };
  f.store.save(true);
  assert.equal(f.appended.length, 1);
  assert.equal(f.reloads, 1);
});

test('unavailable storage fails closed initially and failed grant does not start analytics', () => {
  const f = fixture();
  Object.defineProperty(f.window, 'localStorage', {
    get() {
      throw new Error('storage blocked');
    },
  });
  f.store.initialize();
  assert.equal(f.store.getSnapshot().storageError, true);
  assert.equal(f.store.save(true).success, false);
  assert.equal(f.appended.length, 0);
  assert.equal(f.store.getSnapshot().choice, 'unset');
});

test('silent write failures are reported and do not start analytics', () => {
  const f = fixture();
  f.storage.setItem = () => {};
  f.store.initialize();
  assert.equal(f.store.save(true).success, false);
  assert.equal(f.appended.length, 0);
  assert.equal(f.store.getSnapshot().choice, 'unset');
  assert.equal(f.store.getSnapshot().storageError, true);
});

test('failed revocation never reloads an old stored grant or falsely reports a saved denial', () => {
  const f = fixture({ choice: 'granted' });
  f.store.initialize();
  f.storage.setItem = () => {
    throw new Error('full');
  };
  assert.deepEqual(f.store.save(false), {
    success: false,
    reloadRequired: false,
  });
  assert.equal(f.reloads, 0);
  assert.equal(f.store.getSnapshot().choice, 'granted');
  assert.equal(f.store.getSnapshot().storageError, true);
  assert.equal(f.values.get(ANALYTICS_CONSENT_KEY), 'granted');
  f.storage.setItem = (key, value) => f.values.set(key, value);
  assert.equal(f.store.save(false).success, true);
  assert.equal(f.reloads, 1);
});

test('cross-tab grant and revocation synchronize from current storage, not a stale event payload', () => {
  const f = fixture();
  f.store.initialize();
  f.changeStorage('granted');
  assert.equal(f.appended.length, 2);
  f.changeStorage('denied', { newValue: 'granted' });
  assert.equal(f.store.getSnapshot().choice, 'denied');
  assert.equal(f.reloads, 1);
});

test('storage clear revokes consent; unrelated keys and sessionStorage events are ignored', () => {
  const f = fixture({ choice: 'granted' });
  f.store.initialize();
  f.window.dispatch('storage', { key: 'theme', storageArea: f.storage });
  f.window.dispatch('storage', { key: ANALYTICS_CONSENT_KEY, storageArea: {} });
  assert.equal(f.reloads, 0);
  f.changeStorage(null, { key: null });
  assert.equal(f.store.getSnapshot().choice, 'unset');
  assert.equal(f.reloads, 1);
});

test('restored/focused pages recheck choices after a missed cross-tab event', () => {
  for (const event of ['pageshow', 'visibilitychange']) {
    const f = fixture({ choice: 'granted' });
    f.store.initialize();
    f.values.set(ANALYTICS_CONSENT_KEY, 'denied');
    if (event === 'pageshow') f.window.dispatch(event, { persisted: true });
    else f.document.dispatch(event);
    assert.equal(f.reloads, 1, event);
    assert.equal(f.store.getSnapshot().choice, 'denied', event);
  }
});

test('invalid, executable or malformed inert config never loads a provider', () => {
  for (const content of [
    '{broken',
    JSON.stringify({ websiteId: 'site', scriptUrl: 'javascript:alert(1)' }),
    JSON.stringify({
      websiteId: 'site',
      scriptUrl: 'data:text/javascript,alert(1)',
    }),
    JSON.stringify({
      websiteId: 'site',
      scriptUrl: 'https://user:password@example.test/script.js',
    }),
  ]) {
    const f = fixture({ choice: 'granted', google: false });
    f.nodes.get(ANALYTICS_CONFIG_IDS.umami).textContent = content;
    f.store.initialize();
    assert.equal(f.appended.length, 0);
  }
  const f = fixture({ choice: 'granted', google: false });
  f.nodes.get(ANALYTICS_CONFIG_IDS.umami).type = 'text/javascript';
  f.store.initialize();
  assert.equal(f.appended.length, 0);
});

test('snapshot is stable between changes and subscriptions can be removed', () => {
  const f = fixture();
  f.store.initialize();
  const before = f.store.getSnapshot();
  assert.equal(f.store.getSnapshot(), before);
  let calls = 0;
  const unsubscribe = f.store.subscribe(() => {
    calls += 1;
  });
  f.store.save(false);
  assert.equal(calls, 1);
  assert.notEqual(f.store.getSnapshot(), before);
  unsubscribe();
  f.store.save(true);
  assert.equal(calls, 1);
  f.store.dispose();
  assert.equal(f.window.count('storage'), 0);
  assert.equal(f.document.count('visibilitychange'), 0);
});

test('a failing third-party gtag cannot interrupt withdrawal', () => {
  const f = fixture({ choice: 'granted' });
  f.store.initialize();
  f.window.gtag = () => {
    throw new Error('third-party runtime error');
  };
  assert.deepEqual(f.store.save(false), {
    success: true,
    reloadRequired: true,
  });
  assert.equal(f.window['ga-disable-G-TEST123'], true);
  assert.equal(f.reloads, 1);
  assert.ok(f.appended.every((node) => node.removed));
  assert.ok(f.cookieWrites.length > 0);
});
