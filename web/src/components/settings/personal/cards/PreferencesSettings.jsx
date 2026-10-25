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

import React from 'react';
import { Button, Card, Select, Typography, Avatar } from '@douyinfe/semi-ui';
import { Languages } from 'lucide-react';
import { SITE_LANGUAGES } from '../../../../helpers/siteLanguage';
import { useSiteLanguage } from '../../../../hooks/common/useSiteLanguage';

const languageOptions = SITE_LANGUAGES.map(([value, label]) => ({
  value,
  label,
}));

const PreferencesSettings = ({ t }) => {
  const {
    language,
    pending,
    error,
    accountSaving,
    accountError,
    storageError,
    changeLanguage,
  } = useSiteLanguage();
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

  return (
    <Card className='!rounded-2xl shadow-sm border-0'>
      <div className='flex items-center mb-4'>
        <Avatar size='small' color='violet' className='mr-3 shadow-md'>
          <Languages size={16} />
        </Avatar>
        <div>
          <Typography.Text className='text-lg font-medium'>
            {t('偏好设置')}
          </Typography.Text>
          <div className='text-xs text-gray-600 dark:text-gray-400'>
            {t('界面语言和其他个人偏好')}
          </div>
        </div>
      </div>
      <Card className='!rounded-xl border dark:border-gray-700'>
        <div className='flex flex-col sm:flex-row items-start sm:items-center sm:justify-between gap-4'>
          <div className='flex items-start w-full sm:w-auto'>
            <div className='w-12 h-12 rounded-full bg-violet-50 dark:bg-violet-900/30 flex items-center justify-center mr-4 flex-shrink-0'>
              <Languages
                size={20}
                className='text-violet-600 dark:text-violet-400'
              />
            </div>
            <div>
              <Typography.Title heading={6} className='mb-1'>
                {t('语言偏好')}
              </Typography.Title>
              <Typography.Text type='tertiary' className='text-sm'>
                {t('选择界面语言；加载完成后在本站生效，登录后会保存到账户。')}
              </Typography.Text>
            </div>
          </div>
          <Select
            aria-label={t('语言偏好')}
            value={language}
            onChange={changeLanguage}
            style={{ width: 180 }}
            loading={Boolean(pending || accountSaving)}
            optionList={languageOptions}
          />
        </div>
        {status && (
          <div className='mt-4 text-xs' role={hasError ? 'alert' : 'status'}>
            <Typography.Text type={hasError ? 'danger' : 'tertiary'}>
              {status}
            </Typography.Text>
            {(accountError || storageError) &&
              !pending &&
              !accountSaving &&
              !error && (
                <Button
                  type='tertiary'
                  theme='borderless'
                  size='small'
                  onClick={() => changeLanguage(language)}
                >
                  {t('重试')}
                </Button>
              )}
          </div>
        )}
      </Card>
    </Card>
  );
};

export default PreferencesSettings;
