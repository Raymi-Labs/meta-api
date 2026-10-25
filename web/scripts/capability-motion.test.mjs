import assert from 'node:assert/strict';
import test from 'node:test';
import {
  createCapabilityMotion,
  INITIAL_CAPABILITY_MOTION,
} from '../src/pages/Home/capabilityMotion.js';

class Events {
  listeners = new Map();

  addEventListener(type, listener) {
    if (!this.listeners.has(type)) this.listeners.set(type, new Set());
    this.listeners.get(type).add(listener);
  }

  removeEventListener(type, listener) {
    this.listeners.get(type)?.delete(listener);
  }

  emit(type) {
    for (const listener of this.listeners.get(type) || []) listener();
  }

  get listenerCount() {
    return Array.from(this.listeners.values()).reduce(
      (count, listeners) => count + listeners.size,
      0,
    );
  }
}

function fixture(t, options = {}) {
  const changes = [];
  const timers = new Map();
  let time = 0;
  let nextTimer = 0;
  let maximumTimers = 0;
  let observer;
  const media = Object.assign(new Events(), {
    matches: options.reduced ?? false,
  });
  const doc = Object.assign(new Events(), {
    hidden: options.hidden ?? false,
    focused: options.focused ?? true,
    hasFocus() {
      return this.focused;
    },
  });
  const win = Object.assign(new Events(), {
    matchMedia: () => media,
    setTimeout(callback, delay) {
      const id = nextTimer++;
      timers.set(id, { callback, due: time + delay });
      maximumTimers = Math.max(maximumTimers, timers.size);
      return id;
    },
    clearTimeout(id) {
      timers.delete(id);
    },
    IntersectionObserver: class {
      constructor(callback) {
        this.callback = callback;
        this.disconnected = false;
        observer = this;
      }
      observe(element) {
        this.element = element;
      }
      disconnect() {
        this.disconnected = true;
      }
      fire(...states) {
        if (this.disconnected) return;
        this.callback(states.map((isIntersecting) => ({ isIntersecting })));
      }
    },
  });
  if (options.noObserver) delete win.IntersectionObserver;
  if (options.noMedia) delete win.matchMedia;
  if (options.noFocus) delete doc.hasFocus;
  if (options.noTimers) {
    delete win.setTimeout;
    delete win.clearTimeout;
  }
  if (options.legacyMedia) {
    media.addListener = (listener) =>
      Events.prototype.addEventListener.call(media, 'change', listener);
    media.removeListener = (listener) =>
      Events.prototype.removeEventListener.call(media, 'change', listener);
    media.addEventListener = undefined;
    media.removeEventListener = undefined;
  }
  const element = options.noElement ? null : {};
  const cleanup = createCapabilityMotion({
    element,
    onChange: (state) => changes.push(state),
    window: win,
    document: doc,
    sceneCount: options.sceneCount,
  });
  t.after(cleanup);
  if (!options.deferIntersection) observer?.fire(options.intersecting ?? true);
  return {
    win,
    doc,
    media,
    observer,
    timers,
    changes,
    cleanup,
    get state() {
      return changes.at(-1);
    },
    get maximumTimers() {
      return maximumTimers;
    },
    focus(focused) {
      doc.focused = focused;
      win.emit(focused ? 'focus' : 'blur');
    },
    hidden(hidden) {
      doc.hidden = hidden;
      doc.emit('visibilitychange');
    },
    reduced(matches) {
      media.matches = matches;
      media.emit('change');
    },
    advance(milliseconds) {
      const target = time + milliseconds;
      let fired = 0;
      while (timers.size) {
        const [id, timer] = Array.from(timers.entries()).sort(
          ([, first], [, second]) => first.due - second.due,
        )[0];
        if (timer.due > target) break;
        time = timer.due;
        timers.delete(id);
        timer.callback();
        assert.ok(++fired < 1000, 'timer loop must make progress');
      }
      time = target;
    },
  };
}

