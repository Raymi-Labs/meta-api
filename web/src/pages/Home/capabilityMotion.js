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

export const INITIAL_CAPABILITY_MOTION = {
  scene: 0,
  phase: 'primary',
  active: false,
};

const PHASE_DURATION = {
  primary: 3200,
  unavailable: 1100,
  fallback: 2600,
};

export function createCapabilityMotion({
  element,
  onChange,
  window: win = typeof window === 'undefined' ? undefined : window,
  document: doc = typeof document === 'undefined' ? undefined : document,
  sceneCount = 3,
}) {
  let state = { ...INITIAL_CAPABILITY_MOTION };
  const emit = () => onChange?.({ ...state });
  emit();
  if (!element || !win || !doc) return () => {};

  const count = Number.isInteger(sceneCount) && sceneCount > 0 ? sceneCount : 3;
  const reducedMotion = win.matchMedia?.('(prefers-reduced-motion: reduce)');
  const Observer = win.IntersectionObserver;
  const hasObserver = typeof Observer === 'function';
  const hasTimers =
    typeof win.setTimeout === 'function' &&
    typeof win.clearTimeout === 'function';
  let visible = !hasObserver;
  let focused = doc.hasFocus?.() ?? true;
  let disposed = false;
  let timer = null;
  let generation = 0;

  const update = (next) => {
    if (
      state.scene === next.scene &&
      state.phase === next.phase &&
      state.active === next.active
    )
      return;
    state = next;
    emit();
  };
  const cancel = () => {
    generation++;
    if (timer !== null) win.clearTimeout(timer);
    timer = null;
  };
  const permitted = () =>
    !disposed &&
    hasTimers &&
    visible &&
    focused &&
    !doc.hidden &&
    !reducedMotion?.matches;

  const schedule = () => {
    if (!permitted() || timer !== null) return;
    const currentGeneration = ++generation;
    timer = win.setTimeout(() => {
      if (disposed || currentGeneration !== generation) return;
      timer = null;
      if (!permitted()) return sync();
      if (state.phase === 'primary') {
        update({ ...state, phase: 'unavailable' });
      } else if (state.phase === 'unavailable') {
        update({ ...state, phase: 'fallback' });
      } else {
        update({
          scene: (state.scene + 1) % count,
          phase: 'primary',
          active: true,
        });
      }
      schedule();
    }, PHASE_DURATION[state.phase]);
  };

  const sync = () => {
    if (disposed) return;
    if (reducedMotion?.matches) {
      cancel();
      update({ ...INITIAL_CAPABILITY_MOTION });
      return;
    }
    const active = permitted();
    if (!active) cancel();
    update({ ...state, active });
    if (active) schedule();
  };
  const focus = () => {
    focused = true;
    sync();
  };
  const blur = () => {
    focused = false;
    sync();
  };
  const visibility = () => {
    focused = doc.hasFocus?.() ?? focused;
    sync();
  };
  const observer = hasObserver
    ? new Observer((entries) => {
        if (disposed || !entries.length) return;
        visible = entries[entries.length - 1].isIntersecting;
        sync();
      })
    : undefined;

  win.addEventListener?.('focus', focus);
  win.addEventListener?.('blur', blur);
  doc.addEventListener?.('visibilitychange', visibility);
  if (reducedMotion?.addEventListener) {
    reducedMotion.addEventListener('change', sync);
  } else {
    reducedMotion?.addListener?.(sync);
  }
  observer?.observe(element);
  sync();

  return () => {
    if (disposed) return;
    disposed = true;
    cancel();
    observer?.disconnect();
    win.removeEventListener?.('focus', focus);
    win.removeEventListener?.('blur', blur);
    doc.removeEventListener?.('visibilitychange', visibility);
    if (reducedMotion?.removeEventListener) {
      reducedMotion.removeEventListener('change', sync);
    } else {
      reducedMotion?.removeListener?.(sync);
    }
  };
}
