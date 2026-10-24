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
  const [sampleX, sampleY] = options.samplePoint ?? [716, 261];
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
    focused: options.focused ?? true,
    hasFocus() {
      return this.focused;
    },
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
    draws: [],
    transforms: [],
    clearCount: 0,
    sampleCount: 0,
    setTransform(...transform) {
      this.transforms.push(transform);
    },
    clearRect() {
      this.clearCount++;
    },
    // A tiny island with one chosen grid point keeps the renderer observable
    // without depending on the production map's coastline geometry.
    isPointInPath(_path, x, y) {
      this.sampleCount++;
      return x === sampleX && y === sampleY;
    },
    beginPath() {},
    arc(...arc) {
      this.arcs.push(arc);
      this.draws.push({
        x: arc[0],
        y: arc[1],
        radius: arc[2],
        alpha: this.globalAlpha,
      });
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
  let restored = false;
  const restore = () => {
    if (restored) return;
    unmount();
    for (const [name, descriptor] of previousGlobals) {
      if (descriptor) Object.defineProperty(globalThis, name, descriptor);
      else delete globalThis[name];
    }
    restored = true;
  };
  t.after(restore);
  cleanup = attachWorldMapMotion({ root, svg, canvas, hero });
  if (!options.deferIntersection) {
    observers.intersection?.fire([
      { isIntersecting: options.intersecting ?? true },
    ]);
  }

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
    restore,
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
    focus(focused) {
      document.focused = focused;
      window.emit(focused ? 'focus' : 'blur');
    },
    tick(milliseconds = 34) {
      time += milliseconds;
      const callbacks = Array.from(queue.values());
      queue.clear();
      for (const callback of callbacks) callback(time);
    },
  };
}

function capture(t, { options, frameCount = 90, beforeFrame } = {}) {
  const env = fixture(t, options);
  const frames = [];
  for (let index = 0; index < frameCount; index++) {
    beforeFrame?.(env, index);
    env.tick();
    frames.push({ ...env.context.draws.at(-1) });
  }
  env.restore();
  return frames;
}

test('initial animation waits for intersection and focus before sampling', (t) => {
  const env = fixture(t, { deferIntersection: true, focused: false });
  env.move();
  assert.equal(env.queue.size, 0);
  assert.equal(env.context.sampleCount, 0);
  env.observers.intersection.fire([{ isIntersecting: true }]);
  assert.equal(env.queue.size, 0, 'visible but unfocused must remain static');
  env.focus(true);
  assert.equal(env.queue.size, 1);
  env.tick();
  assert.equal(env.animating, true);
});

test('motion-disabled or unsupported states keep the SVG static without sampling', async (t) => {
  for (const [name, options] of [
    ['reduced motion', { reducedMotion: true }],
    ['hidden page', { hidden: true }],
    ['unfocused page', { focused: false }],
    ['offscreen map', { intersecting: false }],
    ['zero width', { rect: { width: 0 } }],
    ['zero height', { rect: { height: 0 } }],
    ['missing canvas', { noContext: true }],
    ['missing Path2D', { noPath2D: true }],
  ]) {
    await t.test(name, (t) => {
      const env = fixture(t, options);
      env.move();
      assert.equal(env.queue.size, 0);
      assert.equal(env.context.sampleCount, 0);
      assert.equal(env.animating, false);
    });
  }
});

test('a visible focused map keeps changing without any pointer events', (t) => {
  const env = fixture(t);
  env.tick();
  const first = env.context.draws.at(-1);
  for (let index = 0; index < 90; index++) {
    env.tick();
    assert.equal(env.queue.size, 1);
    assert.equal(env.animating, true);
  }
  assert.notDeepEqual(env.context.draws.at(-1), first);
  assert.equal(env.maximumQueued, 1);
});

