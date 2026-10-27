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
import { Link } from 'react-router-dom';
import {
  RefreshCw,
  SlidersHorizontal,
  KeyRound,
  List,
  Wallet,
  Network,
  Users,
  Globe,
  ChevronDown,
} from 'lucide-react';
import { dashboardQuickActions } from '../../helpers/dashboardQuery';
import { useSidebar } from '../../hooks/common/useSidebar';

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
  getGreeting,
  rangePreset,
  onRangeChange,
  inputs,
  dataExportDefaultTime,
  isAdminUser,
  showSearchModal,
  refresh,
  loading,
  t,
}) {
  const { isModuleVisible } = useSidebar();
  const actions = dashboardQuickActions(isAdminUser, isModuleVisible);
  const icons = {
    token: KeyRound,
    log: List,
    topup: Wallet,
    channel: Network,
    user: Users,
  };
  return (
    <header className='dashboard-overview-header'>
      <div className='dashboard-overview-title'>
        <div>
          <h1>{t('概览')}</h1>
          <p>{getGreeting.replace(/^👋/, '')}</p>
        </div>
        <div className='dashboard-query-actions'>
          <span
            className='dashboard-timezone'
            title={Intl.DateTimeFormat().resolvedOptions().timeZone}
          >
            <Globe size={14} aria-hidden='true' />
            {Intl.DateTimeFormat().resolvedOptions().timeZone}
          </span>
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
      </div>
      <div className='dashboard-overview-toolbar'>
        <div className='dashboard-query-summary'>
          <span className='dashboard-query-scope'>
            {t(
              isAdminUser
                ? inputs.username
                  ? '用户名称'
                  : '全部用户'
                : '当前用户',
            )}
            {isAdminUser && inputs.username ? `: ${inputs.username}` : ''}
          </span>
          <span>
            {displayTime(inputs.start_timestamp)} —{' '}
            {displayTime(inputs.end_timestamp)}
          </span>
          <span className='dashboard-query-zone'>
            {t('时间粒度')}:{' '}
            {t({ hour: '小时', day: '天', week: '周' }[dataExportDefaultTime])}
          </span>
        </div>
        <nav className='dashboard-quick-actions' aria-label={t('常用操作')}>
          {actions.map(([, module, to, label]) => {
            const Icon = icons[module];
            return (
              <Link key={module} to={to}>
                <Icon size={15} aria-hidden='true' />
                {t(label)}
              </Link>
            );
          })}
        </nav>
      </div>
      <p className='dashboard-scope-note'>
        {t(
          '余额与累计值属于当前账户；其余指标按所选用户和时间范围统计，不代表请求成功率。',
        )}
      </p>
    </header>
  );
}
