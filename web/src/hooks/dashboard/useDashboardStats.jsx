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

import { useMemo } from 'react';
import { renderQuota } from '../../helpers/dashboardFormat';

export const useDashboardStats = (
  userState,
  consumeQuota,
  consumeTokens,
  times,
  performanceMetrics,
  t,
) => {
  const groupedStatsData = useMemo(
    () => [
      {
        id: 'account',
        title: t('当前余额'),
        value:
          userState?.user?.quota == null
            ? '—'
            : renderQuota(userState.user.quota),
        secondary: [
          {
            label: t('历史消耗'),
            value:
              userState?.user?.used_quota == null
                ? '—'
                : renderQuota(userState.user.used_quota),
          },
          {
            label: t('请求次数'),
            value: userState?.user?.request_count?.toLocaleString() ?? '—',
          },
        ],
      },
      {
        id: 'spend',
        title: t('统计额度'),
        value: renderQuota(consumeQuota),
        secondary: [],
      },
      {
        id: 'requests',
        title: t('统计次数'),
        value: Number(times || 0).toLocaleString(),
        secondary: [{ label: t('平均RPM'), value: performanceMetrics.avgRPM }],
      },
      {
        id: 'tokens',
        title: t('统计Tokens'),
        value: Number(consumeTokens || 0).toLocaleString(),
        secondary: [{ label: t('平均TPM'), value: performanceMetrics.avgTPM }],
      },
    ],
    [
      userState?.user?.quota,
      userState?.user?.used_quota,
      userState?.user?.request_count,
      consumeQuota,
      consumeTokens,
      times,
      performanceMetrics,
      t,
    ],
  );
  return { groupedStatsData };
};