test('ambient color visibly deepens and lightens without flashing between frames', (t) => {
  const frames = capture(t, {
    options: { samplePoint: [1185, 261] },
    frameCount: 900,
  });
  const alphas = frames.map((frame) => frame.alpha);
  const contrast = Math.max(...alphas) - Math.min(...alphas);
  assert.ok(
    contrast > 0.4,
    'ambient color must change noticeably without hover',
  );
  let darkening = 0;
  let lightening = 0;
  for (let index = 1; index < alphas.length; index++) {
    const change = alphas[index] - alphas[index - 1];
    assert.ok(Math.abs(change) < 0.025, 'adjacent frames must not flash');
    if (change > 0.0001) darkening++;
    if (change < -0.0001) lightening++;
  }
  assert.ok(darkening > 100, 'the map must gradually deepen in color');
  assert.ok(lightening > 100, 'the map must gradually lighten again');
});

test('ambient shading retains the soft center and edge around the darker map area', (t) => {
  const alphasAt = (samplePoint) =>
    capture(t, { options: { samplePoint }, frameCount: 900 }).map(
      (frame) => frame.alpha,
    );
  const center = alphasAt([716, 261]);
  const outer = alphasAt([1185, 261]);
  const edge = alphasAt([1437, 261]);
  const average = (values) =>
    values.reduce((sum, value) => sum + value, 0) / values.length;
  assert.ok(
    Math.max(...center) < 0.25,
    'the title area must retain its center fade',
  );
  assert.ok(average(outer) > average(center) * 3);
  assert.ok(average(edge) < average(outer) * 0.4);
  assert.ok(
    Math.max(...center) - Math.min(...center) > 0.07,
    'even faint center dots should still change shade',
  );
});

test('ambient and hovered alpha remain finite and within canvas bounds across the map', (t) => {
  for (const samplePoint of [
    [716, 261],
    [1185, 261],
    [1437, 261],
    [716, 93],
    [716, 429],
  ]) {
    const baseline = capture(t, {
      options: { samplePoint },
      frameCount: 600,
    });
    const hovered = capture(t, {
      options: { samplePoint },
      frameCount: 600,
      beforeFrame: (env) => env.move(...samplePoint),
    });
    for (let index = 0; index < baseline.length; index++) {
      for (const alpha of [baseline[index].alpha, hovered[index].alpha]) {
        assert.ok(Number.isFinite(alpha) && alpha >= 0 && alpha <= 1);
      }
      const added = hovered[index].alpha - baseline[index].alpha;
      assert.ok(added >= -1e-10 && added <= 0.065 + 1e-10);
    }
  }
});

test('coarse pointers, touch and pen retain ambient motion without adding hover', async (t) => {
  const baseline = capture(t);
  for (const [name, options, pointerType] of [
    ['coarse pointer', { finePointer: false }, 'mouse'],
    ['touch', {}, 'touch'],
    ['pen', {}, 'pen'],
  ]) {
    await t.test(name, (t) => {
      const frames = capture(t, {
        options,
        beforeFrame: (env) => env.move(716, 261, pointerType),
      });
      assert.deepEqual(frames, baseline);
      assert.notDeepEqual(frames[0], frames.at(-1));
    });
  }
});

test('rapid pointer movement and repeated observer callbacks keep one RAF chain', (t) => {
  const env = fixture(t);
  const sampled = env.context.sampleCount;
  assert.ok(sampled > 0);
  for (let index = 0; index < 1000; index++) env.move(700 + (index % 50), 261);
  assert.equal(env.queue.size, 1);
  for (let frame = 0; frame < 40; frame++) {
    env.tick(8); // Exercise frames skipped by the rendering rate limit.
    for (let move = 0; move < 10; move++) env.move(710 + move, 261);
    env.observers.intersection.fire([{ isIntersecting: true }]);
    env.window.emit('focus');
    assert.equal(env.queue.size, 1);
  }
  assert.equal(env.maximumQueued, 1);
  assert.equal(env.context.sampleCount, sampled);
  assert.ok(env.context.draws.length > 0);
  assert.equal(env.animating, true);
});

