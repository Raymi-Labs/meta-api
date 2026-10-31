import React, { useId, useLayoutEffect, useRef, useState } from 'react';
import { ChevronDown, ChevronUp } from 'lucide-react';
import {
  filterValues,
  toggleFilterValue,
} from '../../../../helpers/modelCatalogFacets';

export default function CatalogFacetRow({
  title,
  items,
  value,
  onChange,
  loading,
  t,
  total,
  dimension,
}) {
  const selected = filterValues(value);
  const [expanded, setExpanded] = useState(false);
  const [overflow, setOverflow] = useState(false);
  const [clipped, setClipped] = useState([]);
  const ref = useRef(null),
    id = useId();
  useLayoutEffect(() => {
    const el = ref.current;
    const measure = () => {
      setOverflow(el.scrollHeight > 76);
      const top = el.getBoundingClientRect().top;
      const hidden = [...el.children]
        .map((button, index) =>
          button.getBoundingClientRect().bottom - top > 76 ? index : -1,
        )
        .filter((index) => index >= 0);
      setClipped((previous) =>
        JSON.stringify(previous) === JSON.stringify(hidden) ? previous : hidden,
      );
    };
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    measure();
    return () => observer.disconnect();
  }, [items]);
  return (
    <div className='catalog-facet-row' data-facet={dimension}>
      <span className='catalog-facet-title' id={id}>
        {title}
      </span>
      <div
        className={`catalog-facet-options${expanded ? ' is-expanded' : ''}`}
        ref={ref}
        role='group'
        aria-labelledby={id}
      >
        <button
          type='button'
          aria-pressed={!selected.length}
          disabled={loading}
          onClick={() => onChange([])}
        >
          {t('全部')}
          <small>{total}</small>
        </button>
        {items.map((item, index) => (
          <button
            type='button'
            key={item.value}
            aria-pressed={selected.includes(item.value)}
            tabIndex={!expanded && clipped.includes(index + 1) ? -1 : undefined}
            aria-hidden={
              !expanded && clipped.includes(index + 1) ? true : undefined
            }
            disabled={
              loading || (!item.count && !selected.includes(item.value))
            }
            onClick={() => onChange(toggleFilterValue(value, item.value))}
          >
            {item.icon && <span aria-hidden='true'>{item.icon}</span>}
            {item.label}
            <small>{item.count}</small>
          </button>
        ))}
      </div>
      {(overflow || expanded) && (
        <button
          type='button'
          className='catalog-facet-expand'
          aria-expanded={expanded}
          aria-label={`${title} · ${t(expanded ? '收起' : '展开')}`}
          onClick={() => setExpanded(!expanded)}
        >
          {t(expanded ? '收起' : '展开')}
          {expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
        </button>
      )}
    </div>
  );
}
