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
import {
  Button,
  Input,
  InputNumber,
  Select,
  TextArea,
  withField,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  OFFICIAL_PRICE_COLUMNS,
  OFFICIAL_PRICE_UNITS,
} from '../../../../helpers/modelOfficialPricing';

function OfficialPricingEditor({ value, onChange, disabled }) {
  const { t } = useTranslation();
  const pricing = value || {};
  const tiers = pricing.tiers || [];
  const update = (patch) => onChange({ ...pricing, ...patch });
  const updateTier = (index, patch) =>
    update({
      tiers: tiers.map((tier, i) =>
        i === index ? { ...tier, ...patch } : tier,
      ),
    });
  return (
    <div className='official-pricing-editor'>
      <p className='text-xs text-gray-500 mb-3'>
        {t('仅展示，不参与扣费；留空表示未配置，0 表示免费。')}
      </p>
      <div className='flex flex-wrap gap-3 mb-3'>
        <Select
          aria-label={t('币种')}
          value={pricing.currency || 'USD'}
          disabled={disabled}
          onChange={(currency) => update({ currency })}
          optionList={['USD', 'CNY', 'EUR'].map((value) => ({
            value,
            label: value,
          }))}
        />
        <Input
          aria-label={t('价格来源')}
          placeholder={t('价格来源')}
          value={pricing.source_url || ''}
          disabled={disabled}
          onChange={(source_url) => update({ source_url })}
          maxLength={2048}
          style={{ flex: '1 1 220px' }}
        />
      </div>
      {tiers.map((tier, index) => (
        <fieldset
          key={index}
          className='border border-gray-200 dark:border-gray-700 rounded-xl p-3 mb-3'
          style={{ minWidth: 0 }}
        >
          <legend className='text-sm px-1'>
            {t('档位')} {index + 1}
          </legend>
          <div className='flex flex-wrap gap-2 mb-3'>
            <Input
              aria-label={`${t('档位')} ${index + 1}`}
              value={tier.label}
              disabled={disabled}
              maxLength={128}
              onChange={(label) => updateTier(index, { label })}
              style={{ flex: '1 1 180px' }}
            />
            <Select
              aria-label={`${t('单位')} ${index + 1}`}
              value={tier.unit}
              disabled={disabled}
              onChange={(unit) => updateTier(index, { unit })}
              optionList={Object.entries(OFFICIAL_PRICE_UNITS).map(
                ([value, label]) => ({ value, label }),
              )}
            />
            <Button
              type='danger'
              disabled={disabled}
              onClick={() =>
                update({ tiers: tiers.filter((_, i) => i !== index) })
              }
            >
              {t('删除')}
            </Button>
          </div>
          <div className='grid grid-cols-2 gap-3'>
            {OFFICIAL_PRICE_COLUMNS.map(([key, label]) => (
              <label key={key} className='min-w-0 text-xs'>
                <span className='block mb-1'>{t(label)}</span>
                <InputNumber
                  aria-label={`${t(label)} ${index + 1}`}
                  value={tier[key] ?? undefined}
                  min={0}
                  max={1e9}
                  disabled={disabled}
                  onChange={(v) =>
                    updateTier(index, {
                      [key]: typeof v === 'number' ? v : null,
                    })
                  }
                  style={{ width: '100%' }}
                />
              </label>
            ))}
          </div>
        </fieldset>
      ))}
      <Button
        disabled={disabled || tiers.length >= 20}
        onClick={() =>
          update({
            currency: pricing.currency || 'USD',
            tiers: [...tiers, { label: '', unit: 'million_tokens' }],
          })
        }
      >
        {t('添加价格档位')}
      </Button>
      <TextArea
        aria-label={t('价格说明')}
        placeholder={t('价格说明')}
        value={pricing.notes || ''}
        disabled={disabled}
        onChange={(notes) => update({ notes })}
        maxLength={2000}
        rows={2}
        style={{ marginTop: 12 }}
      />
    </div>
  );
}
export default withField(OfficialPricingEditor);
