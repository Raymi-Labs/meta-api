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
import { RefreshCw, SlidersHorizontal, Columns3, Download } from 'lucide-react';
import { renderQuota } from '../../../helpers';
import CompactModeToggle from '../../common/ui/CompactModeToggle';

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
  refresh,
  handleEyeClick,
  loading,
  filtersOpen,
  setFiltersOpen,
  columnViewMode,
  setColumnViewMode,
  openColumnSelector,
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
    <>
      <div className='usage-logs-heading'>
        <h1>{t('使用日志')}</h1>
      </div>
      <div className='usage-logs-toolbar'>
        <div className='usage-logs-tools'>
          <Button
            size='small'
            theme='solid'
            icon={<RefreshCw size={15} />}
            onClick={refresh}
            loading={loading || loadingStat}
          >
            {t('刷新日志')}
          </Button>
          <Button
            size='small'
            theme='light'
            icon={<SlidersHorizontal size={15} />}
            className={filtersOpen ? 'console-control-active' : undefined}
            aria-expanded={filtersOpen}
            onClick={() => setFiltersOpen(!filtersOpen)}
          >
            {t('筛选')}
          </Button>
          {!isAdminUser && (
            <>
              <Button
                size='small'
                theme='light'
                className={
                  columnViewMode === 'summary'
                    ? 'console-control-active'
                    : undefined
                }
                aria-pressed={columnViewMode === 'summary'}
                onClick={() => setColumnViewMode('summary')}
              >
                {t('摘要视图')}
              </Button>
              <Button
                size='small'
                theme='light'
                className={
                  columnViewMode === 'full'
                    ? 'console-control-active'
                    : undefined
                }
                aria-pressed={columnViewMode === 'full'}
                onClick={() => setColumnViewMode('full')}
              >
                {t('完整视图')}
              </Button>
            </>
          )}
          <Button
            size='small'
            theme='light'
            icon={<Columns3 size={15} />}
            onClick={openColumnSelector}
          >
            {t('列设置')}
          </Button>
        </div>
        <div className='usage-logs-tools'>
          {isAdminUser && (
            <>
              <Button size='small' theme='light' onClick={openGroupSummary}>
                {t('分组汇总')}
              </Button>
              <Button size='small' theme='light' onClick={openTopUsersDrawer}>
                {t('大用户榜单')}
              </Button>
            </>
          )}
          <Button
            size='small'
            theme='light'
            icon={<Download size={15} />}
            loading={exporting}
            disabled={exporting}
            onClick={exportLogs}
          >
            {t('导出日志')}
          </Button>
          <CompactModeToggle
            compactMode={compactMode}
            setCompactMode={setCompactMode}
            t={t}
          />
        </div>
      </div>
      <p className='usage-logs-hint'>
        {t('筛选仅在查询后生效，刷新、导出与汇总沿用已应用条件。')}
      </p>
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
                <Skeleton.Title style={{ width: 70, height: 18 }} />
              ) : (
                <strong>{value}</strong>
              )}
            </div>
          ))
        )}
      </div>
      <p className='usage-logs-hint'>
        {t(
          '额度与缓存率仅统计消费记录；缓存率可能取最近样本，RPM/TPM 为所选范围内最近一分钟。',
        )}
      </p>
    </>
  );
};
export default LogsActions;
