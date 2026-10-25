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

export const ANALYTICS_CONSENT_KEY = 'analytics-consent:v1';
export const ANALYTICS_CONFIG_IDS = {
  umami: 'newapi-analytics-umami',
  google: 'newapi-analytics-google',
};

const parseChoice = (value) =>
  value === 'granted' || value === 'denied' ? value : 'unset';

const readConfiguration = (doc) => {
  const read = (id) => {
    try {
      const node = doc?.getElementById(id);
      if (node?.type !== 'application/json') return null;
      return JSON.parse(node.textContent);
    } catch {
      return null;
    }
  };
  const rawGoogle = read(ANALYTICS_CONFIG_IDS.google);
  const rawUmami = read(ANALYTICS_CONFIG_IDS.umami);
  const google =
    typeof rawGoogle?.measurementId === 'string' &&
    rawGoogle.measurementId.trim()
      ? { measurementId: rawGoogle.measurementId.trim() }
      : null;
  let umami = null;
  try {
    const url = new URL(rawUmami?.scriptUrl);
    if (
      ['http:', 'https:'].includes(url.protocol) &&
      !url.username &&
      !url.password &&
      typeof rawUmami?.websiteId === 'string' &&
      rawUmami.websiteId.trim()
    ) {
      umami = { websiteId: rawUmami.websiteId.trim(), scriptUrl: url.href };
    }
  } catch {
    // Missing or invalid configuration never enables optional requests.
  }
  return { google, umami };
};

const clearAnalyticsCookies = (win, doc) => {
  try {
    const names = doc.cookie
      .split(';')
      .map((cookie) => cookie.trim().split('=')[0])
      .filter((name) =>
        /^(_ga(?:_|$)|_gid$|_gat(?:_|$)|_gac_|_gcl_|AMP_TOKEN$)/.test(name),
      );
    const host = win.location.hostname;
    const domains = new Set(['', host, `.${host}`]);
    const labels = host.split('.');
    for (let index = 1; index < labels.length - 1; index += 1) {
      const domain = labels.slice(index).join('.');
      domains.add(domain);
      domains.add(`.${domain}`);
    }
    const paths = new Set(['/']);
    const segments = win.location.pathname.split('/').filter(Boolean);
    for (let length = 1; length <= segments.length; length += 1) {
      const path = `/${segments.slice(0, length).join('/')}`;
      paths.add(path);
      paths.add(`${path}/`);
    }
    for (const name of names) {
      for (const domain of domains) {
        for (const path of paths) {
          doc.cookie = `${name}=; Max-Age=0; Path=${path}${domain ? `; Domain=${domain}` : ''}; SameSite=Lax`;
        }
      }
    }
  } catch {
    // Cookie access can also be blocked; withdrawal still stops the scripts.
  }
};

