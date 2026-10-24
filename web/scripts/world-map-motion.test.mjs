import assert from 'node:assert/strict';
import test from 'node:test';
import { attachWorldMapMotion } from '../src/pages/Home/worldMapMotion.js';

class Events {
  listeners = new Map();

  addEventListener(type, listener) {
    if (!this.listeners.has(type)) this.listeners.set(type, new Set());
    this.listeners.get(type).add(listener);
  }

  removeEventListener(type, listener) {
    this.listeners.get(type)?.delete(listener);
  }

  emit(type, properties = {}) {
    for (const listener of this.listeners.get(type) || []) {
      listener({ type, ...properties });
    }
  }

  get listenerCount() {
    return Array.from(this.listeners.values()).reduce(
      (count, listeners) => count + listeners.size,
      0,
    );
  }
}

function fixture(t, options = {}) {
  const rect = {
    left: 0,
    top: 0,
    width: 1440,
    height: 520,
    ...options.rect,
  };
  const root = {
    attributes: new Map(),
    getBoundingClientRect: () => rect,
    setAttribute(name, value) {
      this.attributes.set(name, value);
    },
    removeAttribute(name) {
      this.attributes.delete(name);
    },
  };
  const media = {
    reduced: Object.assign(new Events(), {
      matches: options.reducedMotion ?? false,
    }),
    fine: Object.assign(new Events(), {
      matches: options.finePointer ?? true,
    }),
  };
  const hero = new Events();
  const document = Object.assign(new Events(), {
    hidden: options.hidden ?? false,
    documentElement: {},
  });
  const queue = new Map();
  let nextFrame = 1;
  let time = 0;
  let maximumQueued = 0;
  const window = Object.assign(new Events(), {
    devicePixelRatio: options.devicePixelRatio ?? 1,
    matchMedia: (query) =>
      query.includes('reduced-motion') ? media.reduced : media.fine,
    getComputedStyle: () => ({ color: '#6684bf' }),
    requestAnimationFrame(callback) {
      const id = nextFrame++;
      queue.set(id, callback);
      maximumQueued = Math.max(maximumQueued, queue.size);
      return id;
    },
    cancelAnimationFrame(id) {
      queue.delete(id);
    },
  });
  const context = {
    arcs: [],
    transforms: [],
    clearCount: 0,
    sampleCount: 0,
    setTransform(...transform) {
      this.transforms.push(transform);
    },
    clearRect() {
      this.clearCount++;
    },
    // A tiny island with one grid point keeps the renderer observable without
    // making this test depend on the production map's coastline geometry.
    isPointInPath(_path, x, y) {
      this.sampleCount++;
      return x === 716 && y === 261;
    },
    beginPath() {},
    arc(...arc) {
      this.arcs.push(arc);
    },
    fill() {},
  };
  const canvas = {
    width: 0,
    height: 0,
    getContext: () => (options.noContext ? null : context),
  };
  const matrix = () => {
    const scale = Math.min(rect.width / 1440, rect.height / 520);
    const x = rect.left + (rect.width - 1440 * scale) / 2;
    const y = rect.top + (rect.height - 520 * scale) / 2;
    return { scale, x, y };
  };
  const svg = {
    querySelectorAll: () => [{ getAttribute: () => 'test island' }],
    getScreenCTM: () => ({
      inverse() {
        const { scale, x, y } = matrix();
        return { a: 1 / scale, d: 1 / scale, e: -x / scale, f: -y / scale };
      },
    }),
  };
  const observers = {};
  const observerClass = (name) =>
    class {
      constructor(callback) {
        this.callback = callback;
        this.disconnected = false;
        observers[name] = this;
      }
      observe() {}
      disconnect() {
        this.disconnected = true;
      }
      fire(value) {
        if (!this.disconnected) this.callback(value);
      }
    };
  const globals = {
    window,
    document,
    Path2D: options.noPath2D ? undefined : class {},
    DOMPoint: class {
      constructor(x, y) {
        this.x = x;
        this.y = y;
      }
      matrixTransform({ a, d, e, f }) {
        return { x: this.x * a + e, y: this.y * d + f };
      }
    },
    MutationObserver: observerClass('theme'),
    IntersectionObserver: observerClass('intersection'),
    ResizeObserver: observerClass('size'),
  };
  const previousGlobals = new Map();
  for (const [name, value] of Object.entries(globals)) {
    previousGlobals.set(
      name,
      Object.getOwnPropertyDescriptor(globalThis, name),
    );
    Object.defineProperty(globalThis, name, { configurable: true, value });
  }
  let cleanup;
  const unmount = () => {
    cleanup?.();
    cleanup = undefined;
  };
  t.after(() => {
    unmount();
    for (const [name, descriptor] of previousGlobals) {
      if (descriptor) Object.defineProperty(globalThis, name, descriptor);
      else delete globalThis[name];
    }
  });
  cleanup = attachWorldMapMotion({ root, svg, canvas, hero });

  return {
    root,
    rect,
    hero,
    document,
    window,
    media,
    context,
    canvas,
    observers,
    queue,
    unmount,
    get maximumQueued() {
      return maximumQueued;
    },
    get animating() {
      return root.attributes.has('data-animating');
    },
    move(x = 716, y = 261, pointerType = 'mouse') {
      const { scale, x: left, y: top } = matrix();
      hero.emit('pointermove', {
        pointerType,
        clientX: left + x * scale,
        clientY: top + y * scale,
      });
    },
    tick(milliseconds = 34) {
      time += milliseconds;
      const callbacks = Array.from(queue.values());
      queue.clear();
      for (const callback of callbacks) callback(time);
    },
  };
}

