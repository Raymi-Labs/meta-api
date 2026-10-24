import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import vm from 'node:vm';
import {
  applyTheme,
  getStoredTheme,
  getSystemTheme,
  normalizeTheme,
  storeTheme,
  subscribeSystemTheme,
} from '../src/context/Theme/theme.js';

const html = await readFile(new URL('../index.html', import.meta.url), 'utf8');
const extract = (id) =>
  html.match(new RegExp(`<script id="${id}">([\\s\\S]*?)</script>`))[1];
const headScript = extract('theme-init');
const bodyScript = extract('theme-body-init');
const savedWindow = globalThis.window;
const savedDocument = globalThis.document;

function mockDocument() {
  const classes = new Set(['dark', 'unrelated']);
  const attributes = new Map([['theme-mode', 'dark']]);
  return {
    documentElement: {
      classList: {
        toggle: (name, enabled) =>
          enabled ? classes.add(name) : classes.delete(name),
        contains: (name) => classes.has(name),
      },
      style: {},
    },
    body: {
      setAttribute: (name, value) => attributes.set(name, value),
      removeAttribute: (name) => attributes.delete(name),
      getAttribute: (name) => attributes.get(name) ?? null,
    },
  };
}

function assertDOM(document, expected) {
  assert.equal(
    document.documentElement.classList.contains('dark'),
    expected === 'dark',
  );
  assert.equal(document.documentElement.classList.contains('unrelated'), true);
  assert.equal(document.documentElement.style.colorScheme, expected);
  assert.equal(
    document.body.getAttribute('theme-mode'),
    expected === 'dark' ? 'dark' : null,
  );
}

test('bootstrap precedes content and the React entry without changing metadata', () => {
  assert.ok(html.indexOf('id="theme-init"') < html.indexOf('</head>'));
  assert.match(html, /<body>\s*<script id="theme-body-init">/);
  assert.ok(html.indexOf('id="theme-body-init"') < html.indexOf('id="root"'));
  assert.ok(
    html.indexOf('id="theme-init"') < html.indexOf('src="/src/index.jsx"'),
  );
  assert.match(html, /<meta name="generator" content="new-api"/);
});

for (const stored of [
  'dark',
  'light',
  'auto',
  null,
  '',
  'broken',
  'system',
  'DARK',
]) {
  for (const systemDark of [false, true]) {
    test(`inline and React agree: saved=${String(stored)}, systemDark=${systemDark}`, () => {
      const document = mockDocument();
      const body = document.body;
      document.body = null;
      const localStorage = {
        getItem: (key) => {
          assert.equal(key, 'theme-mode');
          return stored;
        },
        setItem: () =>
          assert.fail('bootstrap must not overwrite saved preference'),
      };
      const window = {
        localStorage,
        matchMedia: () => ({ matches: systemDark }),
      };
      const context = vm.createContext({ document, localStorage, window });
      vm.runInContext(headScript, context);
      document.body = body;
      vm.runInContext(bodyScript, context);
      const mode = normalizeTheme(stored);
      const expected = mode === 'auto' ? (systemDark ? 'dark' : 'light') : mode;
      assertDOM(document, expected);
      globalThis.window = window;
      globalThis.document = document;
      assert.equal(getStoredTheme(), mode);
      applyTheme(mode === 'auto' ? getSystemTheme() : mode);
      assertDOM(document, expected);
    });
  }
}

test('storage access throws, media unavailable/throws, and missing DOM are safe', () => {
  const blockedStorage = {
    getItem() {
      throw new Error('denied');
    },
    setItem() {
      throw new Error('quota');
    },
  };
  for (const matchMedia of [
    undefined,
    () => {
      throw new Error('unavailable');
    },
    () => ({ matches: true }),
  ]) {
    const document = mockDocument();
    const window = {
      matchMedia,
      get localStorage() {
        throw new Error('denied');
      },
    };
    const context = vm.createContext({
      document,
      window,
      localStorage: blockedStorage,
    });
    vm.runInContext(headScript, context);
    vm.runInContext(bodyScript, context);
    globalThis.window = window;
    globalThis.document = document;
    assert.equal(getStoredTheme(), 'auto');
    assert.doesNotThrow(() => storeTheme('dark'));
    const expected = getSystemTheme();
    assertDOM(document, expected);
    applyTheme(expected);
    assertDOM(document, expected);
  }
  delete globalThis.window;
  delete globalThis.document;
  assert.equal(getStoredTheme(), 'auto');
  assert.equal(getSystemTheme(), 'light');
  assert.doesNotThrow(() => storeTheme('dark'));
  assert.doesNotThrow(() => applyTheme('dark'));
  assert.equal(
    subscribeSystemTheme(() => {}),
    undefined,
  );
});

test('system subscription synchronizes mount, changes and cleanup, including old browsers', () => {
  for (const modern of [true, false]) {
    let handler;
    let removed;
    const events = [];
    const media = { matches: true };
    if (modern) {
      media.addEventListener = (type, callback) => {
        assert.equal(type, 'change');
        handler = callback;
      };
      media.removeEventListener = (type, callback) => {
        assert.equal(type, 'change');
        removed = callback;
      };
    } else {
      media.addListener = (callback) => {
        handler = callback;
      };
      media.removeListener = (callback) => {
        removed = callback;
      };
    }
    globalThis.window = { matchMedia: () => media };
    const unsubscribe = subscribeSystemTheme((theme) => events.push(theme));
    handler({ matches: false });
    handler({ matches: true });
    unsubscribe();
    assert.deepEqual(events, ['dark', 'light', 'dark']);
    assert.equal(removed, handler);
  }
});

test.after(() => {
  if (savedWindow === undefined) delete globalThis.window;
  else globalThis.window = savedWindow;
  if (savedDocument === undefined) delete globalThis.document;
  else globalThis.document = savedDocument;
});