test('waits for intersection and focus before starting the first phase', (t) => {
  const env = fixture(t, { deferIntersection: true, focused: false });
  assert.deepEqual(env.state, INITIAL_CAPABILITY_MOTION);
  assert.equal(env.timers.size, 0);
  env.observer.fire(true);
  assert.equal(env.timers.size, 0);
  env.focus(true);
  assert.deepEqual(env.state, { scene: 0, phase: 'primary', active: true });
  assert.equal(env.timers.size, 1);
});

test('primary, failure and fallback stay in one scene until the full round ends', (t) => {
  const env = fixture(t);
  env.advance(3199);
  assert.deepEqual(env.state, { scene: 0, phase: 'primary', active: true });
  env.advance(1);
  assert.deepEqual(env.state, { scene: 0, phase: 'unavailable', active: true });
  env.advance(1099);
  assert.equal(env.state.phase, 'unavailable');
  env.advance(1);
  assert.deepEqual(env.state, { scene: 0, phase: 'fallback', active: true });
  env.advance(2599);
  assert.equal(env.state.phase, 'fallback');
  env.advance(1);
  assert.deepEqual(env.state, { scene: 1, phase: 'primary', active: true });
  env.advance(6900);
  assert.equal(env.state.scene, 2);
  env.advance(6900);
  assert.deepEqual(env.state, { scene: 0, phase: 'primary', active: true });
  assert.equal(env.maximumTimers, 1);
});

test('scene count wraps without changing the phase sequence', async (t) => {
  for (const sceneCount of [1, 2, 5]) {
    await t.test(`${sceneCount} scenes`, (t) => {
      const env = fixture(t, { sceneCount });
      env.advance(6900 * sceneCount);
      assert.deepEqual(env.state, { scene: 0, phase: 'primary', active: true });
      for (let index = 1; index < env.changes.length - 1; index += 3) {
        assert.equal(env.changes[index].scene, env.changes[index + 1].scene);
        assert.equal(env.changes[index].scene, env.changes[index + 2].scene);
      }
    });
  }
});

test('hidden, blurred and offscreen cards pause and resume the current phase', async (t) => {
  const states = {
    hidden: [(env) => env.hidden(true), (env) => env.hidden(false)],
    blurred: [(env) => env.focus(false), (env) => env.focus(true)],
    offscreen: [
      (env) => env.observer.fire(false),
      (env) => env.observer.fire(true),
    ],
  };
  for (const [name, [pause, resume]] of Object.entries(states)) {
    await t.test(name, (t) => {
      const env = fixture(t);
      env.advance(3600);
      pause(env);
      assert.deepEqual(env.state, {
        scene: 0,
        phase: 'unavailable',
        active: false,
      });
      assert.equal(env.timers.size, 0);
      const changes = env.changes.length;
      env.advance(50000);
      assert.equal(env.changes.length, changes);
      resume(env);
      assert.equal(env.state.active, true);
      env.advance(1099);
      assert.equal(
        env.state.phase,
        'unavailable',
        'resume grants a complete phase duration',
      );
      env.advance(1);
      assert.deepEqual(env.state, {
        scene: 0,
        phase: 'fallback',
        active: true,
      });
      assert.equal(env.maximumTimers, 1);
    });
  }
});

test('every visibility prerequisite must recover before the timer resumes', (t) => {
  const env = fixture(t);
  env.hidden(true);
  env.focus(false);
  env.observer.fire(false);
  env.hidden(false);
  assert.equal(env.timers.size, 0);
  env.focus(true);
  assert.equal(env.timers.size, 0);
  env.observer.fire(true);
  assert.equal(env.timers.size, 1);
  assert.equal(env.state.active, true);
});

test('runtime reduced motion resets scene and phase and restarts cleanly when disabled', (t) => {
  const env = fixture(t);
  env.advance(11200);
  assert.deepEqual(env.state, { scene: 1, phase: 'fallback', active: true });
  env.reduced(true);
  assert.deepEqual(env.state, INITIAL_CAPABILITY_MOTION);
  assert.equal(env.timers.size, 0);
  env.advance(50000);
  env.observer.fire(true);
  env.focus(true);
  assert.deepEqual(env.state, INITIAL_CAPABILITY_MOTION);
  assert.equal(env.timers.size, 0);
  env.reduced(false);
  assert.deepEqual(env.state, { scene: 0, phase: 'primary', active: true });
  env.advance(3199);
  assert.equal(env.state.phase, 'primary');
  env.advance(1);
  assert.equal(env.state.phase, 'unavailable');
});