test('unsupported input and reduced motion keep the SVG static without sampling', async (t) => {
  for (const [name, options, pointerType] of [
    ['reduced motion', { reducedMotion: true }, 'mouse'],
    ['coarse pointer', { finePointer: false }, 'mouse'],
    ['hidden page', { hidden: true }, 'mouse'],
    ['touch', {}, 'touch'],
    ['pen', {}, 'pen'],
    ['missing canvas', { noContext: true }, 'mouse'],
    ['missing Path2D', { noPath2D: true }, 'mouse'],
  ]) {
    await t.test(name, (t) => {
      const env = fixture(t, options);
      env.move(716, 261, pointerType);
      assert.equal(env.queue.size, 0);
      assert.equal(env.context.sampleCount, 0);
      assert.equal(env.animating, false);
    });
  }
});

test('rapid pointer movement uses one animation chain and reuses sampled geometry', (t) => {
  const env = fixture(t);
  for (let index = 0; index < 1000; index++) env.move(700 + (index % 50), 261);
  assert.equal(env.queue.size, 1);
  const sampled = env.context.sampleCount;
  assert.ok(sampled > 0);
  for (let frame = 0; frame < 20; frame++) {
    env.tick(8); // Includes frames skipped by the rendering rate limit.
    for (let move = 0; move < 10; move++) env.move(710 + move, 261);
    assert.equal(env.queue.size, 1);
  }
  assert.equal(env.maximumQueued, 1);
  assert.equal(env.context.sampleCount, sampled);
  assert.ok(env.context.arcs.length > 0);
  assert.equal(env.animating, true);
});

test('pointer leave eases back, restores the static SVG and stops requesting frames', (t) => {
  const env = fixture(t);
  env.move();
  for (let frame = 0; frame < 20; frame++) env.tick();
  assert.equal(env.animating, true);
  env.hero.emit('pointerleave');
  env.tick();
  assert.equal(env.animating, true, 'the return should not snap instantly');
  for (let frame = 0; frame < 40 && env.queue.size; frame++) env.tick();
  assert.equal(env.queue.size, 0);
  assert.equal(env.animating, false);
  const rendered = env.context.arcs.length;
  env.tick(1000);
  assert.equal(env.context.arcs.length, rendered);
  env.move();
  env.tick();
  assert.equal(env.animating, true, 'hover can restart after settling');
});

