import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import {
  CONSOLE_LANGUAGE_KEY,
  PUBLIC_LANGUAGE_KEY,
  PUBLIC_LANGUAGES,
  createPublicLanguageStore,
  normalizePublicLanguage,
} from '../src/helpers/publicLanguage.js';
import { SUPPORTED_LOCALES } from '../src/helpers/localeNormalize.js';

const expectedLanguages = [
  'en',
  'zh-CN',
  'zh-TW',
  'ja',
  'ko',
  'de',
  'es',
  'fr',
  'pt',
  'ru',
  'tr',
  'pl',
  'nl',
  'it',
  'vi',
  'uk',
  'bg',
];
const readLocale = (name) =>
  JSON.parse(
    readFileSync(
      new URL(`../src/helpers/publicLocales/${name}.json`, import.meta.url),
      'utf8',
    ),
  );
const deferred = () => {
  let resolve;
  let reject;
  const promise = new Promise((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
};
const memoryStorage = (initial = {}) => {
  const values = new Map(Object.entries(initial));
  const writes = [];
  return {
    values,
    writes,
    getItem: (key) => values.get(key) ?? null,
    setItem(key, value) {
      writes.push([key, value]);
      values.set(key, value);
    },
  };
};
const fixture = (options = {}) => {
  const storage = options.storage ?? memoryStorage();
  const store = createPublicLanguageStore({
    storage: () => storage,
    browserLanguage: () => 'en-US',
    load: async (language) => ({ default: { 首页: `home-${language}` } }),
    ...options,
    ...(typeof options.storage === 'function'
      ? {}
      : { storage: () => storage }),
  });
  return { store, storage };
};

test('public menu has exactly 17 distinct languages in the intended order', () => {
  assert.deepEqual(
    PUBLIC_LANGUAGES.map(([code]) => code),
    expectedLanguages,
  );
  assert.equal(new Set(PUBLIC_LANGUAGES.map(([code]) => code)).size, 17);
  assert.ok(
    PUBLIC_LANGUAGES.every(
      ([, label]) => typeof label === 'string' && label.trim(),
    ),
  );
  for (const language of expectedLanguages) {
    assert.equal(normalizePublicLanguage(language), language);
  }
});

test('browser and saved aliases resolve consistently without losing Chinese variants', () => {
  const aliases = {
    'en-US': 'en',
    ' EN_gb ': 'en',
    zh: 'zh-CN',
    zh_CN: 'zh-CN',
    'zh-Hans': 'zh-CN',
    'zh-SG': 'zh-CN',
    zh_TW: 'zh-TW',
    'ZH-hant': 'zh-TW',
    'zh-Hant-TW': 'zh-TW',
    'zh-HK': 'zh-TW',
    'zh-MO': 'zh-TW',
    'ja-JP': 'ja',
    ko_KR: 'ko',
    'de-DE': 'de',
    'es-419': 'es',
    'fr-CA': 'fr',
    'pt-BR': 'pt',
    'ru-RU': 'ru',
    'tr-TR': 'tr',
    'pl-PL': 'pl',
    'nl-NL': 'nl',
    'it-IT': 'it',
    'vi-VN': 'vi',
    'uk-UA': 'uk',
    'bg-BG': 'bg',
    unsupported: 'en',
  };
  for (const [alias, language] of Object.entries(aliases)) {
    assert.equal(normalizePublicLanguage(alias), language, alias);
  }
  for (const missing of [undefined, null, '', 123]) {
    assert.equal(normalizePublicLanguage(missing), 'zh-CN');
  }
});

test('all 15 lazy dictionaries cover the 137 public keys and preserve API identifiers', () => {
  const keys = readLocale('keys');
  const english = readLocale('en');
  assert.equal(keys.length, 137);
  assert.equal(new Set(keys).size, 137);
  assert.ok(
    keys.every(
      (key) => typeof english[key] === 'string' && english[key].trim(),
    ),
  );
  const languages = expectedLanguages.filter(
    (language) => !['en', 'zh-CN'].includes(language),
  );
  assert.equal(languages.length, 15);
  const identifiers = [
    ['示例使用 Chat Completions，其他接口请参阅文档。', ['Chat Completions']],
    ['兼容主流 SDK', ['SDK']],
    [
      '将示例中的 API_KEY 和 MODEL_ID 替换为你的配置。',
      ['API_KEY', 'MODEL_ID'],
    ],
    [
      '兼容 OpenAI、Claude、Gemini 等常见接口。按接入文档配置地址、密钥和模型即可；不同模型支持的参数可能有所不同。',
      ['OpenAI', 'Claude', 'Gemini'],
    ],
    ['GitHub 备用下载', ['GitHub']],
    ['下载安卓 APK', ['APK']],
    ['下载 DMG', ['DMG']],
  ];
  for (const language of languages) {
    const dictionary = readLocale(language);
    assert.deepEqual(
      Object.keys(dictionary).sort(),
      [...keys].sort(),
      language,
    );
    for (const key of keys) {
      assert.ok(
        typeof dictionary[key] === 'string' && dictionary[key].trim(),
        `${language}: ${key}`,
      );
      // Short homographs such as Console, Team and Agent are legitimate in
      // several languages. Full English sentences must not be placeholders.
      if (english[key].length >= 25) {
        assert.notEqual(
          dictionary[key],
          english[key],
          `${language}: untranslated ${key}`,
        );
      }
    }
    for (const [key, tokens] of identifiers) {
      for (const token of tokens)
        assert.ok(dictionary[key].includes(token), `${language}: ${token}`);
    }
  }
});

test('real lazy JSON imports can select and translate every public language', async () => {
  const { store } = fixture({ load: undefined });
  for (const language of expectedLanguages) {
    assert.equal(await store.change(language), true, language);
    assert.equal(store.getSnapshot().language, language);
    const expected =
      language === 'zh-CN' ? '首页' : readLocale(language)['首页'];
    assert.equal(store.translate('首页'), expected);
  }
});

test('the console keeps its seven supported languages while new public choices fall back to English there', async () => {
  const consoleLanguages = ['zh-CN', 'zh-TW', 'en', 'fr', 'ru', 'ja', 'vi'];
  assert.deepEqual(SUPPORTED_LOCALES, consoleLanguages);
  const { store, storage } = fixture();
  for (const language of expectedLanguages) {
    await store.change(language);
    assert.equal(storage.getItem(PUBLIC_LANGUAGE_KEY), language);
    assert.equal(
      storage.getItem(CONSOLE_LANGUAGE_KEY),
      consoleLanguages.includes(language) ? language : 'en',
    );
    assert.equal(store.getSnapshot().language, language);
  }
});

test('a lazy selection changes visible language and storage only after loading succeeds', async () => {
  const load = deferred();
  const { store, storage } = fixture({ load: () => load.promise });
  const before = store.getSnapshot();
  const change = store.change('fr-CA');
  assert.equal(store.getSnapshot().language, 'en');
  assert.equal(store.getSnapshot().dictionary, before.dictionary);
  assert.equal(store.getSnapshot().pending, 'fr');
  assert.equal(store.translate('首页'), 'Home');
  assert.deepEqual(storage.writes, []);
  load.resolve({ default: { 首页: 'Accueil' } });
  assert.equal(await change, true);
  assert.deepEqual(
    { ...store.getSnapshot(), dictionary: undefined },
    {
      language: 'fr',
      dictionary: undefined,
      pending: null,
      error: false,
    },
  );
  assert.equal(store.translate('首页'), 'Accueil');
  assert.equal(storage.getItem(PUBLIC_LANGUAGE_KEY), 'fr');
});

test('a failed import preserves the previous language and can be retried', async () => {
  let attempts = 0;
  const { store, storage } = fixture({
    load: async () => {
      if (++attempts === 1) throw new Error('temporary network failure');
      return { default: { 首页: 'Startseite' } };
    },
  });
  const dictionary = store.getSnapshot().dictionary;
  assert.equal(await store.change('de'), false);
  assert.equal(store.getSnapshot().language, 'en');
  assert.equal(store.getSnapshot().dictionary, dictionary);
  assert.equal(store.getSnapshot().pending, null);
  assert.equal(store.getSnapshot().error, true);
  assert.deepEqual(storage.writes, []);
  assert.equal(await store.change('de'), true);
  assert.equal(attempts, 2);
  assert.equal(store.getSnapshot().error, false);
  assert.equal(store.translate('首页'), 'Startseite');
});

test('out-of-order loads and stale errors cannot replace the most recent choice', async () => {
  for (const staleFails of [false, true]) {
    const french = deferred();
    const german = deferred();
    const { store, storage } = fixture({
      load: (language) => (language === 'fr' ? french.promise : german.promise),
    });
    const first = store.change('fr');
    const second = store.change('de');
    german.resolve({ 首页: 'Startseite' });
    assert.equal(await second, true);
    if (staleFails) french.reject(new Error('late failure'));
    else french.resolve({ 首页: 'Accueil' });
    assert.equal(await first, false);
    assert.equal(store.getSnapshot().language, 'de');
    assert.equal(store.getSnapshot().pending, null);
    assert.equal(store.getSnapshot().error, false);
    assert.equal(storage.getItem(PUBLIC_LANGUAGE_KEY), 'de');
  }
});

test('returning to the visible language cancels a pending choice without another import', async () => {
  const french = deferred();
  let loads = 0;
  const { store, storage } = fixture({
    load: () => {
      loads++;
      return french.promise;
    },
  });
  const pending = store.change('fr');
  assert.equal(await store.change('en'), true);
  french.resolve({ 首页: 'Accueil' });
  assert.equal(await pending, false);
  assert.equal(store.getSnapshot().language, 'en');
  assert.equal(store.getSnapshot().pending, null);
  assert.equal(storage.getItem(PUBLIC_LANGUAGE_KEY), 'en');
  assert.equal(loads, 1);
  assert.equal(await store.change('fr'), true);
  assert.equal(loads, 1, 'a superseded successful import can be reused');
});

test('repeated requests for one pending language share its import', async () => {
  const french = deferred();
  let loads = 0;
  const { store } = fixture({
    load: () => {
      loads++;
      return french.promise;
    },
  });
  const first = store.change('fr');
  const second = store.change('fr');
  await Promise.resolve();
  assert.equal(loads, 1);
  french.resolve({ 首页: 'Accueil' });
  assert.equal(await first, false);
  assert.equal(await second, true);
  assert.equal(store.getSnapshot().language, 'fr');
});

test('storage access, read and write errors never prevent a successful in-memory choice', async () => {
  const unavailable = () => {
    throw new Error('storage blocked');
  };
  for (const storage of [
    unavailable,
    { getItem: unavailable, setItem: unavailable },
    { getItem: () => 'en', setItem: unavailable },
  ]) {
    const { store } = fixture({ storage, browserLanguage: () => 'en' });
    assert.equal(store.getSnapshot().language, 'en');
    assert.equal(await store.change('fr'), true);
    assert.equal(store.getSnapshot().language, 'fr');
    assert.equal(store.getSnapshot().error, false);
    assert.equal(store.translate('首页'), 'home-fr');
  }
});

test('restore keeps the chosen language across page changes when persistence is blocked', async () => {
  const unavailable = () => {
    throw new Error('storage blocked');
  };
  for (const storage of [
    unavailable,
    () => null,
    { getItem: () => 'en', setItem: unavailable },
    { getItem: unavailable, setItem: unavailable },
  ]) {
    let loads = 0;
    const { store } = fixture({
      storage,
      load: async () => {
        loads++;
        return { 首页: 'Startseite' };
      },
    });
    assert.equal(await store.change('de'), true);
    const selected = store.getSnapshot();
    for (let visit = 0; visit < 3; visit++) {
      assert.equal(store.restore(), undefined);
      assert.equal(store.getSnapshot(), selected);
      assert.equal(store.translate('首页'), 'Startseite');
    }
    assert.equal(
      loads,
      1,
      'page restoration must not reload or revert the selected language',
    );
  }
});

test('restore respects public preference over console fallback and does not rewrite storage', async () => {
  const { store, storage } = fixture({
    storage: memoryStorage({
      [PUBLIC_LANGUAGE_KEY]: 'de-DE',
      [CONSOLE_LANGUAGE_KEY]: 'en',
    }),
  });
  assert.equal(store.getSnapshot().language, 'en');
  assert.equal(await store.restore(), true);
  assert.equal(store.getSnapshot().language, 'de');
  assert.deepEqual(storage.writes, []);
  assert.equal(store.restore(), undefined);
  storage.values.set(CONSOLE_LANGUAGE_KEY, 'en');
  assert.equal(
    store.handleStorage({ key: CONSOLE_LANGUAGE_KEY, storageArea: storage }),
    undefined,
  );
  assert.equal(store.getSnapshot().language, 'de');
});

test('initial restore uses a legacy console preference, then the browser when none is saved', async () => {
  for (const [values, browserLanguage, expected] of [
    [{ [CONSOLE_LANGUAGE_KEY]: 'fr-CA' }, 'ja-JP', 'fr'],
    [{}, 'ja-JP', 'ja'],
    [{}, 'zh-Hant-TW', 'zh-TW'],
    [{}, 'unsupported', 'en'],
  ]) {
    const { store, storage } = fixture({
      storage: memoryStorage(values),
      browserLanguage: () => browserLanguage,
    });
    await store.restore();
    assert.equal(store.getSnapshot().language, expected);
    assert.deepEqual(storage.writes, []);
  }
});

test('cross-tab public and legacy changes load the preferred language without echo writes', async () => {
  const { store, storage } = fixture();
  storage.values.set(PUBLIC_LANGUAGE_KEY, 'es');
  await store.handleStorage({ key: PUBLIC_LANGUAGE_KEY, storageArea: storage });
  assert.equal(store.getSnapshot().language, 'es');
  storage.values.delete(PUBLIC_LANGUAGE_KEY);
  storage.values.set(CONSOLE_LANGUAGE_KEY, 'ja');
  await store.handleStorage({ key: PUBLIC_LANGUAGE_KEY, storageArea: storage });
  assert.equal(store.getSnapshot().language, 'ja');
  storage.values.set(CONSOLE_LANGUAGE_KEY, 'vi');
  await store.handleStorage({
    key: CONSOLE_LANGUAGE_KEY,
    storageArea: storage,
  });
  assert.equal(store.getSnapshot().language, 'vi');
  storage.values.clear();
  await store.handleStorage({ key: null, storageArea: storage });
  assert.equal(store.getSnapshot().language, 'en');
  assert.deepEqual(storage.writes, []);
});

test('unrelated storage keys do not cancel an in-progress user selection', async () => {
  const french = deferred();
  const { store, storage } = fixture({ load: () => french.promise });
  const selection = store.change('fr');
  const snapshot = store.getSnapshot();
  assert.equal(
    store.handleStorage({ key: 'unrelated-setting', storageArea: storage }),
    undefined,
  );
  assert.equal(store.getSnapshot(), snapshot);
  french.resolve({ 首页: 'Accueil' });
  assert.equal(await selection, true);
  assert.equal(store.getSnapshot().language, 'fr');
});

test('storage events from another storage area do not cancel an in-progress choice', async () => {
  const french = deferred();
  const { store } = fixture({ load: () => french.promise });
  const selection = store.change('fr');
  const snapshot = store.getSnapshot();
  const ignored = store.handleStorage({
    key: PUBLIC_LANGUAGE_KEY,
    storageArea: memoryStorage(),
  });
  assert.equal(ignored, undefined);
  assert.equal(store.getSnapshot(), snapshot);
  french.resolve({ 首页: 'Accueil' });
  assert.equal(await selection, true);
});

test('snapshot subscriptions unsubscribe cleanly and missing keys use readable fallbacks', async () => {
  const { store } = fixture();
  let notifications = 0;
  const unsubscribe = store.subscribe(() => notifications++);
  assert.equal(store.getSnapshot(), store.getSnapshot());
  await store.change('fr');
  assert.ok(notifications > 0);
  assert.equal(store.translate('产品'), 'Products');
  assert.equal(store.translate('不存在的词条'), '不存在的词条');
  const previous = notifications;
  unsubscribe();
  await store.change('zh-CN');
  assert.equal(notifications, previous);
  assert.equal(store.translate('产品'), '产品');
});
