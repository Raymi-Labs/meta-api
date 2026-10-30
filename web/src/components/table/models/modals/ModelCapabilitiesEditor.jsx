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
import { Select, withField } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { MODEL_CAPABILITY_LABELS } from '../../../../helpers/modelAttributes';

function ModelCapabilitiesEditor({ value = {}, onChange }) {
  const { t } = useTranslation();
  return (
    <div className='grid grid-cols-1 sm:grid-cols-2 gap-x-4 gap-y-3'>
      {Object.entries(MODEL_CAPABILITY_LABELS).map(([key, label]) => (
        <div
          key={key}
          className='flex items-center justify-between gap-2 min-w-0'
        >
          <span className='text-sm' id={`capability-label-${key}`}>
            {t(label)}
          </span>
          <Select
            aria-labelledby={`capability-label-${key}`}
            value={
              value?.[key] === true
                ? 'yes'
                : value?.[key] === false
                  ? 'no'
                  : 'unknown'
            }
            style={{ width: 112, flexShrink: 0 }}
            optionList={[
              { value: 'unknown', label: t('未知') },
              { value: 'yes', label: t('支持') },
              { value: 'no', label: t('不支持') },
            ]}
            onChange={(selected) => {
              const next = { ...value };
              if (selected === 'unknown') delete next[key];
              else next[key] = selected === 'yes';
              onChange(next);
            }}
          />
        </div>
      ))}
    </div>
  );
}

export default withField(ModelCapabilitiesEditor);