test('visibility and pointer interruptions immediately clear the animation', async (t) => {
  const interruptions = {
    blur: (env) => env.window.emit('blur'),
    scroll: (env) => env.window.emit('scroll'),
    pointercancel: (env) => env.hero.emit('pointercancel'),
    hidden: (env) => {
      env.document.hidden = true;
      env.document.emit('visibilitychange');
    },
    offscreen: (env) =>
      env.observers.intersection.fire([{ isIntersecting: false }]),
  };
  for (const [name, interrupt] of Object.entries(interruptions)) {
    await t.test(name, (t) => {
      const env = fixture(t);
      env.move();
      env.tick();
      assert.equal(env.animating, true);
      const cleared = env.context.clearCount;
      interrupt(env);
      assert.equal(env.queue.size, 0);
      assert.equal(env.animating, false);
      assert.ok(env.context.clearCount > cleared);
    });
  }
});

test('changing motion or input preferences cancels active work and permits later hover', async (t) => {
  for (const name of ['reduced', 'fine']) {
    await t.test(name, (t) => {
      const env = fixture(t);
      env.move();
      env.tick();
      env.media[name].matches = name === 'reduced';
      env.media[name].emit('change');
      env.move();
      assert.equal(env.animating, false);
      assert.equal(env.queue.size, 0);
      env.media[name].matches = name !== 'reduced';
      env.media[name].emit('change');
      assert.equal(
        env.queue.size,
        0,
        'preference change alone must not animate',
      );
      env.move();
      env.tick();
      assert.equal(env.animating, true);
    });
  }
});

test('letterboxed SVG coordinates keep the hover centered and cap canvas resolution', (t) => {
  const env = fixture(t, {
    rect: { left: 150, top: 100, width: 1000, height: 400 },
    devicePixelRatio: 3,
  });
  assert.equal(env.canvas.width, 2000);
  assert.equal(env.canvas.height, 800);
  env.hero.emit('pointermove', {
    pointerType: 'mouse',
    clientX: 650,
    clientY: 110, // Inside the root box, above the letterboxed map itself.
  });
  assert.equal(env.queue.size, 0);
  env.move();
  env.tick();
  const [x, y, radius] = env.context.arcs.at(-1);
  assert.ok(Math.abs(x - 716) < 1e-8);
  assert.ok(Math.abs(y - 261) < 1e-8);
  assert.ok(radius > 1.25, 'the dot under the pointer should respond');
  const transform = env.context.transforms.at(-1);
  assert.ok(Math.abs(transform[0] - 2000 / 1440) < 1e-8);
  assert.equal(transform[0], transform[3]);
  assert.equal(transform[4], 0);
  assert.ok(Math.abs(transform[5] - (800 - (520 * 2000) / 1440) / 2) < 1e-8);
});

test('resizing resets the active canvas and recalculates its backing dimensions', (t) => {
  const env = fixture(t);
  env.move();
  env.tick();
  env.rect.width = 1000;
  env.rect.height = 400;
  env.observers.size.fire();
  assert.equal(env.queue.size, 0);
  assert.equal(env.animating, false);
  assert.equal(env.canvas.width, 1000);
  assert.equal(env.canvas.height, 400);
  env.move();
  env.tick();
  assert.equal(env.animating, true);
});

test('unmount cancels RAF, removes listeners and disconnects every observer', (t) => {
  const env = fixture(t);
  env.move();
  env.tick();
  env.unmount();
  assert.equal(env.queue.size, 0);
  assert.equal(env.animating, false);
  for (const target of [
    env.hero,
    env.window,
    env.document,
    env.media.reduced,
    env.media.fine,
  ]) {
    assert.equal(target.listenerCount, 0);
  }
  for (const observer of Object.values(env.observers)) {
    assert.equal(observer.disconnected, true);
  }
  const cleared = env.context.clearCount;
  env.move();
  env.window.emit('resize');
  env.window.emit('scroll');
  env.observers.size.fire();
  env.tick(1000);
  assert.equal(env.queue.size, 0);
  assert.equal(env.context.clearCount, cleared);
});
