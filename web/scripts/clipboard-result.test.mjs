import assert from 'node:assert/strict';
import test from 'node:test';
import { copy } from '../src/helpers/clipboard.js';
const originalNavigator = Object.getOwnPropertyDescriptor(
  globalThis,
  'navigator',
);
const originalWindow = Object.getOwnPropertyDescriptor(globalThis, 'window');
function fixture(write, command) {
  let removed = 0,
    focused = 0,
    selected = 0,
    appended = 0;
  const node = {
    style: {},
    setAttribute() {},
    select() {
      selected++;
    },
    remove() {
      removed++;
    },
  };
  Object.defineProperty(globalThis, 'navigator', {
    configurable: true,
    value: { clipboard: { writeText: write } },
  });
  Object.defineProperty(globalThis, 'window', {
    configurable: true,
    value: {
      document: {
        activeElement: {
          isConnected: true,
          focus() {
            focused++;
          },
        },
        createElement() {
          return node;
        },
        body: {
          appendChild() {
            appended++;
          },
        },
        execCommand: command,
      },
    },
  });
  return () => ({ removed, focused, selected, appended, text: node.value });
}
function restore() {
  for (const [key, original] of [
    ['navigator', originalNavigator],
    ['window', originalWindow],
  ]) {
    if (original) Object.defineProperty(globalThis, key, original);
    else delete globalThis[key];
  }
}
test('native clipboard receives the entire long value', async () => {
  try {
    let got;
    fixture(
      async (text) => {
        got = text;
      },
      () => {
        throw Error('unexpected');
      },
    );
    const value = 'code\n'.repeat(2000);
    assert.equal(await copy(value), true);
    assert.equal(got, value);
  } finally {
    restore();
  }
});
test('fallback false or exception reports failure and always removes its node', async () => {
  try {
    for (const command of [
      () => false,
      () => {
        throw Error('blocked');
      },
    ]) {
      const read = fixture(async () => {
        throw Error('denied');
      }, command);
      assert.equal(await copy('content'), false);
      assert.equal(read().removed, 1);
      assert.equal(read().focused, 1);
    }
  } finally {
    restore();
  }
});
test('successful legacy fallback copies exact value and restores focus', async () => {
  try {
    const read = fixture(
      async () => {
        throw Error('denied');
      },
      () => true,
    );
    assert.equal(await copy('full\ntext'), true);
    assert.deepEqual(read(), {
      removed: 1,
      focused: 1,
      selected: 1,
      appended: 1,
      text: 'full\ntext',
    });
  } finally {
    restore();
  }
});