// One store lives for the whole document. It is deliberately independent of
// component lifetimes so StrictMode and navigation cannot duplicate trackers.
export const createAnalyticsConsentStore = ({
  window: win = globalThis.window,
  document: doc = win?.document,
} = {}) => {
  const listeners = new Set();
  const scripts = new Set();
  let initialized = false;
  let started = false;
  let reloading = false;
  let configuration = { google: null, umami: null };
  let snapshot = {
    choice: 'unset',
    analyticsEnabled: false,
    configured: false,
    storageError: false,
  };
  const publish = (next) => {
    if (Object.keys(next).every((key) => snapshot[key] === next[key])) return;
    snapshot = { ...snapshot, ...next };
    for (const listener of [...listeners]) listener();
  };
  const read = () => {
    try {
      if (!win?.localStorage) throw new Error('storage unavailable');
      return {
        choice: parseChoice(win.localStorage.getItem(ANALYTICS_CONSENT_KEY)),
        storageError: false,
      };
    } catch {
      return { choice: 'unset', storageError: true };
    }
  };
  const appendScript = (id, src, attributes = {}) => {
    if (reloading || snapshot.choice !== 'granted') return;
    let script = doc.getElementById(id);
    if (!script) {
      script = doc.createElement('script');
      script.id = id;
      script.src = src;
      script.async = true;
      for (const [name, value] of Object.entries(attributes)) {
        script.setAttribute(name, value);
      }
      scripts.add(script);
      // Mark before append: even a pending download may have executed by revoke.
      started = true;
      doc.head.appendChild(script);
    } else {
      scripts.add(script);
      started = true;
    }
  };
  const stop = () => {
    const { google } = configuration;
    if (google) win[`ga-disable-${google.measurementId}`] = true;
    try {
      if (typeof win.gtag === 'function') {
        win.gtag('consent', 'update', { analytics_storage: 'denied' });
      }
    } catch {
      // A third-party runtime must not prevent cookie cleanup and withdrawal.
    }
    clearAnalyticsCookies(win, doc);
    for (const script of scripts) script.remove();
    if (started && !reloading) {
      // Removing a tag cannot stop an arbitrary Umami script's event handlers.
      reloading = true;
      win.location.reload();
    }
    return reloading;
  };
  const start = () => {
    if (started || reloading || !doc?.head) return;
    const { google, umami } = configuration;
    if (google) {
      win[`ga-disable-${google.measurementId}`] = false;
      win.dataLayer = win.dataLayer || [];
      win.gtag = function () {
        win.dataLayer.push(arguments);
      };
      // Advertising storage/signals are not part of this analytics permission.
      win.gtag('consent', 'default', {
        analytics_storage: 'granted',
        ad_storage: 'denied',
        ad_user_data: 'denied',
        ad_personalization: 'denied',
      });
      win.gtag('js', new Date());
      win.gtag('config', google.measurementId, {
        allow_google_signals: false,
        allow_ad_personalization_signals: false,
      });
      appendScript(
        'newapi-analytics-google-runtime',
        `https://www.googletagmanager.com/gtag/js?id=${encodeURIComponent(google.measurementId)}`,
      );
    }
    if (umami) {
      appendScript('newapi-analytics-umami-runtime', umami.scriptUrl, {
        'data-website-id': umami.websiteId,
      });
    }
  };
  const apply = (state) => {
    const granted = state.choice === 'granted' && !state.storageError;
    // Publish before a reload so subscribers never show an outdated grant.
    publish({
      ...state,
      analyticsEnabled: granted && snapshot.configured && !reloading,
    });
    if (granted) start();
    else stop();
  };
  const synchronize = () => {
    if (!reloading) apply(read());
  };
  const onStorage = (event) => {
    if (event.key !== ANALYTICS_CONSENT_KEY && event.key !== null) return;
    try {
      if (event.storageArea && event.storageArea !== win.localStorage) return;
    } catch {
      // If storage became unavailable, fail closed when synchronizing below.
    }
    // Read current storage: a queued older "grant" event cannot undo a revoke.
    synchronize();
  };
  const onVisibility = () => {
    if (doc.visibilityState === 'visible') synchronize();
  };
  const initialize = () => {
    if (initialized || !win || !doc) return snapshot;
    initialized = true;
    configuration = readConfiguration(doc);
    publish({
      configured: Boolean(configuration.google || configuration.umami),
    });
    win.addEventListener('storage', onStorage);
    win.addEventListener('pageshow', synchronize);
    doc.addEventListener('visibilitychange', onVisibility);
    synchronize();
    return snapshot;
  };
  const save = (allowed) => {
    initialize();
    if (typeof allowed !== 'boolean' || !win || reloading) {
      return { success: false, reloadRequired: reloading };
    }
    const choice = allowed ? 'granted' : 'denied';
    try {
      win.localStorage.setItem(ANALYTICS_CONSENT_KEY, choice);
      if (win.localStorage.getItem(ANALYTICS_CONSENT_KEY) !== choice) {
        throw new Error('storage did not retain preference');
      }
    } catch {
      // In particular, do not reload an old persisted grant after a failed deny.
      publish({ storageError: true });
      return { success: false, reloadRequired: false };
    }
    apply({ choice, storageError: false });
    return { success: true, reloadRequired: reloading };
  };
  return {
    initialize,
    save,
    getSnapshot: () => snapshot,
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    // Only for isolated document/test owners; React components should unsubscribe.
    dispose: () => {
      win?.removeEventListener('storage', onStorage);
      win?.removeEventListener('pageshow', synchronize);
      doc?.removeEventListener('visibilitychange', onVisibility);
      listeners.clear();
    },
  };
};

let store;
const getStore = () => {
  if (!store) store = createAnalyticsConsentStore();
  return store;
};
export const initializeAnalyticsConsent = () => getStore().initialize();
export const getAnalyticsConsentSnapshot = () => getStore().getSnapshot();
export const subscribeAnalyticsConsent = (listener) =>
  getStore().subscribe(listener);
export const saveAnalyticsConsent = (allowed) => getStore().save(allowed);