test('initial reduced motion remains a static primary scene', (t) => {
  const env = fixture(t, { reduced: true });
  env.advance(50000);
  assert.deepEqual(env.state, INITIAL_CAPABILITY_MOTION);
  assert.equal(env.timers.size, 0);
});

test('repeated lifecycle notifications keep one timer and preserve its deadline', (t) => {
  const env = fixture(t);
  env.advance(1000);
  for (let index = 0; index < 100; index++) {
    env.observer.fire(true);
    env.focus(true);
    env.doc.emit('visibilitychange');
    env.media.emit('change');
  }
  assert.equal(env.timers.size, 1);
  assert.equal(env.maximumTimers, 1);
  env.advance(2200);
  assert.equal(env.state.phase, 'unavailable');
});

test('batched intersection records use the last visibility state', (t) => {
  const env = fixture(t);
  env.observer.fire(true, false);
  assert.equal(env.state.active, false);
  assert.equal(env.timers.size, 0);
  env.observer.fire(false, true);
  assert.equal(env.state.active, true);
  assert.equal(env.timers.size, 1);
});

test('a canceled callback cannot advance a resumed phase or create another timer', (t) => {
  const env = fixture(t);
  const stale = Array.from(env.timers.values())[0].callback;
  env.focus(false);
  env.focus(true);
  const changes = env.changes.length;
  stale();
  assert.equal(env.changes.length, changes);
  assert.equal(env.timers.size, 1);
  env.advance(3200);
  assert.equal(env.state.phase, 'unavailable');
  assert.equal(env.maximumTimers, 1);
});

test('missing optional browser APIs use safe fallbacks', async (t) => {
  for (const [name, options, runs] of [
    ['no intersection observer', { noObserver: true }, true],
    ['no media query', { noMedia: true }, true],
    ['no focus API', { noFocus: true }, true],
    ['no timer API', { noTimers: true }, false],
    ['missing element', { noElement: true }, false],
  ]) {
    await t.test(name, (t) => {
      const env = fixture(t, options);
      assert.equal(env.state.active, runs);
      assert.equal(env.timers.size, runs ? 1 : 0);
      env.advance(3200);
      assert.equal(env.state.phase, runs ? 'unavailable' : 'primary');
    });
  }
});

test('legacy media listeners still reset motion and are cleaned up', (t) => {
  const env = fixture(t, { legacyMedia: true });
  env.advance(3200);
  env.reduced(true);
  assert.deepEqual(env.state, INITIAL_CAPABILITY_MOTION);
  env.cleanup();
  assert.equal(env.media.listenerCount, 0);
});

test('cleanup cancels everything and late callbacks cannot revive the state machine', (t) => {
  const env = fixture(t);
  const staleTimer = Array.from(env.timers.values())[0].callback;
  const staleFocus = Array.from(env.win.listeners.get('focus'))[0];
  env.cleanup();
  assert.equal(env.timers.size, 0);
  assert.equal(env.observer.disconnected, true);
  for (const target of [env.win, env.doc, env.media]) {
    assert.equal(target.listenerCount, 0);
  }
  const changes = env.changes.length;
  env.observer.callback([{ isIntersecting: true }]);
  staleTimer();
  staleFocus();
  env.media.emit('change');
  env.advance(50000);
  env.cleanup();
  assert.equal(env.timers.size, 0);
  assert.equal(env.changes.length, changes);
});

test('initial state emitted to a consumer does not share the exported object', (t) => {
  const env = fixture(t, { deferIntersection: true });
  env.state.scene = 100;
  assert.deepEqual(INITIAL_CAPABILITY_MOTION, {
    scene: 0,
    phase: 'primary',
    active: false,
  });
  env.observer.fire(true);
  assert.deepEqual(env.state, { scene: 0, phase: 'primary', active: true });
});
