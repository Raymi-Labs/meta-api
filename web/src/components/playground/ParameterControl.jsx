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
import { InputNumber, Slider, Switch, Tooltip } from '@douyinfe/semi-ui';
import { Info } from 'lucide-react';
import { useTranslation } from 'react-i18next';

export default function ParameterControl({
  inputs,
  parameterEnabled,
  onInputChange,
  onParameterToggle,
  disabled = false,
}) {
  const { t } = useTranslation();
  const parameters = [
    {
      key: 'temperature',
      label: 'Temperature',
      min: 0,
      max: 2,
      step: 0.1,
      hint: t('控制输出的随机性和创造性'),
    },
    {
      key: 'top_p',
      label: 'Top P',
      min: 0,
      max: 1,
      step: 0.05,
      hint: t('核采样，控制词汇选择的多样性'),
    },
    {
      key: 'frequency_penalty',
      label: 'Frequency penalty',
      min: -2,
      max: 2,
      step: 0.1,
      hint: t('频率惩罚，减少重复词汇的出现'),
    },
    {
      key: 'presence_penalty',
      label: 'Presence penalty',
      min: -2,
      max: 2,
      step: 0.1,
      hint: t('存在惩罚，鼓励讨论新话题'),
    },
    { key: 'max_tokens', label: 'Max tokens', min: 1, step: 1 },
    { key: 'seed', label: 'Seed', step: 1, hint: t('可选，用于复现结果') },
  ];
  return (
    <div className='pg-parameter-list'>
      {parameters.map((param) => (
        <div className='pg-parameter' key={param.key}>
          <div className='pg-parameter-heading'>
            <label htmlFor={`pg-${param.key}`}>
              {param.label}
              {param.hint && (
                <Tooltip content={param.hint}>
                  <Info size={12} />
                </Tooltip>
              )}
            </label>
            <Switch
              size='small'
              aria-label={`${t('参数')} ${param.key}`}
              checked={Boolean(parameterEnabled[param.key])}
              disabled={disabled}
              onChange={() => onParameterToggle(param.key)}
            />
          </div>
          <div className='pg-parameter-value'>
            {param.max !== undefined && (
              <Slider
                min={param.min}
                max={param.max}
                step={param.step}
                value={Number(inputs[param.key])}
                aria-label={param.label}
                disabled={disabled || !parameterEnabled[param.key]}
                onChange={(value) => onInputChange(param.key, value)}
              />
            )}
            <InputNumber
              id={`pg-${param.key}`}
              aria-label={param.label}
              hideButtons
              min={param.min}
              max={param.max}
              step={param.step}
              value={inputs[param.key]}
              disabled={disabled || !parameterEnabled[param.key]}
              onChange={(value) =>
                onInputChange(param.key, value === '' ? null : value)
              }
            />
          </div>
        </div>
      ))}
    </div>
  );
}
