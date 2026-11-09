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
import { Tooltip } from '@douyinfe/semi-ui';
import { Info } from 'lucide-react';
import { useTranslation } from 'react-i18next';

export default function ServiceTierControl({
  value = 'default',
  capability,
  onChange,
  disabled,
}) {
  const { t } = useTranslation();
  const labels = {
    default: t('标准'),
    priority: 'Fast ×2',
    ultrafast: 'UltraFast ×6',
  };
  const selected = value || 'default';
  const invalid = !capability.allows(selected);
  return (
    <section className='pg-settings-section pg-service-tier'>
      <div className='pg-service-tier-heading'>
        <label htmlFor='pg-service-tier'>{t('速度档位')}</label>
        <Tooltip
          content={t('加速按所选分组普通价的2倍或6倍计费，与推理强度独立。')}
        >
          <button
            type='button'
            className='pg-info-button'
            aria-label={t('速度档位说明')}
          >
            <Info size={14} />
          </button>
        </Tooltip>
      </div>
      <select
        id='pg-service-tier'
        value={selected}
        disabled={disabled || capability.loading}
        onChange={(e) => onChange(e.target.value)}
        aria-invalid={invalid}
      >
        {capability.tiers.map((tier) => (
          <option value={tier.id} key={tier.id}>
            {labels[tier.id]}
          </option>
        ))}
        {invalid && (
          <option value={selected} disabled>
            {labels[selected] || selected} — {t('不可用')}
          </option>
        )}
      </select>
      {(capability.error || invalid) && (
        <div className='pg-service-tier-error' role='status'>
          <span>
            {capability.loading ? t('加载中...') : t('当前速度档位不可用')}
          </span>
          <button
            type='button'
            onClick={capability.retry}
            disabled={capability.loading}
          >
            {t('重试')}
          </button>
        </div>
      )}
    </section>
  );
}
