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

import React, { useState } from 'react';
import { Button, Select } from '@douyinfe/semi-ui';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  useGroupHealth,
  UPTIME_WINDOW_HOURS,
} from '../../../../hooks/group-health/useGroupHealth';
import {
  GroupHealthLegend,
  GroupHealthCollectionNote,
  GroupHealthPanel,
  GroupHealthScopeNote,
  GroupHealthStatus,
  resolveGroupHealthMetric,
  selectHealthGroups,
} from '../../../group-health';

const GroupHealthOverview = ({
  filteredModels = [],
  usableGroup = {},
  groupRatio = {},
  filterGroup = 'all',
  filterVendor = 'all',
  filterTag = 'all',
  filterQuotaType = 'all',
  filterEndpointType = 'all',
  searchValue = '',
  loading: pricingLoading,
  t,
}) => {
  const { i18n } = useTranslation();
  const [selectedModel, setSelectedModel] = useState('');
  const currentModel = filteredModels.find(
    (model) => model.model_name === selectedModel,
  );
  const modelName = currentModel?.model_name || '';
  const { data, loading, error, refresh, isAuthenticated } = useGroupHealth({
    hours: UPTIME_WINDOW_HOURS,
    model: modelName,
    group: filterGroup === 'all' ? undefined : filterGroup,
    enabled: !pricingLoading && filterGroup !== 'auto',
  });
  const groups = selectHealthGroups({
    groups: data?.groups,
    models: currentModel ? [currentModel] : filteredModels,
    usableGroup,
    filterGroup,
    exactModel: Boolean(currentModel),
    filterByModels:
      filterVendor !== 'all' ||
      filterTag !== 'all' ||
      filterQuotaType !== 'all' ||
      filterEndpointType !== 'all' ||
      searchValue.length > 0,
  });

  return (
    <div className='group-health-overview'>
      <div className='group-health-toolbar'>
        <div>
          <div className='group-health-toolbar-title'>{t('分组健康')}</div>
          <div className='group-health-caption'>
            {t('最近 {{hours}} 小时', { hours: UPTIME_WINDOW_HOURS })} ·{' '}
            {t('每分钟更新')}
            {data?.updated_at
              ? ` · ${t('最近采样')} ${new Date(data.updated_at * 1000).toLocaleTimeString(i18n.resolvedLanguage || i18n.language, { hour: '2-digit', minute: '2-digit' })}`
              : ''}
          </div>
        </div>
        <div className='group-health-toolbar-controls'>
          <Select
            className='group-health-filter-select'
            aria-label={t('健康统计模型')}
            filter
            value={modelName || '__all__'}
            onChange={(value) =>
              setSelectedModel(value === '__all__' ? '' : value)
            }
            optionList={[
              { value: '__all__', label: t('全部模型 · 分组汇总') },
              ...filteredModels.map((model) => ({
                value: model.model_name,
                label: model.model_name,
              })),
            ]}
          />
          <Button
            size='small'
            disabled={!isAuthenticated || loading || filterGroup === 'auto'}
            onClick={() => refresh()}
          >
            {t('刷新')}
          </Button>
        </div>
      </div>
      {!isAuthenticated ? (
        <div className='group-health-state'>
          <Link to='/login'>{t('登录后查看可用分组的调用表现')}</Link>
        </div>
      ) : filterGroup === 'auto' ? (
        <div className='group-health-state'>
          {t('自动选择分组，表现按实际调用分组统计')}
        </div>
      ) : (
        <>
          {!modelName && (
            <p
              className='group-health-note'
              style={{ marginTop: 0, marginBottom: 12 }}
            >
              {t('分组汇总包含该组全部模型；选择具体模型可比较同模型表现。')}
            </p>
          )}
          <GroupHealthLegend uptime />
          <GroupHealthStatus
            loading={pricingLoading || (loading && !data)}
            error={error}
            onRetry={refresh}
          />
          <GroupHealthCollectionNote data={data} />
          {!pricingLoading && !error && data?.enabled === false && (
            <div className='group-health-state'>{t('分组健康统计未启用')}</div>
          )}
          {!pricingLoading && !error && data && data.enabled !== false && (
            <>
              {groups.length ? (
                <div className='group-health-grid'>
                  {groups.map((stats) => (
                    <GroupHealthPanel
                      uptime
                      key={`${modelName}:${stats.group}`}
                      stats={stats}
                      ratio={groupRatio[stats.group]}
                      metric={resolveGroupHealthMetric(
                        stats,
                        `${stats.group} ${modelName}`,
                        true,
                      )}
                    />
                  ))}
                </div>
              ) : (
                <div className='group-health-state'>
                  {t('当前筛选下没有可用分组')}
                </div>
              )}
              <GroupHealthScopeNote uptime />
              <p className='group-health-note'>
                {t('仅展示你可用的分组。灰色表示样本不足或无有效统计样本。')}
              </p>
            </>
          )}
        </>
      )}
    </div>
  );
};

export default GroupHealthOverview;
