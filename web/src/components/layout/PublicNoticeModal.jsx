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

import React, { useEffect, useRef } from 'react';
import { Bell, X } from 'lucide-react';
import { usePublicTranslation } from '../../helpers/publicLocale';
import { getAnnouncementKey } from '../../helpers/noticeReadState';
import RichContent from '../common/RichContent';

const PublicNoticeModal = ({
  visible,
  content,
  announcements = [],
  loading = false,
  error = false,
  onRetry,
  onClose,
}) => {
  const { t, language } = usePublicTranslation();
  const dialogRef = useRef(null);
  const closeButtonRef = useRef(null);
  const closeRef = useRef(null);
  const close = () =>
    onClose({
      announcementKeys: announcements.map(getAnnouncementKey),
      content: content || '',
    });
  closeRef.current = close;

  useEffect(() => {
    if (!visible) return;
    const previousFocus = document.activeElement;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    closeButtonRef.current?.focus();
    const handleKeyDown = (event) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        closeRef.current();
      } else if (event.key === 'Tab') {
        const elements = Array.from(
          dialogRef.current?.querySelectorAll(
            'button:not([disabled]), a[href], [tabindex="0"]',
          ) || [],
        ).filter((element) => element.getClientRects().length > 0);
        const first = elements[0];
        const last = elements[elements.length - 1];
        if (
          event.shiftKey &&
          (document.activeElement === first ||
            !dialogRef.current?.contains(document.activeElement))
        ) {
          event.preventDefault();
          last?.focus();
        } else if (
          !event.shiftKey &&
          (document.activeElement === last ||
            !dialogRef.current?.contains(document.activeElement))
        ) {
          event.preventDefault();
          first?.focus();
        }
      }
    };
    document.addEventListener('keydown', handleKeyDown, true);
    return () => {
      document.removeEventListener('keydown', handleKeyDown, true);
      document.body.style.overflow = previousOverflow;
      if (previousFocus?.isConnected) previousFocus.focus();
    };
  }, [visible]);

  if (!visible) return null;

  const formatDate = (value) => {
    const date = new Date(value);
    if (!Number.isFinite(date.getTime())) return '';
    return date.toLocaleString(language, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    });
  };

  return (
    <div
      className='public-notice-backdrop'
      role='presentation'
      onClick={(event) => {
        if (event.target === event.currentTarget) close();
      }}
    >
      <section
        ref={dialogRef}
        id='public-notice-dialog'
        className='public-notice-modal'
        role='dialog'
        aria-modal='true'
        aria-labelledby='public-notice-title'
      >
        <header className='public-notice-header'>
          <h2 id='public-notice-title'>
            <Bell size={19} aria-hidden='true' />
            {t('系统公告')}
          </h2>
          <button
            ref={closeButtonRef}
            type='button'
            className='public-notice-close'
            aria-label={t('关闭公告')}
            onClick={close}
          >
            <X size={18} />
          </button>
        </header>
        <div className='public-notice-content' aria-busy={loading}>
          {loading && (
            <p role='status' className='public-notice-state'>
              {t('正在加载')}
            </p>
          )}
          {error && (
            <div className='public-notice-error' role='alert'>
              <p>{t('公告加载失败，请稍后重试。')}</p>
              <button
                type='button'
                className='public-notice-ghost'
                onClick={onRetry}
                disabled={loading}
              >
                {t('重新加载')}
              </button>
            </div>
          )}
          {content && (
            <RichContent
              className='public-notice-primary-content'
              content={content}
              breaks
            />
          )}
          {announcements.length > 0 && (
            <ol className='public-announcement-list'>
              {announcements.map((item, index) => (
                <li
                  key={`${getAnnouncementKey(item)}-${index}`}
                  className='public-announcement-item'
                >
                  {item.publishDate && (
                    <time dateTime={item.publishDate}>
                      {formatDate(item.publishDate)}
                    </time>
                  )}
                  <RichContent content={item.content} breaks />
                  {item.extra && (
                    <RichContent
                      className='public-announcement-extra'
                      content={item.extra}
                      breaks
                    />
                  )}
                </li>
              ))}
            </ol>
          )}
          {!loading && !error && !content && announcements.length === 0 && (
            <div className='public-notice-empty'>
              <Bell size={30} aria-hidden='true' />
              <p>{t('暂无系统公告')}</p>
            </div>
          )}
        </div>
        <footer className='public-notice-footer'>
          <button
            type='button'
            className='public-notice-primary'
            onClick={close}
          >
            {t('关闭公告')}
          </button>
        </footer>
      </section>
    </div>
  );
};

export default PublicNoticeModal;