test('mouse effects stay subtle compared with the same ambient animation phase', (t) => {
  const frameCount = 360;
  const baseline = capture(t, { frameCount });
  const offsets = [0, 1, 8, 20, 40, 70, 110, 124, 150, -40, -70, 0];
  const hover = capture(t, {
    frameCount,
    beforeFrame: (env, index) =>
      env.move(716 + offsets[Math.floor(index / 30)], 261),
  });
  const maxima = { radius: 0, displacement: 0, alpha: 0 };
  for (let index = 0; index < frameCount; index++) {
    const difference = {
      radius: hover[index].radius - baseline[index].radius,
      displacement: Math.hypot(
        hover[index].x - baseline[index].x,
        hover[index].y - baseline[index].y,
      ),
      alpha: hover[index].alpha - baseline[index].alpha,
    };
    assert.ok(difference.radius >= -1e-10 && difference.radius <= 0.2 + 1e-10);
    assert.ok(difference.displacement <= 3 + 1e-10);
    assert.ok(difference.alpha >= -1e-10 && difference.alpha <= 0.065 + 1e-10);
    for (const key of Object.keys(maxima)) {
      maxima[key] = Math.max(maxima[key], difference[key]);
    }
  }
  assert.ok(maxima.radius > 0.1, 'hover must visibly affect dot size');
  assert.ok(
    maxima.displacement > 0.1,
    'nearby dots must respond to the pointer',
  );
  assert.ok(maxima.alpha > 0.03, 'hover must softly brighten nearby dots');
});

test('pointer leave, cancel and scroll return to ambient motion without stopping RAF', async (t) => {
  const baseline = capture(t, { frameCount: 130 });
  const exits = {
    pointerleave: (env) => env.hero.emit('pointerleave'),
    pointercancel: (env) => env.hero.emit('pointercancel'),
    scroll: (env) => env.window.emit('scroll'),
  };
  for (const [name, exit] of Object.entries(exits)) {
    await t.test(name, (t) => {
      const env = fixture(t);
      env.move();
      for (let index = 0; index < 30; index++) env.tick();
      assert.ok(env.context.draws.at(-1).radius > baseline[29].radius);
      exit(env);
      assert.equal(env.queue.size, 1);
      for (let index = 30; index < 130; index++) env.tick();
      const settled = env.context.draws.at(-1);
      for (const key of ['x', 'y', 'radius', 'alpha']) {
        assert.ok(Math.abs(settled[key] - baseline[129][key]) < 1e-8);
      }
      assert.equal(env.animating, true);
      assert.equal(env.queue.size, 1);
      env.tick();
      assert.notDeepEqual(env.context.draws.at(-1), settled);
    });
  }
});

test('blur, hidden, offscreen and reduced motion pause and automatically resume', async (t) => {
  const cases = {
    focus: {
      pause: (env) => env.focus(false),
      resume: (env) => env.focus(true),
    },
    visibility: {
      pause: (env) => {
        env.document.hidden = true;
        env.document.emit('visibilitychange');
      },
      resume: (env) => {
        env.document.hidden = false;
        env.document.emit('visibilitychange');
      },
    },
    intersection: {
      pause: (env) =>
        env.observers.intersection.fire([{ isIntersecting: false }]),
      resume: (env) =>
        env.observers.intersection.fire([{ isIntersecting: true }]),
    },
    reducedMotion: {
      pause: (env) => {
        env.media.reduced.matches = true;
        env.media.reduced.emit('change');
      },
      resume: (env) => {
        env.media.reduced.matches = false;
        env.media.reduced.emit('change');
      },
    },
  };
  for (const [name, { pause, resume }] of Object.entries(cases)) {
    await t.test(name, (t) => {
      const env = fixture(t);
      env.tick();
      assert.equal(env.animating, true);
      pause(env);
      env.move();
      assert.equal(env.queue.size, 0);
      assert.equal(env.animating, false);
      const rendered = env.context.draws.length;
      env.tick(10000);
      assert.equal(env.context.draws.length, rendered);
      resume(env);
      assert.equal(
        env.queue.size,
        1,
        'resume must not require a pointer event',
      );
      env.tick();
      assert.equal(env.animating, true);
      assert.equal(env.maximumQueued, 1);
    });
  }
});

