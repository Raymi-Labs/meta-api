import assert from 'node:assert/strict';
import { test } from 'node:test';
import { startVisibleRefresh } from '../src/helpers/visibleRefresh.js';

function fixture(refresh) {
  const doc = new EventTarget();
  doc.visibilityState = 'visible';
  const win = new EventTarget();
  let tick;
  let cleared = false;
  win.setInterval = (fn, ms) => {
    assert.equal(ms, 15000);
    tick = fn;
    return 1;
  };
  win.clearInterval = (id) => {
    assert.equal(id, 1);
    cleared = true;
  };
  const stop = startVisibleRefresh(refresh, { document: doc, window: win });
  return { doc, win, stop, tick: () => tick(), cleared: () => cleared };
}

test('polls only visible pages and refreshes on return/focus', async () => {
  let calls = 0;
  const f = fixture(async () => {
    calls++;
  });
  await f.tick();
  assert.equal(calls, 1);
  f.doc.visibilityState = 'hidden';
  await f.tick();
  f.win.dispatchEvent(new Event('focus'));
  assert.equal(calls, 1);
  f.doc.visibilityState = 'visible';
  f.doc.dispatchEvent(new Event('visibilitychange'));
  await Promise.resolve();
  assert.equal(calls, 2);
  f.win.dispatchEvent(new Event('focus'));
  await Promise.resolve();
  assert.equal(calls, 3);
  f.stop();
  assert.equal(f.cleared(), true);
  await f.tick();
  f.win.dispatchEvent(new Event('focus'));
  assert.equal(calls, 3);
});

test('slow or failed refreshes do not overlap or disable later retries', async () => {
  let release;
  let calls = 0;
  const f = fixture(async () => {
    calls++;
    if (calls === 1)
      await new Promise((resolve) => {
        release = resolve;
      });
    else if (calls === 2) throw Error('offline');
  });
  const pending = f.tick();
  await f.tick();
  f.win.dispatchEvent(new Event('focus'));
  assert.equal(calls, 1);
  release();
  await pending;
  await f.tick();
  assert.equal(calls, 2);
  await f.tick();
  assert.equal(calls, 3);
  f.stop();
});
