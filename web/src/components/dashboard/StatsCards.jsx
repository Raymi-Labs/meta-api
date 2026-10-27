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
import { Skeleton } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { Wallet, CircleDollarSign, Activity, Hash } from 'lucide-react';

const icons = {
  account: Wallet,
  spend: CircleDollarSign,
  requests: Activity,
  tokens: Hash,
};
export default function StatsCards({
  groupedStatsData,
  loading,
  quotaError,
  accountLoading,
  accountError,
}) {
  const { t } = useTranslation();
  return (
    <div className='dashboard-stat-grid'>
      {groupedStatsData.map((card) => {
        const Icon = icons[card.id];
        const busy = card.id === 'account' ? accountLoading : loading;
        const failed = card.id === 'account' ? accountError : quotaError;
        return (
          <section
            className='dashboard-stat-card'
            key={card.id}
            aria-label={card.title}
            aria-busy={busy}
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
            <div className='dashboard-stat-scope'>
              {t(card.id === 'account' ? '当前账户' : '选定时间范围')}
            </div>
            {card.secondary.length > 0 && (
              <dl className='dashboard-stat-secondary'>
                {card.secondary.map((item) => (
                  <div key={item.label}>
                    <dt>{item.label}</dt>
                    <dd>{busy || failed ? '—' : item.value}</dd>
                  </div>
                ))}
              </dl>
            )}
          </section>
        );
      })}
    </div>
  );
}