test('losing fine-pointer capability removes hover while ambient motion continues', (t) => {
  const baseline = capture(t, { frameCount: 32 });
  const env = fixture(t);
  env.move();
  for (let index = 0; index < 30; index++) env.tick();
  assert.ok(env.context.draws.at(-1).radius > baseline[29].radius);
  env.media.fine.matches = false;
  env.media.fine.emit('change');
  env.move();
  env.tick();
  assert.deepEqual(env.context.draws.at(-1), baseline[30]);
  assert.equal(env.queue.size, 1);
  env.media.fine.matches = true;
  env.media.fine.emit('change');
  env.move();
  env.tick();
  assert.ok(env.context.draws.at(-1).radius > baseline[31].radius);
});

test('letterboxing aligns hover with the SVG and caps canvas resolution', (t) => {
  const baseline = capture(t, { frameCount: 2 });
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
  env.tick();
  assert.deepEqual(env.context.draws.at(-1), baseline[0]);
  env.move();
  env.tick();
  const hovered = env.context.draws.at(-1);
  assert.ok(Math.abs(hovered.x - baseline[1].x) < 1e-8);
  assert.ok(Math.abs(hovered.y - baseline[1].y) < 1e-8);
  assert.ok(
    hovered.radius > baseline[1].radius,
    'the centered dot should respond',
  );
  const transform = env.context.transforms.at(-1);
  assert.ok(Math.abs(transform[0] - 2000 / 1440) < 1e-8);
  assert.equal(transform[0], transform[3]);
  assert.equal(transform[4], 0);
  assert.ok(Math.abs(transform[5] - (800 - (520 * 2000) / 1440) / 2) < 1e-8);
});

test('resizing preserves one RAF chain with updated canvas dimensions', (t) => {
  const env = fixture(t);
  env.tick();
  const sampled = env.context.sampleCount;
  env.rect.width = 1000;
  env.rect.height = 400;
  env.observers.size.fire();
  assert.equal(env.queue.size, 1);
  assert.equal(env.canvas.width, 1000);
  assert.equal(env.canvas.height, 400);
  env.tick();
  assert.equal(env.animating, true);
  assert.equal(env.context.sampleCount, sampled);
  assert.equal(env.maximumQueued, 1);
});

test('normal desktops keep DPR detail while 4K and ultrawide canvases obey the pixel budget', async (t) => {
  for (const [name, width, height, capped] of [
    ['desktop', 1920, (1920 * 520) / 1440, false],
    ['4K', 3840, (3840 * 520) / 1440, true],
    ['ultrawide', 5120, (5120 * 520) / 1440, true],
  ]) {
    await t.test(name, (t) => {
      const env = fixture(t, {
        rect: { width, height },
        devicePixelRatio: 3,
      });
      const pixels = env.canvas.width * env.canvas.height;
      assert.ok(pixels <= 6_000_000);
      if (capped) {
        assert.ok(
          pixels > 5_980_000,
          'use the budget without unnecessarily blurring the map',
        );
        assert.ok(env.canvas.width < width * 2);
      } else {
        assert.equal(env.canvas.width, Math.round(width * 2));
        assert.equal(env.canvas.height, Math.round(height * 2));
      }
      env.tick();
      assert.equal(env.animating, true);
      assert.equal(env.maximumQueued, 1);
    });
  }
});

