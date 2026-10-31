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

import React, { useState } from 'react';
import { Button, Select, TextArea, Tag, withField } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  SITE_LANGUAGES,
  normalizeLocale,
} from '../../../../helpers/localeNormalize';

function ModelDescriptionEditor({ value, onChange, automatic = {} }) {
  const { t, i18n } = useTranslation();
  const [locale, setLocale] = useState(() => normalizeLocale(i18n.language));
  const overrides = value || {};
  const manual = Boolean(overrides[locale]?.trim());
  const text = overrides[locale] ?? automatic[locale] ?? '';
  const translatedCount = SITE_LANGUAGES.filter(([code]) =>
    (overrides[code] || automatic[code])?.trim(),
  ).length;
  return (
    <div className='flex flex-col gap-3 min-w-0'>
      <div className='flex items-center gap-2 flex-wrap'>
        <Select
          aria-label={t('介绍语言')}
          value={locale}
          onChange={setLocale}
          style={{ minWidth: 170 }}
          optionList={SITE_LANGUAGES.map(([value, label]) => ({
            value,
            label,
          }))}
        />
        <Tag>
          {t(manual ? '人工配置' : automatic[locale] ? '自动翻译' : '等待翻译')}
        </Tag>
        <span className='text-xs text-[var(--semi-color-text-2)]'>
          {translatedCount} / {SITE_LANGUAGES.length}
        </span>
        <Button
          htmlType='button'
          size='small'
          disabled={!Object.hasOwn(overrides, locale)}
          onClick={() => {
            const next = { ...overrides };
            delete next[locale];
            onChange(next);
          }}
        >
          {t('恢复自动')}
        </Button>
      </div>
      <TextArea
        aria-label={t('多语言介绍')}
        value={text}
        rows={5}
        maxLength={16000}
        onChange={(text) => onChange({ ...overrides, [locale]: text })}
      />
      <p className='text-xs text-[var(--semi-color-text-2)]'>
        {t('译文跟随站点语言；人工修改优先，清空后恢复自动翻译。')}
      </p>
    </div>
  );
}
export default withField(ModelDescriptionEditor);
