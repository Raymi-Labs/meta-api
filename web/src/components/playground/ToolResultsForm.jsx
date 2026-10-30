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
import { Wrench } from 'lucide-react';
import { useTranslation } from 'react-i18next';

export default function ToolResultsForm({
  calls,
  onSubmit,
  disabled,
  values,
  onChange,
}) {
  const { t } = useTranslation();
  return (
    <form
      className='pg-tool-results'
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit(values);
      }}
    >
      <div className='pg-tool-results-heading'>
        <Wrench size={15} />
        <strong>{t('等待工具结果')}</strong>
        <span>{calls.length}</span>
      </div>
      <p>{t('请自行执行函数并填入结果，页面不会自动执行工具。')}</p>
      <div className='pg-tool-result-fields'>
        {calls.map((call) => (
          <label key={call.id}>
            <code>{call.function.name}</code>
            <textarea
              aria-label={`${t('工具结果')} ${call.function.name}`}
              placeholder={t('填入工具执行结果')}
              required
              disabled={disabled}
              value={values[call.id] || ''}
              onChange={(e) =>
                onChange((current) => ({
                  ...current,
                  [call.id]: e.target.value,
                }))
              }
            />
          </label>
        ))}
      </div>
      <button
        className='pg-primary-button'
        disabled={disabled || calls.some((call) => !values[call.id]?.trim())}
        type='submit'
      >
        {t('提交结果并继续')}
      </button>
    </form>
  );
}
