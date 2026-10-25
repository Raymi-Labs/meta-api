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
import { Check, ChevronDown, Globe, LoaderCircle } from 'lucide-react';
import { useLocation } from 'react-router-dom';
import { PUBLIC_LANGUAGES } from '../../helpers/publicLanguage';
import { usePublicTranslation } from '../../helpers/publicLocale';
import { useSiteLanguage } from '../../hooks/common/useSiteLanguage';
import './PublicLanguageSelector.css';

export default function PublicLanguageSelector({ variant = 'icon' }) {
  const { t, language, pending, error, setLanguage } = usePublicTranslation();
  const { accountSaving, accountError, storageError } = useSiteLanguage();
  const [open, setOpen] = useState(false);
  const root = useRef(null);
  const trigger = useRef(null);
  const items = useRef([]);
  const menuId = useId();
  const statusId = useId();
  const languageLabel =
    PUBLIC_LANGUAGES.find(([code]) => code === language)?.[1] || language;
  const showLabel = variant === 'label';
  const status = pending
    ? t('正在加载')
    : error
      ? t('语言加载失败，请重试')
      : accountSaving
        ? t('正在保存账户语言偏好…')
        : accountError
          ? t('语言已在本机切换，但账户偏好保存失败。')
          : storageError
            ? t('语言已切换，但浏览器无法保存偏好，刷新后可能恢复原语言。')
            : '';
  const hasError = Boolean(error || accountError || storageError);
  const location = useLocation();
  useEffect(() => setOpen(false), [location.pathname]);
  useEffect(() => {
    if (!open) return;
    items.current[
      PUBLIC_LANGUAGES.findIndex(([code]) => code === language)
    ]?.focus();
    const dismiss = (event) => {
      if (!root.current?.contains(event.target)) setOpen(false);
    };
    document.addEventListener('pointerdown', dismiss);
    return () => document.removeEventListener('pointerdown', dismiss);
  }, [open, language]);
  const close = () => {
    setOpen(false);
    trigger.current?.focus();
  };
  const onKeyDown = (event) => {
    const index = items.current.indexOf(document.activeElement);
    let next;
    if (event.key === 'ArrowDown') next = (index + 1) % PUBLIC_LANGUAGES.length;
    if (event.key === 'ArrowUp')
      next = (index - 1 + PUBLIC_LANGUAGES.length) % PUBLIC_LANGUAGES.length;
    if (event.key === 'Home') next = 0;
    if (event.key === 'End') next = PUBLIC_LANGUAGES.length - 1;
    if (event.key === 'Escape') {
      event.stopPropagation();
      event.preventDefault();
      close();
    } else if (next !== undefined) {
      event.preventDefault();
      items.current[next]?.focus();
    }
  };
  return (
    <div
      className='public-language'
      ref={root}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false);
      }}
    >
      <button
        type='button'
        ref={trigger}
        className={`public-icon-button public-language-trigger${showLabel ? ' public-language-trigger--label' : ''}`}
        aria-label={
          showLabel ? `${t('切换语言')}：${languageLabel}` : t('切换语言')
        }
        aria-haspopup='menu'
        aria-expanded={open}
        aria-controls={menuId}
        aria-describedby={status ? statusId : undefined}
        title={status || undefined}
        data-language-error={hasError || undefined}
        onPointerDown={(event) => {
          if (open) event.preventDefault();
        }}
        onClick={() => setOpen(!open)}
        onKeyDown={(event) => {
          if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
            event.preventDefault();
            setOpen(true);
          }
        }}
      >
        {pending || accountSaving ? (
          <LoaderCircle
            size={18}
            className='public-language-loading'
            aria-hidden='true'
          />
        ) : (
          <Globe size={18} aria-hidden='true' />
        )}
        {showLabel && (
          <>
            <span lang={language}>{languageLabel}</span>
            <ChevronDown size={14} aria-hidden='true' />
          </>
        )}
      </button>
      <span
        id={statusId}
        className='public-language-announcement'
        role={hasError ? 'alert' : 'status'}
      >
        {status}
      </span>
      {open && (
        <div className='public-language-popover'>
          <div
            id={menuId}
            role='menu'
            tabIndex={-1}
            aria-label={t('切换语言')}
            className='public-language-menu'
            onKeyDown={onKeyDown}
            onBlur={(event) => {
              if (!event.currentTarget.contains(event.relatedTarget))
                setOpen(false);
            }}
          >
            {PUBLIC_LANGUAGES.map(([code, label], index) => (
              <button
                key={code}
                ref={(node) => {
                  items.current[index] = node;
                }}
                type='button'
                role='menuitemradio'
                aria-checked={language === code}
                lang={code}
                className='public-language-option'
                tabIndex={-1}
                onClick={async () => {
                  if (await setLanguage(code)) {
                    const menu = root.current?.querySelector('[role=menu]');
                    const restoreFocus = menu?.contains(document.activeElement);
                    setOpen(false);
                    if (restoreFocus) trigger.current?.focus();
                  }
                }}
              >
                <span>{label}</span>
                {pending === code ? (
                  <LoaderCircle
                    size={16}
                    className='public-language-loading'
                    aria-label={t('正在加载')}
                  />
                ) : language === code ? (
                  <Check size={16} aria-hidden='true' />
                ) : null}
              </button>
            ))}
          </div>
          {status && (
            <div
              className={
                hasError ? 'public-language-error' : 'public-language-status'
              }
            >
              <p>{status}</p>
              {accountError && <p>{t('重新选择当前语言可重试保存。')}</p>}
              {storageError && accountError && (
                <p>
                  {t(
                    '语言已切换，但浏览器无法保存偏好，刷新后可能恢复原语言。',
                  )}
                </p>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
