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
import { Button } from '@douyinfe/semi-ui';
import {
  CalendarDays,
  RefreshCw,
  SlidersHorizontal,
  ChevronDown,
} from 'lucide-react';

const displayTime = (value) => {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return '—';
  return date.toLocaleString(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  });
};

export default function DashboardHeader({
  inputs = {},
  rangePreset,
  onRangeChange,
  showSearchModal,
  refresh,
  loading,
  t,
}) {
  return (
    <header className='dashboard-overview-header'>
      <div className='dashboard-applied-range' aria-label={t('时间范围')}>
        <CalendarDays size={15} aria-hidden='true' />
        <span>{displayTime(inputs.start_timestamp)}</span>
        <span aria-hidden='true'>—</span>
        <span>{displayTime(inputs.end_timestamp)}</span>
      </div>
      <div className='dashboard-query-actions'>
        <Button
          icon={<SlidersHorizontal size={15} />}
          aria-label={t('筛选')}
          title={t('筛选')}
          theme='outline'
          type='tertiary'
          onClick={showSearchModal}
        />
        <div className='dashboard-range-control'>
          <select
            className='dashboard-range-select'
            aria-label={t('时间范围')}
            value={rangePreset}
            onChange={(event) => onRangeChange(event.target.value)}
          >
            <option value='today'>{t('今天')}</option>
            <option value='24h'>{t('近24小时')}</option>
            <option value='7d'>{t('最近7天')}</option>
            <option value='30d'>{t('近30天')}</option>
            <option value='custom'>{t('自定义')}</option>
          </select>
          <ChevronDown size={14} aria-hidden='true' />
        </div>
        <Button
          icon={<RefreshCw size={15} />}
          aria-label={t('刷新')}
          title={t('刷新')}
          theme='borderless'
          type='tertiary'
          onClick={refresh}
          loading={loading}
        />
      </div>
    </header>
  );
}