test('ultrawide-to-mobile resizing preserves SVG alignment and pointer response', (t) => {
  const env = fixture(t, {
    rect: { left: 27, top: 80, width: 5120, height: (5120 * 520) / 1440 },
    devicePixelRatio: 3,
  });
  const sampled = env.context.sampleCount;
  for (const [width, height] of [
    [5120, (5120 * 520) / 1440],
    [1000, 400],
    [390, 400],
    [3840, (3840 * 520) / 1440],
  ]) {
    env.rect.width = width;
    env.rect.height = height;
    env.observers.size.fire();
    env.move();
    env.tick();
    const draw = env.context.draws.at(-1);
    const [scaleX, , , scaleY, offsetX, offsetY] =
      env.context.transforms.at(-1);
    const svgScale = Math.min(width / 1440, height / 520);
    const svgX = (width - 1440 * svgScale) / 2 + draw.x * svgScale;
    const svgY = (height - 520 * svgScale) / 2 + draw.y * svgScale;
    const canvasX = ((draw.x * scaleX + offsetX) * width) / env.canvas.width;
    const canvasY = ((draw.y * scaleY + offsetY) * height) / env.canvas.height;
    assert.ok(
      Math.abs(canvasX - svgX) < 1e-8,
      'canvas and SVG must agree on the screen x coordinate',
    );
    assert.ok(
      Math.abs(canvasY - svgY) < 1e-8,
      'canvas and SVG must agree on the screen y coordinate',
    );
    assert.ok(Math.abs((scaleX * width) / env.canvas.width - svgScale) < 1e-8);
    assert.ok(
      Math.abs((scaleY * height) / env.canvas.height - svgScale) < 1e-8,
    );
    assert.ok(env.canvas.width * env.canvas.height <= 6_000_000);
    assert.equal(env.context.sampleCount, sampled);
    assert.equal(env.queue.size, 1);
  }
  assert.equal(env.maximumQueued, 1);
  env.restore();

  // Resolution affects raster sharpness, not the logical hover location or
  // its dot displacement, size, opacity and ambient animation phase.
  const pointerFrames = (options) =>
    capture(t, {
      options,
      frameCount: 20,
      beforeFrame: (map) => map.move(750, 280),
    });
  const normal = pointerFrames({});
  for (const rect of [
    { left: 27, top: 80, width: 5120, height: (5120 * 520) / 1440 },
    { left: -305, top: 80, width: 1000, height: 400 },
  ]) {
    const resized = pointerFrames({ rect, devicePixelRatio: 3 });
    for (let index = 0; index < normal.length; index++) {
      for (const key of ['x', 'y', 'radius', 'alpha']) {
        assert.ok(Math.abs(resized[index][key] - normal[index][key]) < 1e-8);
      }
    }
  }
});

test('batched intersection entries use the most recent visibility state', (t) => {
  const env = fixture(t);
  env.tick();
  env.observers.intersection.fire([
    { isIntersecting: true },
    { isIntersecting: false },
  ]);
  assert.equal(env.queue.size, 0);
  assert.equal(env.animating, false);
  env.observers.intersection.fire([
    { isIntersecting: false },
    { isIntersecting: true },
  ]);
  assert.equal(env.queue.size, 1);
  env.tick();
  assert.equal(env.animating, true);
  assert.equal(env.maximumQueued, 1);
});

test('unmount removes listeners and late observer callbacks cannot restart animation', (t) => {
  const env = fixture(t);
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
  const rendered = env.context.draws.length;
  const cleared = env.context.clearCount;
  env.move();
  env.focus(true);
  env.window.emit('resize');
  env.window.emit('scroll');
  // An already queued browser callback may run after disconnect().
  env.observers.intersection.callback([{ isIntersecting: true }]);
  env.observers.size.callback();
  env.observers.theme.callback();
  env.tick(1000);
  assert.equal(env.queue.size, 0);
  assert.equal(env.animating, false);
  assert.equal(env.context.draws.length, rendered);
  assert.equal(env.context.clearCount, cleared);
});
