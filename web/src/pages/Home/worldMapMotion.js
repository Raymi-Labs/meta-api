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

const WIDTH = 1440;
const HEIGHT = 520;
const RADIUS = 125;
const FRAME_INTERVAL = 1000 / 30;
const MAX_CANVAS_PIXELS = 6000000;

// The SVG remains the fallback while motion is paused or reduced.
export function attachWorldMapMotion({ root, svg, canvas, hero }) {
  const context = canvas.getContext('2d');
  if (!context || !hero || typeof Path2D === 'undefined') return;

  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');
  const finePointer = window.matchMedia('(hover: hover) and (pointer: fine)');
  let points;
  let frame = 0;
  let previousTime = 0;
  let active = false;
  let visible = false;
  let focused = document.hasFocus();
  let disposed = false;
  let animationTime = 0;
  let strength = 0;
  let pointer = { x: 0, y: 0 };
  let target = pointer;
  let color;
  let transform;

  const permitted = () =>
    !disposed &&
    !reducedMotion.matches &&
    !document.hidden &&
    focused &&
    visible &&
    canvas.width > 0 &&
    canvas.height > 0;

  const reset = () => {
    window.cancelAnimationFrame(frame);
    frame = 0;
    previousTime = 0;
    active = false;
    strength = 0;
    root.removeAttribute('data-animating');
    context.setTransform(1, 0, 0, 1, 0, 0);
    context.clearRect(0, 0, canvas.width, canvas.height);
  };

  const resize = () => {
    if (disposed) return;
    reset();
    const { width, height } = root.getBoundingClientRect();
    let ratio = Math.min(window.devicePixelRatio || 1, 2);
    let pixelWidth = Math.round(width * ratio);
    let pixelHeight = Math.round(height * ratio);
    if (pixelWidth * pixelHeight > MAX_CANVAS_PIXELS) {
      // Keep full-width maps affordable on high-density ultrawide displays.
      ratio = Math.min(ratio, Math.sqrt(MAX_CANVAS_PIXELS / (width * height)));
      pixelWidth = Math.floor(width * ratio);
      pixelHeight = Math.floor(height * ratio);
    }
    canvas.width = pixelWidth;
    canvas.height = pixelHeight;
    // Account for integer backing dimensions so CSS scaling stays aligned
    // with the SVG even when the pixel budget reduces the canvas resolution.
    const ratioX = width > 0 ? pixelWidth / width : 0;
    const ratioY = height > 0 ? pixelHeight / height : 0;
    // Match SVG's default xMidYMid meet, including its letterboxing.
    const scale = Math.min(width / WIDTH, height / HEIGHT);
    transform = [
      ratioX * scale,
      0,
      0,
      ratioY * scale,
      (ratioX * (width - WIDTH * scale)) / 2,
      (ratioY * (height - HEIGHT * scale)) / 2,
    ];
    sync();
  };

  const samplePoints = () => {
    const paths = Array.from(
      svg.querySelectorAll('.yh-map-land path'),
      (path) => new Path2D(path.getAttribute('d')),
    );
    const result = [];
    context.setTransform(1, 0, 0, 1, 0, 0);
    for (let y = 2; y < HEIGHT; y += 7) {
      for (let x = 2; x < WIDTH; x += 7) {
        if (!paths.some((path) => context.isPointInPath(path, x, y))) continue;
        const distance = Math.min(
          1,
          Math.hypot((x - 720) / 720, (y - 260) / 260),
        );
        const opacity =
          distance < 0.65
            ? 0.12 + (distance / 0.65) * 0.6
            : 0.72 - ((distance - 0.65) / 0.35) * 0.57;
        result.push({ x, y, opacity });
      }
    }
    return result;
  };

  const render = (time) => {
    if (disposed) return;
    frame = 0;
    if (!permitted()) return reset();
    const elapsed = previousTime ? time - previousTime : FRAME_INTERVAL;
    if (elapsed < FRAME_INTERVAL - 1) {
      frame = window.requestAnimationFrame(render);
      return;
    }
    previousTime = time;
    const delta = Math.min(elapsed, 64);
    animationTime += delta;
    const easing = 1 - Math.exp(-delta / 150);
    strength += ((active ? 1 : 0) - strength) * easing;
    pointer = {
      x: pointer.x + (target.x - pointer.x) * easing,
      y: pointer.y + (target.y - pointer.y) * easing,
    };

    context.setTransform(1, 0, 0, 1, 0, 0);
    context.clearRect(0, 0, canvas.width, canvas.height);
    context.setTransform(...transform);
    context.fillStyle = color;
    const phase = animationTime / 1000;
    for (const point of points) {
      // Broad, slow waves keep the dots alive without moving the whole map.
      const wave = Math.sin(point.x / 90 + point.y / 120 - phase * 0.85);
      const crossWave = Math.cos(point.y / 80 - point.x / 160 + phase * 0.55);
      // Move broad light/dark bands with the waves, preserving the center fade
      // and average contrast instead of making the whole map darker at once.
      const shade = (wave * 0.8 + crossWave * 0.2 + 1) / 2;
      const ambientX = crossWave * 0.65;
      const ambientY = wave * 1.8 + crossWave * 0.55;
      const dx = point.x - pointer.x;
      const dy = point.y - pointer.y;
      const distance = Math.hypot(dx, dy);
      const falloff = Math.max(0, 1 - distance / RADIUS);
      const influence = falloff * falloff * strength;
      const ripple = Math.sin(distance / 28 - phase * 1.8);
      const displacement =
        influence * (2.4 + ripple * 0.6) * Math.min(1, distance / 40);
      context.globalAlpha = Math.min(
        1,
        point.opacity * (0.4 + shade * 1.08) + influence * 0.065,
      );
      context.beginPath();
      context.arc(
        point.x + ambientX + (dx / (distance || 1)) * displacement,
        point.y + ambientY + (dy / (distance || 1)) * displacement,
        1.25 + wave * 0.16 + influence * 0.2,
        0,
        Math.PI * 2,
      );
      context.fill();
    }
    context.globalAlpha = 1;
    root.setAttribute('data-animating', 'true');
    frame = window.requestAnimationFrame(render);
  };

  const sync = () => {
    if (disposed) return;
    if (!permitted()) return reset();
    if (frame) return;
    if (!points) points = samplePoints();
    color = window.getComputedStyle(root).color;
    frame = window.requestAnimationFrame(render);
  };

  const move = (event) => {
    if (event.pointerType !== 'mouse' || !finePointer.matches || !permitted())
      return;
    const matrix = svg.getScreenCTM();
    if (!matrix) return;
    const position = new DOMPoint(event.clientX, event.clientY).matrixTransform(
      matrix.inverse(),
    );
    if (
      position.x < 0 ||
      position.x > WIDTH ||
      position.y < 0 ||
      position.y > HEIGHT
    ) {
      active = false;
      return;
    }
    target = { x: position.x, y: position.y };
    if (!active) pointer = target;
    active = true;
  };
  const leave = () => {
    active = false;
  };
  const clearHover = () => {
    active = false;
    strength = 0;
  };
  const blur = () => {
    focused = false;
    sync();
  };
  const focus = () => {
    focused = true;
    sync();
  };
  const visibility = () => {
    focused = document.hasFocus();
    sync();
  };
  const preference = () => {
    if (!finePointer.matches) clearHover();
    sync();
  };
  const theme = new MutationObserver(() => {
    if (disposed) return;
    color = window.getComputedStyle(root).color;
  });
  const observer = new IntersectionObserver((entries) => {
    if (disposed) return;
    const entry = entries[entries.length - 1];
    if (!entry) return;
    visible = entry.isIntersecting;
    sync();
  });
  const size = new ResizeObserver(resize);

  resize();
  hero.addEventListener('pointermove', move, { passive: true });
  hero.addEventListener('pointerleave', leave);
  hero.addEventListener('pointercancel', clearHover);
  window.addEventListener('blur', blur);
  window.addEventListener('focus', focus);
  window.addEventListener('resize', resize);
  // Scrolling changes map coordinates beneath a stationary pointer.
  window.addEventListener('scroll', clearHover, { passive: true });
  document.addEventListener('visibilitychange', visibility);
  reducedMotion.addEventListener('change', preference);
  finePointer.addEventListener('change', preference);
  theme.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class'],
  });
  observer.observe(root);
  size.observe(root);
  return () => {
    disposed = true;
    reset();
    hero.removeEventListener('pointermove', move);
    hero.removeEventListener('pointerleave', leave);
    hero.removeEventListener('pointercancel', clearHover);
    window.removeEventListener('blur', blur);
    window.removeEventListener('focus', focus);
    window.removeEventListener('resize', resize);
    window.removeEventListener('scroll', clearHover);
    document.removeEventListener('visibilitychange', visibility);
    reducedMotion.removeEventListener('change', preference);
    finePointer.removeEventListener('change', preference);
    theme.disconnect();
    observer.disconnect();
    size.disconnect();
  };
}
