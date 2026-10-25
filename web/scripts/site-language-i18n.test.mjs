import assert from 'node:assert/strict';
import test from 'node:test';
import { createInstance } from 'i18next';
import { createSiteLanguageStore } from '../src/helpers/siteLanguage.js';
import { connectSiteLanguageI18n } from '../src/helpers/siteLanguageI18n.js';
const deferred = () => {
  let resolve, reject;
  const promise = new Promise((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
};
const tick = async () => {
  for (let i = 0; i < 20; i++) await Promise.resolve();
};
const fixture = () =>
  createSiteLanguageStore({
    storage: () => undefined,
    browserLanguage: () => 'en',
  });
const connect = (store, loaders) => {
  const i18n = createInstance();
  const initPromise = i18n.init({
    lng: store.getSnapshot().language,
    resources: {},
    fallbackLng: 'zh-CN',
    returnEmptyString: false,
    nsSeparator: false,
  });
  const bridge = connectSiteLanguageI18n({ i18n, store, loaders, initPromise });
  return { i18n, ...bridge };
};

test('public choice in flight immediately entering console waits for both resources and translates the same language', async () => {
  const store = fixture();
  const publicFrench = deferred();
  const consoleFrench = deferred();
  store.registerLoader((language) =>
    language === 'fr' ? publicFrench.promise : undefined,
  );
  const choice = store.change('fr');
  const { i18n, ready, dispose } = connect(store, {
    en: async () => ({ 首页: 'Home' }),
    fr: () => consoleFrench.promise,
  });
  let resolved = false;
  ready.then(() => {
    resolved = true;
  });
  publicFrench.resolve();
  await tick();
  assert.equal(resolved, false);
  consoleFrench.resolve({ 首页: 'Accueil', 空值: '' });
  await choice;
  await ready;
  assert.equal(store.getSnapshot().language, 'fr');
  assert.equal(i18n.language, 'fr');
  assert.equal(i18n.t('首页'), 'Accueil');
  assert.equal(i18n.t('空值'), '空值');
  dispose();
});

test('console initial ready follows a later public selection while the original dictionary is stuck', async () => {
  const store = fixture();
  const english = deferred();
  const japanese = deferred();
  const { i18n, ready, dispose } = connect(store, {
    en: () => english.promise,
    ja: () => japanese.promise,
  });
  await tick();
  const choice = store.change('ja');
  japanese.resolve({ 首页: 'ホーム' });
  await choice;
  let resolved = false;
  ready.then(() => {
    resolved = true;
  });
  await tick();
  assert.equal(resolved, true);
  assert.equal(i18n.language, 'ja');
  assert.equal(i18n.t('首页'), 'ホーム');
  dispose();
});

test('legacy console changeLanguage also commits to the canonical store and does not win over newer selection', async () => {
  const store = fixture();
  const french = deferred();
  const { i18n, ready, dispose } = connect(store, {
    en: async () => ({ 首页: 'Home' }),
    fr: () => french.promise,
    de: async () => ({ 首页: 'Startseite' }),
  });
  await ready;
  const old = i18n.changeLanguage('fr');
  await i18n.changeLanguage('de');
  french.resolve({ 首页: 'Accueil' });
  await old;
  assert.equal(store.getSnapshot().language, 'de');
  assert.equal(i18n.language, 'de');
  assert.equal(i18n.t('首页'), 'Startseite');
  dispose();
});
