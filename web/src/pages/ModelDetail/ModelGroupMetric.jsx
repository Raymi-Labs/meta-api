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
import { Link } from 'react-router-dom';
import { ChevronDown } from 'lucide-react';
import { modelGroupHealthState } from '../../helpers/modelGroupHealthState';

export default function ModelGroupMetric({
  health,
  value,
  formatted,
  rate = false,
  samples = 0,
  modelName,
  label,
  t,
  onToggle,
  open = false,
  tone = 'empty',
}) {
  const state = modelGroupHealthState(health, value, { rate, samples });
  if (state === 'ready')
    return onToggle ? (
      <button
        type='button'
        className={'model-group-uptime group-health-text-' + tone}
        aria-expanded={open}
        aria-label={label}
        onClick={onToggle}
      >
        {formatted}
        <ChevronDown size={13} aria-hidden='true' />
      </button>
    ) : (
      <span>{formatted}</span>
    );
  if (state === 'anonymous')
    return (
      <Link
        className='model-group-metric-state'
        to={
          '/login?next=' +
          encodeURIComponent('/model/' + encodeURIComponent(modelName))
        }
      >
        {t('登录后查看')}
      </Link>
    );
  if (state === 'error')
    return (
      <button
        type='button'
        className='model-group-uptime model-group-metric-state'
        aria-label={label + ' · ' + t('重试')}
        onClick={() => health.refresh()}
      >
        {t('加载失败')} · {t('重试')}
      </button>
    );
  const text = {
    loading: '加载中...',
    disabled: '暂未开启',
    limited: '样本不足',
    empty: '暂无统计数据',
  }[state];
  return (
    <span className='model-group-metric-state' role='status'>
      {t(text)}
    </span>
  );
}
