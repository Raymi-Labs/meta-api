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

import assert from 'node:assert/strict';
import puppeteer from '../../../node_modules/puppeteer-core/lib/esm/puppeteer/puppeteer-core.js';

// Run against a dedicated local Vite server, using an isolated browser profile.
// No model request is submitted; fixtures are written only to this test origin.
const url = process.env.IMAGE_LANGUAGE_TEST_URL || 'http://localhost:5198/';
assert.ok(['localhost', '127.0.0.1', '[::1]'].includes(new URL(url).hostname));
const browser = await puppeteer.launch({
  executablePath:
    process.env.CHROME_BIN ||
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  headless: true,
});
try {
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.setViewport({ width: 1440, height: 1000 });
  await page.goto(url, { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('header select');
  await page.evaluate(async () => {
    // Use the same module instance as the UI after Vite adds HMR cache keys.
    const languageUrl = performance
      .getEntriesByType('resource')
      .map((entry) => entry.name)
      .find((name) => new URL(name).pathname === '/src/language.ts');
    window.__languageTestModule = await import(
      languageUrl || '/src/language.ts'
    );
    const { useStore, initStore } = await import('/src/store.ts');
    await initStore();
    window.__languageTestStore = useStore;
  });
  await page.evaluate(() => {
    const store = window.__languageTestStore;
    const task = {
      id: 'language-browser-task',
      status: 'running',
      prompt: '用户原始提示词 @图1',
      inputImageIds: [],
      outputImages: [],
      params: {
        size: '1024x1024',
        quality: 'auto',
        output_format: 'png',
        output_compression: 100,
        moderation: 'auto',
        n: 1,
      },
      createdAt: Date.now(),
      apiProvider: 'openai',
      apiModel: 'gpt-image-2',
    };
    store.setState({
      tasks: [task],
      prompt: '保留中文输入 @图1',
      showSettings: false,
    });
    window.__languageTestTasks = store.getState().tasks;
    window.__languageTestEditor = document.querySelector(
      '[contenteditable="true"]',
    );
    const settings = store.getState().settings;
    store.setState({
      settings: {
        ...settings,
        apiKey: 'sk-language-browser-fixture-not-a-real-token',
        profiles: settings.profiles.map((profile) => ({
          ...profile,
          apiKey: 'sk-language-browser-fixture-not-a-real-token',
        })),
      },
    });
    window.__languageTestSettings = store.getState().settings;
  });
  await page.select('header select', 'en');
  await page.waitForFunction(
    () =>
      document.documentElement.lang === 'en' &&
      document.body.innerText.includes('Gallery'),
  );
  assert.deepEqual(
    await page.evaluate(() => ({
      tasks:
        window.__languageTestTasks ===
        window.__languageTestStore.getState().tasks,
      status: window.__languageTestStore.getState().tasks[0].status,
      prompt: window.__languageTestStore.getState().prompt,
      editor:
        window.__languageTestEditor ===
        document.querySelector('[contenteditable="true"]'),
      credentials:
        window.__languageTestSettings ===
        window.__languageTestStore.getState().settings,
    })),
    {
      tasks: true,
      status: 'running',
      prompt: '保留中文输入 @图1',
      editor: true,
      credentials: true,
    },
  );

  await page.evaluate(async () => {
    const { imageLanguageStore } = window.__languageTestModule;
    const original = Storage.prototype.setItem;
    Storage.prototype.setItem = function (key, value) {
      if (key === 'site-language-pending-account')
        throw new Error('test quota');
      return original.call(this, key, value);
    };
    try {
      await imageLanguageStore.change('en');
    } finally {
      Storage.prototype.setItem = original;
    }
  });
  await page.waitForSelector('header [role="alert"] button');
  await page.click('header [role="alert"] button');
  await page.waitForFunction(() => {
    const { imageLanguageStore } = window.__languageTestModule;
    return !imageLanguageStore.getSnapshot().storageError;
  });
  assert.equal(
    await page.evaluate(
      () =>
        JSON.parse(localStorage.getItem('site-language-pending-account'))
          .language,
    ),
    'en',
  );

  const anotherPage = await browser.newPage();
  await anotherPage.goto(url, { waitUntil: 'domcontentloaded' });
  await anotherPage.waitForFunction(
    () => document.documentElement.lang === 'en',
  );
  await anotherPage.select('header select', 'zh-TW');
  await page.waitForFunction(() => document.documentElement.lang === 'zh-TW', {
    polling: 50,
  });
  assert.equal(
    await page.evaluate(() => window.__languageTestStore.getState().prompt),
    '保留中文输入 @图1',
  );
  await page.setViewport({ width: 390, height: 844 });
  assert.equal(
    await page.evaluate(() => document.body.scrollWidth <= innerWidth),
    true,
  );

  const languages = await page.evaluate(() => {
    const { IMAGE_LANGUAGES } = window.__languageTestModule;
    return IMAGE_LANGUAGES.map(([language]) => language);
  });
  await page.bringToFront();
  for (const language of languages) {
    await page.select('header select', language);
    await page
      .waitForFunction(
        (expected) => {
          const { imageLanguageStore } = window.__languageTestModule;
          const snapshot = imageLanguageStore.getSnapshot();
          return (
            snapshot.language === expected &&
            !snapshot.pending &&
            !snapshot.error &&
            document.documentElement.lang === expected
          );
        },
        { polling: 50 },
        language,
      )
      .catch(async (error) => {
        const state = await page.evaluate(() => ({
          language:
            window.__languageTestModule.imageLanguageStore.getSnapshot(),
          html: document.documentElement.lang,
          selected: document.querySelector('header select')?.value,
        }));
        throw new Error(
          `Language ${language} did not settle: ${JSON.stringify(state)}`,
          { cause: error },
        );
      });
    assert.deepEqual(
      await page.evaluate(() => ({
        tasks:
          window.__languageTestTasks ===
          window.__languageTestStore.getState().tasks,
        editor:
          window.__languageTestEditor ===
          document.querySelector('[contenteditable="true"]'),
        credentials:
          window.__languageTestSettings ===
          window.__languageTestStore.getState().settings,
        prompt: window.__languageTestStore.getState().prompt,
        fits: document.body.scrollWidth <= innerWidth,
      })),
      {
        tasks: true,
        editor: true,
        credentials: true,
        prompt: '保留中文输入 @图1',
        fits: true,
      },
      `Language switching must preserve live state and fit the viewport: ${language}`,
    );
  }
  await page.select('header select', 'zh-TW');
  await page.waitForFunction(() => document.documentElement.lang === 'zh-TW');

  await page.evaluate(async () => {
    const { putTask } = await import('/src/lib/db.ts');
    await putTask({ ...window.__languageTestTasks[0], status: 'done' });
  });
  await page.reload({ waitUntil: 'domcontentloaded' });
  await page.waitForFunction(() => document.documentElement.lang === 'zh-TW');
  await page.waitForFunction(async () => {
    const { useStore } = await import('/src/store.ts');
    return useStore
      .getState()
      .tasks.some(
        (task) =>
          task.id === 'language-browser-task' &&
          task.prompt === '用户原始提示词 @图1',
      );
  });
  assert.deepEqual(errors, []);
  console.log(
    'Image language browser checks passed: all 17 languages, live state and credentials, editor identity, storage retry, cross-tab, refresh, persisted task, 390px layout.',
  );
} finally {
  await browser.close();
}
