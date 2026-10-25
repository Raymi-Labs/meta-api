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

import React, { useId } from 'react';
import { Button, Dropdown } from '@douyinfe/semi-ui';
import { Globe, LoaderCircle } from 'lucide-react';
import { SITE_LANGUAGES } from '../../../helpers/siteLanguage';
import { useSiteLanguage } from '../../../hooks/common/useSiteLanguage';

const LanguageSelector = ({ currentLang, onLanguageChange, t }) => {
  const { pending, error, accountSaving, accountError, storageError } =
    useSiteLanguage();
  const statusId = useId();
  const status = pending
    ? t('正在加载语言…')
    : error
      ? t('语言加载失败，请重试')
      : accountSaving
        ? t('正在保存账户语言偏好…')
        : [
            accountError && t('语言已在本机切换，但账户偏好保存失败。'),
            storageError &&
              t('语言已切换，但浏览器无法保存偏好，刷新后可能恢复原语言。'),
          ]
            .filter(Boolean)
            .join(' ');
  const hasError = Boolean(error || accountError || storageError);
  const canRetry =
    (accountError || storageError) && !pending && !accountSaving && !error;

  return (
    <>
      <Dropdown
        position='bottomRight'
        render={
          <Dropdown.Menu
            className='!bg-semi-color-bg-overlay !border-semi-color-border !shadow-lg !rounded-lg dark:!bg-gray-700 dark:!border-gray-600'
            style={{ maxHeight: 'min(70vh, 480px)', overflowY: 'auto' }}
          >
            {status && (
              <div
                className={`px-3 py-2 text-xs ${hasError ? 'text-red-600 dark:text-red-400' : 'text-semi-color-text-2'}`}
                style={{ maxWidth: 260 }}
              >
                {status}
                {canRetry && (
                  <Button
                    type='tertiary'
                    theme='borderless'
                    size='small'
                    onClick={() => onLanguageChange(currentLang)}
                  >
                    {t('重试')}
                  </Button>
                )}
              </div>
            )}
            {SITE_LANGUAGES.map(([code, label]) => (
              <Dropdown.Item
                key={code}
                onClick={() => onLanguageChange(code)}
                className={`!px-3 !py-1.5 !text-sm !text-semi-color-text-0 dark:!text-gray-200 ${currentLang === code ? '!bg-semi-color-primary-light-default dark:!bg-blue-600 !font-semibold' : 'hover:!bg-semi-color-fill-1 dark:hover:!bg-gray-600'}`}
              >
                <span lang={code}>{label}</span>
              </Dropdown.Item>
            ))}
          </Dropdown.Menu>
        }
      >
        <Button
          icon={
            pending || accountSaving ? (
              <LoaderCircle size={18} className='animate-spin' />
            ) : (
              <Globe
                size={18}
                aria-hidden='true'
                className={hasError ? 'text-red-500' : ''}
              />
            )
          }
          aria-label={t('选择语言')}
          aria-busy={Boolean(pending || accountSaving)}
          aria-describedby={status ? statusId : undefined}
          title={status || t('选择语言')}
          theme='borderless'
          type='tertiary'
          className='public-icon-button'
        />
      </Dropdown>
      <span
        id={statusId}
        className='sr-only'
        role={hasError ? 'alert' : 'status'}
        aria-atomic='true'
      >
        {status}
      </span>
    </>
  );
};

export default LanguageSelector;
