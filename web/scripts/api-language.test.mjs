import assert from 'node:assert/strict';
import test from 'node:test';
import { siteLanguageStore } from '../src/helpers/siteLanguage.js';

test('API requests send the chosen language and do not reuse a pending response in another language', async () => {
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = { getItem: () => null };
  const { API } = await import('../src/helpers/apiCore.js');
  const requests = [];
  const original = API.defaults.adapter;
  API.defaults.adapter = (config) =>
    new Promise((resolve) =>
      requests.push({
        config,
        finish: () =>
          resolve({
            config,
            status: 200,
            statusText: 'OK',
            headers: {},
            data: { language: config.headers['Accept-Language'] },
          }),
      }),
    );
  try {
    await siteLanguageStore.change('de');
    const first = API.get('/language-fixture', { skipGlobalLoading: true });
    await new Promise((resolve) => setTimeout(resolve, 0));
    const sameLanguage = API.get('/language-fixture', {
      skipGlobalLoading: true,
    });
    assert.equal(first, sameLanguage);
    await siteLanguageStore.change('uk');
    const second = API.get('/language-fixture', { skipGlobalLoading: true });
    await new Promise((resolve) => setTimeout(resolve, 0));
    assert.equal(requests.length, 2);
    assert.equal(requests[0].config.headers['Accept-Language'], 'de');
    assert.equal(requests[1].config.headers['Accept-Language'], 'uk');
    requests[1].finish();
    requests[0].finish();
    assert.equal((await second).data.language, 'uk');
    assert.equal((await first).data.language, 'de');
  } finally {
    API.defaults.adapter = original;
    if (previousStorage) globalThis.localStorage = previousStorage;
    else delete globalThis.localStorage;
  }
});

test('a queued GET keeps its captured language when selection changes before dispatch', async () => {
  const previousStorage = globalThis.localStorage;
  globalThis.localStorage = { getItem: () => null };
  const { API } = await import('../src/helpers/apiCore.js');
  const original = API.defaults.adapter;
  let dispatch;
  const gate = new Promise((resolve) => {
    dispatch = resolve;
  });
  const interceptor = API.interceptors.request.use(async (config) => {
    await gate;
    return config;
  });
  API.defaults.adapter = async (config) => ({
    config,
    status: 200,
    statusText: 'OK',
    headers: {},
    data: config.headers['Accept-Language'],
  });
  try {
    await siteLanguageStore.change('en');
    const first = API.get('/queued-language-fixture', {
      skipGlobalLoading: true,
    });
    await siteLanguageStore.change('de');
    const second = API.get('/queued-language-fixture', {
      skipGlobalLoading: true,
    });
    assert.notEqual(first, second);
    dispatch();
    assert.equal((await first).data, 'en');
    assert.equal((await second).data, 'de');
  } finally {
    dispatch();
    API.interceptors.request.eject(interceptor);
    API.defaults.adapter = original;
    if (previousStorage) globalThis.localStorage = previousStorage;
    else delete globalThis.localStorage;
  }
});
