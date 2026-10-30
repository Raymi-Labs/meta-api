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
import { renderQuota } from '../../../helpers';

const LogsStats = ({
  stat,
  statError,
  loadingStat,
  showStat,
  handleEyeClick,
  t,
}) => {
  const number = (value) =>
    typeof value === 'number' && Number.isFinite(value) ? value : null;
  const percent = (value) =>
    number(value) === null ? '—' : `${(value * 100).toFixed(2)}%`;
  const metrics = [
    [
      t('消耗额度'),
      number(stat.quota) === null ? '—' : renderQuota(stat.quota),
    ],
    ['RPM', number(stat.rpm) ?? '—'],
    ['TPM', number(stat.tpm) ?? '—'],
    [t('命中缓存率'), percent(stat.cache_hit_rate)],
    [t('全局缓存率'), percent(stat.cache_global_rate)],
  ];
  return (
    <div className='usage-logs-stats'>
      {statError ? (
        <div role='alert' className='usage-logs-stat-error'>
          <span>{t('加载统计失败')}</span>
          <Button size='small' onClick={handleEyeClick}>
            {t('重试')}
          </Button>
        </div>
      ) : (
        metrics.map(([label, value]) => (
          <div className='usage-logs-metric' key={label}>
            <span>{label}</span>
            {loadingStat || !showStat ? (
              <Skeleton.Title style={{ width: 42, height: 13 }} />
            ) : (
              <strong>{value}</strong>
            )}
          </div>
        ))
      )}
    </div>
  );
};
export default LogsStats;
