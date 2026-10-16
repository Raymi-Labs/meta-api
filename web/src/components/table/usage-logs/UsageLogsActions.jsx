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
import { Button, Tag, Space, Skeleton } from '@douyinfe/semi-ui';
import { renderQuota } from '../../../helpers';
import CompactModeToggle from '../../common/ui/CompactModeToggle';
import { useMinimumLoadingTime } from '../../../hooks/common/useMinimumLoadingTime';
import { BarChart3 } from 'lucide-react';

function formatPercent(value) {
  const rate = Number(value || 0);
  if (!Number.isFinite(rate) || rate <= 0) {
    return '0.00%';
  }
  return `${(rate * 100).toFixed(2)}%`;
}

const LogsActions = ({
  exporting,
  exportLogs,
  stat,
  statError,
  loadingStat,
  showStat,
  compactMode,
  setCompactMode,
  isAdminUser,
  openTopUsersDrawer,
  openGroupSummary,
  t,
}) => {
  const showSkeleton = useMinimumLoadingTime(loadingStat);
  const needSkeleton = !statError && (!showStat || showSkeleton);

  const placeholder = (
    <Space>
      <Skeleton.Title style={{ width: 108, height: 21, borderRadius: 6 }} />
      <Skeleton.Title style={{ width: 65, height: 21, borderRadius: 6 }} />
      <Skeleton.Title style={{ width: 64, height: 21, borderRadius: 6 }} />
      <Skeleton.Title style={{ width: 108, height: 21, borderRadius: 6 }} />
      <Skeleton.Title style={{ width: 108, height: 21, borderRadius: 6 }} />
    </Space>
  );

  return (
    <div className='flex flex-col md:flex-row justify-between items-start md:items-center gap-2 w-full'>
      <Skeleton loading={needSkeleton} active placeholder={placeholder}>
        {statError ? (
          <Tag color='red'>{statError}</Tag>
        ) : (
          <Space
            title={t(
              '额度与缓存率仅统计消费记录；缓存率可能取最近样本，RPM/TPM 为所选范围内最近一分钟。',
            )}
          >
            <Tag
              color='blue'
              style={{
                fontWeight: 500,
                boxShadow: '0 2px 8px rgba(0, 0, 0, 0.1)',
                padding: 13,
              }}
              className='!rounded-lg'
            >
              {t('消耗额度')}: {renderQuota(stat.quota)}
            </Tag>
            <Tag
              color='pink'
              style={{
                fontWeight: 500,
                boxShadow: '0 2px 8px rgba(0, 0, 0, 0.1)',
                padding: 13,
              }}
              className='!rounded-lg'
            >
              RPM: {stat.rpm}
            </Tag>
            <Tag
              color='white'
              style={{
                border: 'none',
                boxShadow: '0 2px 8px rgba(0, 0, 0, 0.1)',
                fontWeight: 500,
                padding: 13,
              }}
              className='!rounded-lg'
            >
              TPM: {stat.tpm}
            </Tag>
            <Tag
              color='green'
              style={{
                fontWeight: 500,
                boxShadow: '0 2px 8px rgba(0, 0, 0, 0.1)',
                padding: 13,
              }}
              className='!rounded-lg'
            >
              {t('命中缓存率')}: {formatPercent(stat.cache_hit_rate)}
            </Tag>
            <Tag
              color='teal'
              style={{
                fontWeight: 500,
                boxShadow: '0 2px 8px rgba(0, 0, 0, 0.1)',
                padding: 13,
              }}
              className='!rounded-lg'
            >
              {t('全局缓存率')}: {formatPercent(stat.cache_global_rate)}
            </Tag>
          </Space>
        )}
      </Skeleton>

      <div className='flex flex-wrap items-center gap-2 self-end md:self-auto'>
        <Button
          type='primary'
          size='small'
          loading={exporting}
          disabled={exporting}
          onClick={exportLogs}
        >
          {t('导出日志')}
        </Button>
        {isAdminUser && (
          <Button type='tertiary' size='small' onClick={openGroupSummary}>
            {t('分组汇总')}
          </Button>
        )}
        {isAdminUser ? (
          <Button
            type='tertiary'
            icon={<BarChart3 size={16} />}
            onClick={openTopUsersDrawer}
            size='small'
          >
            {t('大用户榜单')}
          </Button>
        ) : null}
        <CompactModeToggle
          compactMode={compactMode}
          setCompactMode={setCompactMode}
          t={t}
        />
      </div>
    </div>
  );
};

export default LogsActions;
