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

import React, { useLayoutEffect, useState } from 'react';

function getScrollContainer(page) {
  for (
    let parent = page.parentElement;
    parent && parent !== document.body;
    parent = parent.parentElement
  ) {
    if (
      /auto|scroll/.test(getComputedStyle(parent).overflowY) &&
      parent.scrollHeight > parent.clientHeight + 1
    ) {
      return parent;
    }
  }
  return document.scrollingElement;
}

export default function ModelSectionNav({ sections, pageRef, t }) {
  const [activeId, setActiveId] = useState(sections[0]?.[0]);
  const sectionKey = sections.map(([id]) => id).join(',');

  useLayoutEffect(() => {
    const page = pageRef.current;
    if (!page) return;
    const elements = sectionKey
      .split(',')
      .map((id) => page.querySelector(`#${id}`))
      .filter(Boolean);
    let frame = 0;

    const update = () => {
      frame = 0;
      const visible = elements
        .map((element) => ({ element, rect: element.getBoundingClientRect() }))
        .filter(({ rect }) => rect.height > 0);
      if (!visible.length) return;

      const container = getScrollContainer(page);
      const isDocument = container === document.scrollingElement;
      const containerTop = isDocument
        ? 0
        : container.getBoundingClientRect().top + container.clientTop;
      const viewportTop = Math.max(0, containerTop);
      const viewportBottom = Math.min(
        window.innerHeight,
        containerTop + container.clientHeight,
      );
      const offset =
        parseFloat(getComputedStyle(visible[0].element).scrollMarginTop) || 0;
      const readingTop = viewportTop + offset + 1;
      let current = visible[0].element.id;
      for (const { element, rect } of visible) {
        if (rect.top > readingTop) break;
        current = element.id;
      }

      // Short final sections cannot always reach the reading line.
      const last = visible[visible.length - 1];
      if (
        container.scrollHeight > container.clientHeight + 1 &&
        container.scrollTop + container.clientHeight >=
          container.scrollHeight - 2 &&
        last.rect.top < viewportBottom
      ) {
        current = last.element.id;
      }
      setActiveId((previous) => (previous === current ? previous : current));
    };
    const schedule = () => {
      if (!frame) frame = requestAnimationFrame(update);
    };
    // Capture also observes scrolls in the desktop application's inner workspace.
    window.addEventListener('scroll', schedule, {
      passive: true,
      capture: true,
    });
    window.addEventListener('resize', schedule);
    const observer =
      typeof ResizeObserver === 'function'
        ? new ResizeObserver(schedule)
        : null;
    observer?.observe(page);
    elements.forEach((element) => observer?.observe(element));
    update();

    return () => {
      window.removeEventListener('scroll', schedule, true);
      window.removeEventListener('resize', schedule);
      observer?.disconnect();
      cancelAnimationFrame(frame);
    };
  }, [pageRef, sectionKey]);

  return (
    <nav className='model-detail-sections' aria-label={t('模型详情')}>
      {sections.map(([id, label]) => (
        <button
          key={id}
          type='button'
          aria-controls={id}
          aria-current={activeId === id ? 'location' : undefined}
          onClick={() =>
            pageRef.current
              ?.querySelector(`#${id}`)
              ?.scrollIntoView({ block: 'start' })
          }
        >
          {t(label)}
        </button>
      ))}
    </nav>
  );
}
