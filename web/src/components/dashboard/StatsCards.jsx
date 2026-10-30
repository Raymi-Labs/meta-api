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
import { useTranslation } from 'react-i18next';
import { Skeleton } from '@douyinfe/semi-ui';
import { CircleDollarSign, Activity, Hash, Gauge, Coins } from 'lucide-react';

const icons = {
  spend: CircleDollarSign,
  requests: Activity,
  tokens: Hash,
  cache: Gauge,
  'unit-price': Coins,
};
export default function StatsCards({
  groupedStatsData,
  loading,
  quotaError,
  cacheLoading,
  cacheError,
  retryCache,
}) {
  const { t } = useTranslation();
  return (
    <div className='dashboard-stat-grid'>
      {groupedStatsData.map((card) => {
        const Icon = icons[card.id];
        const isCache = card.id === 'cache';
        const busy = loading || (isCache && cacheLoading);
        const failed = quotaError || (isCache && cacheError);
        return (
          <section
            className='dashboard-stat-card'
            key={card.id}
            aria-label={card.title}
            aria-busy={busy}
            title={card.description}
          >
            <div className='dashboard-stat-heading'>
              <span>{card.title}</span>
              <Icon size={17} aria-hidden='true' />
            </div>
            <div className='dashboard-stat-value'>
              <Skeleton
                loading={busy}
                active
                placeholder={
                  <Skeleton.Title style={{ width: 110, height: 30 }} />
                }
              >
                {failed ? '—' : card.value}
              </Skeleton>
            </div>
            {isCache && cacheError && !busy && !quotaError && (
              <button
                className='dashboard-cache-retry'
                type='button'
                onClick={retryCache}
              >
                {t('加载失败，请重试')}
              </button>
            )}
          </section>
        );
      })}
    </div>
  );
}
