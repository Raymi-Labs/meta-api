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
import { Button, Skeleton } from '@douyinfe/semi-ui';
import { BarChart2, RefreshCw, TrendingUp, Wallet } from 'lucide-react';
import { renderQuota } from '../../helpers';

export default function WalletOverview({ user, loading, error, onRefresh, t }) {
  const quota = (value) =>
    typeof value === 'number' && Number.isFinite(value)
      ? renderQuota(value)
      : '—';
  const stats = [
    { label: '当前余额', value: quota(user?.quota), Icon: Wallet },
    { label: '历史消耗', value: quota(user?.used_quota), Icon: TrendingUp },
    {
      label: '请求次数',
      value: Number.isFinite(user?.request_count) ? user.request_count : '—',
      Icon: BarChart2,
    },
  ];
  return (
    <section className='wallet-account-summary'>
      <div className='wallet-summary-heading'>
        <h2>{t('账户统计')}</h2>
        <Button
          theme='borderless'
          icon={<RefreshCw size={14} />}
          loading={loading}
          aria-label={t('刷新')}
          title={t('刷新')}
          onClick={onRefresh}
          className='wallet-refresh'
          size='small'
        />
      </div>
      <div className='wallet-statistics'>
        {stats.map(({ label, value, Icon }) => (
          <div key={label}>
            {loading ? (
              <Skeleton.Title active style={{ width: '80%', height: 28 }} />
            ) : (
              <strong>{error ? '—' : value}</strong>
            )}
            <span>
              <Icon size={14} />
              {t(label)}
            </span>
          </div>
        ))}
      </div>
      {error && (
        <p role='alert' className='wallet-balance-error'>
          {t('加载失败，请重试')}
        </p>
      )}
    </section>
  );
}
