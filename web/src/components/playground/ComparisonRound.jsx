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
import React, { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { comparisonReplyState } from '../../helpers/playgroundParallel';

// Presentation state never owns a request: switching tabs must not resend it.
export default function ComparisonRound({
  replies,
  layout,
  isMobile,
  renderReply,
  preview = false,
}) {
  const { t } = useTranslation();
  const [activeId, setActiveId] = useState(null);
  const [expandedId, setExpandedId] = useState(null);
  const root = useRef(null);
  const focusReturn = useRef(null);
  const active = replies.find((m) => m.id === activeId)?.id || replies[0]?.id;
  const expanded = replies.some((m) => m.id === expandedId) ? expandedId : null;
  const restore = () => {
    setExpandedId(null);
    focusReturn.current?.focus();
  };
  useEffect(() => {
    if (!expanded) return;
    const onKey = (event) => {
      if (event.key === 'Escape' && !event.defaultPrevented) {
        event.preventDefault();
        setExpandedId(null);
        focusReturn.current?.focus();
      }
    };
    const el = root.current;
    el?.addEventListener('keydown', onKey);
    return () => el?.removeEventListener('keydown', onKey);
  }, [expanded]);
  return (
    <div
      ref={root}
      className={`pg-answer-comparison ${expanded ? 'is-expanded' : ''}`}
    >
      {isMobile && !expanded && (
        <div
          className='pg-answer-tabs'
          role='tablist'
          aria-label={t('选择模型回答')}
        >
          {replies.map((reply, index) => (
            <button
              key={reply.id}
              role='tab'
              id={`answer-tab-${reply.id}`}
              aria-controls={`answer-panel-${reply.id}`}
              aria-selected={active === reply.id}
              tabIndex={active === reply.id ? 0 : -1}
              onClick={() => setActiveId(reply.id)}
              onKeyDown={(e) => {
                const offsets = { ArrowRight: 1, ArrowLeft: -1 };
                const next =
                  e.key === 'Home'
                    ? 0
                    : e.key === 'End'
                      ? replies.length - 1
                      : e.key in offsets
                        ? (index + offsets[e.key] + replies.length) %
                          replies.length
                        : null;
                if (next === null) return;
                e.preventDefault();
                setActiveId(replies[next].id);
                e.currentTarget.parentElement.children[next]?.focus();
              }}
            >
              <strong>{reply.parallelModel}</strong>
              <span>
                {preview ? t('待发送') : t(comparisonReplyState(reply))}
              </span>
            </button>
          ))}
        </div>
      )}
      <div
        className={`pg-round-answers is-${layout} ${preview ? 'is-preview' : ''} ${expanded ? 'is-expanded' : ''}`}
        style={{ '--answer-columns': replies.length }}
      >
        {replies.map((reply) => (
          <div
            key={reply.id}
            className='pg-answer-slot'
            hidden={
              expanded ? reply.id !== expanded : isMobile && reply.id !== active
            }
            id={`answer-panel-${reply.id}`}
            role={isMobile && !expanded ? 'tabpanel' : undefined}
            aria-labelledby={
              isMobile && !expanded ? `answer-tab-${reply.id}` : undefined
            }
          >
            {renderReply(reply, {
              preview,
              expanded: expanded === reply.id,
              toggleExpand: (e) => {
                if (expanded) restore();
                else {
                  focusReturn.current = e.currentTarget;
                  setExpandedId(reply.id);
                }
              },
            })}
          </div>
        ))}
      </div>
    </div>
  );
}
