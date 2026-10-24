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
const RADIUS = 180;
const FRAME_INTERVAL = 1000 / 30;

// Keep the original SVG as the default, including on touch and reduced motion.
export function attachWorldMapMotion({ root, svg, canvas, hero }) {
  const context = canvas.getContext('2d');
  if (!context || !hero || typeof Path2D === 'undefined') return;

  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');
  const finePointer = window.matchMedia('(hover: hover) and (pointer: fine)');
  let points;
  let frame = 0;
  let previousTime = 0;
  let active = false;
  let visible = true;
  let strength = 0;
  let pointer = { x: 0, y: 0 };
  let target = pointer;
  let color;
  let transform;

  const permitted = () =>
    !reducedMotion.matches &&
    finePointer.matches &&
    !document.hidden &&
    visible;

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
    reset();
    const { width, height } = root.getBoundingClientRect();
    const ratio = Math.min(window.devicePixelRatio || 1, 2);
    canvas.width = Math.round(width * ratio);
    canvas.height = Math.round(height * ratio);
    // Match SVG's default xMidYMid meet, including its letterboxing.
    const scale = Math.min(width / WIDTH, height / HEIGHT);
    transform = [
      ratio * scale,
      0,
      0,
      ratio * scale,
      (ratio * (width - WIDTH * scale)) / 2,
      (ratio * (height - HEIGHT * scale)) / 2,
    ];
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
    frame = 0;
    if (!permitted()) return reset();
    const elapsed = previousTime ? time - previousTime : FRAME_INTERVAL;
    if (elapsed < FRAME_INTERVAL - 1) {
      frame = window.requestAnimationFrame(render);
      return;
    }
    previousTime = time;
    const easing = 1 - Math.exp(-Math.min(elapsed, 64) / 100);
    strength += ((active ? 1 : 0) - strength) * easing;
    pointer = {
      x: pointer.x + (target.x - pointer.x) * easing,
      y: pointer.y + (target.y - pointer.y) * easing,
    };
    if (!active && strength < 0.005) return reset();

    context.setTransform(1, 0, 0, 1, 0, 0);
    context.clearRect(0, 0, canvas.width, canvas.height);
    context.setTransform(...transform);
    context.fillStyle = color;
    for (const point of points) {
      const dx = point.x - pointer.x;
      const dy = point.y - pointer.y;
      const distance = Math.hypot(dx, dy);
      const falloff = Math.max(0, 1 - distance / RADIUS);
      const influence = falloff * falloff * strength;
      const ripple = Math.sin(distance / 19 - time / 220);
      const displacement =
        influence * (20 + ripple * 10) * Math.min(1, distance / 45);
      context.globalAlpha = Math.min(1, point.opacity + influence * 0.55);
      context.beginPath();
      context.arc(
        point.x + (dx / (distance || 1)) * displacement,
        point.y + (dy / (distance || 1)) * displacement,
        1.25 + influence * (1.7 + ripple * 0.25),
        0,
        Math.PI * 2,
      );
      context.fill();
    }
    context.globalAlpha = 1;
    root.setAttribute('data-animating', 'true');
    frame = window.requestAnimationFrame(render);
  };

  const move = (event) => {
    if (event.pointerType !== 'mouse' || !permitted()) return;
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
    if (!frame) {
      if (!points) points = samplePoints();
      pointer = target;
      color = window.getComputedStyle(root).color;
      frame = window.requestAnimationFrame(render);
    }
    active = true;
  };
  const leave = () => {
    active = false;
  };
  const visibility = () => {
    if (document.hidden) reset();
  };
  const preference = () => {
    if (!permitted()) reset();
  };
  const theme = new MutationObserver(() => {
    color = window.getComputedStyle(root).color;
  });
  const observer = new IntersectionObserver(([entry]) => {
    visible = entry.isIntersecting;
    if (!visible) reset();
  });
  const size = new ResizeObserver(resize);

  resize();
  hero.addEventListener('pointermove', move, { passive: true });
  hero.addEventListener('pointerleave', leave);
  hero.addEventListener('pointercancel', reset);
  window.addEventListener('blur', reset);
  window.addEventListener('resize', resize);
  // Scrolling changes map coordinates beneath a stationary pointer.
  window.addEventListener('scroll', reset, { passive: true });
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
    reset();
    hero.removeEventListener('pointermove', move);
    hero.removeEventListener('pointerleave', leave);
    hero.removeEventListener('pointercancel', reset);
    window.removeEventListener('blur', reset);
    window.removeEventListener('resize', resize);
    window.removeEventListener('scroll', reset);
    document.removeEventListener('visibilitychange', visibility);
    reducedMotion.removeEventListener('change', preference);
    finePointer.removeEventListener('change', preference);
    theme.disconnect();
    observer.disconnect();
    size.disconnect();
  };
}
