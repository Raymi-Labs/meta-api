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

import React, { useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import './composerResize.css';

const MIN_HEIGHT = 72;
const clamp = (height, max) =>
  Math.round(Math.max(MIN_HEIGHT, Math.min(max, height)));

export function useComposerResize() {
  const textareaRef = useRef(null);
  const drag = useRef(null);
  const [height, setHeight] = useState(MIN_HEIGHT);
  const [maxHeight, setMaxHeight] = useState(420);
  const id = useId();
  const bounds = useRef(420);

  useEffect(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;
    const composer = textarea.closest(
      '.pg-composer-wrap, .pg-parallel-composer',
    );
    const panel = textarea.closest('.pg-chat, .pg-parallel');
    const measure = () => {
      if (!panel || panel.clientHeight === 0) return;
      const viewport = window.visualViewport?.height || window.innerHeight;
      const siblings = [...panel.children].filter(
        (el) =>
          el !== composer && !el.matches('.pg-messages, .pg-parallel-messages'),
      );
      const extras = siblings.reduce(
        (sum, el) => sum + el.getBoundingClientRect().height,
        0,
      );
      const chrome =
        composer.getBoundingClientRect().height -
        textarea.getBoundingClientRect().height;
      const max = Math.max(
        MIN_HEIGHT,
        Math.floor(
          Math.min(
            420,
            viewport * 0.45,
            panel.clientHeight - chrome - extras - 120,
          ),
        ),
      );
      bounds.current = max;
      setMaxHeight(max);
      setHeight((current) => clamp(current, max));
    };
    const observer = new ResizeObserver(measure);
    observer.observe(panel);
    observer.observe(composer);
    measure();
    window.addEventListener('resize', measure);
    window.visualViewport?.addEventListener('resize', measure);
    return () => {
      observer.disconnect();
      window.removeEventListener('resize', measure);
      window.visualViewport?.removeEventListener('resize', measure);
      drag.current = null;
    };
  }, []);

  const finish = (event, restore = false) => {
    if (drag.current?.pointerId !== event.pointerId) return;
    const initial = drag.current.height;
    drag.current = null;
    if (restore) setHeight(clamp(initial, bounds.current));
    if (event.currentTarget.hasPointerCapture(event.pointerId))
      event.currentTarget.releasePointerCapture(event.pointerId);
  };
  return {
    textareaProps: {
      ref: textareaRef,
      id,
      style: { height, maxHeight, minHeight: MIN_HEIGHT },
    },
    handleProps: {
      'aria-controls': id,
      'aria-valuemin': MIN_HEIGHT,
      'aria-valuemax': maxHeight,
      'aria-valuenow': height,
      onPointerDown: (event) => {
        if (!event.isPrimary || event.button !== 0 || drag.current) return;
        event.preventDefault();
        event.currentTarget.focus({ preventScroll: true });
        event.currentTarget.setPointerCapture(event.pointerId);
        drag.current = {
          pointerId: event.pointerId,
          y: event.clientY,
          height: textareaRef.current.getBoundingClientRect().height,
        };
      },
      onPointerMove: (event) => {
        const start = drag.current;
        if (!start || start.pointerId !== event.pointerId) return;
        setHeight(
          clamp(start.height + start.y - event.clientY, bounds.current),
        );
      },
      onPointerUp: (event) => finish(event),
      onPointerCancel: (event) => finish(event, true),
      onLostPointerCapture: () => {
        drag.current = null;
      },
      onDoubleClick: () => setHeight(MIN_HEIGHT),
      onKeyDown: (event) => {
        const step = event.shiftKey ? 48 : 16;
        if (!['ArrowUp', 'ArrowDown', 'Home', 'End'].includes(event.key))
          return;
        event.preventDefault();
        setHeight((current) =>
          clamp(
            event.key === 'Home'
              ? MIN_HEIGHT
              : event.key === 'End'
                ? bounds.current
                : current + (event.key === 'ArrowUp' ? step : -step),
            bounds.current,
          ),
        );
      },
    },
  };
}

export default function ComposerResizeHandle(props) {
  const { t } = useTranslation();
  return (
    <div
      className='pg-composer-resize'
      role='separator'
      tabIndex={0}
      aria-orientation='horizontal'
      aria-label={t('调整输入框高度')}
      {...props}
    >
      <span aria-hidden='true' />
    </div>
  );
}
