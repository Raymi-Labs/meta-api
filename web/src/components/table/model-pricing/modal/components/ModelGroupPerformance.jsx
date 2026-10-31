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

import React, { useEffect, useState } from 'react';
import { Button, Tabs, TabPane } from '@douyinfe/semi-ui';
import { Link } from 'react-router-dom';
import {
  useGroupHealth,
  UPTIME_WINDOW_HOURS,
} from '../../../../../hooks/group-health/useGroupHealth';
import {
  GroupHealthLegend,
  GroupHealthCollectionNote,
  GroupHealthPanel,
  GroupHealthScopeNote,
  GroupHealthStatus,
  GroupHealthSummary,
  resolveGroupHealthMetric,
  selectHealthGroups,
} from '../../../../group-health';
import ModelPricingTable from './ModelPricingTable';

const ModelGroupPerformance = ({ enabled, ...props }) => {
  const { modelData, groupRatio, usableGroup, t } = props;
  const [activeTab, setActiveTab] = useState('pricing');
  const modelName = modelData?.model_name || modelData?.modelName;
  const { data, loading, error, refresh, isAuthenticated } = useGroupHealth({
    hours: UPTIME_WINDOW_HOURS,
    model: modelName,
    enabled: enabled && Boolean(modelName),
  });
  useEffect(() => {
    setActiveTab('pricing');
  }, [modelName]);
  const groups = selectHealthGroups({
    groups: data?.groups,
    models: modelData ? [modelData] : [],
    usableGroup,
    exactModel: true,
  });

  const content = !isAuthenticated ? (
    <div className='group-health-state'>
      <Link to='/login'>{t('登录后查看可用分组的调用表现')}</Link>
    </div>
  ) : (
    <>
      <GroupHealthStatus
        loading={loading && !data}
        error={error}
        onRetry={refresh}
      />
      <GroupHealthCollectionNote data={data} />
      {!error && data?.enabled === false && (
        <div className='group-health-state'>{t('分组健康统计未启用')}</div>
      )}
      {!error && data && data.enabled !== false && (
        <>
          <div className='group-health-caption' style={{ marginBottom: 10 }}>
            {modelName} ·{' '}
            {t('最近 {{hours}} 小时', { hours: UPTIME_WINDOW_HOURS })} ·{' '}
            {t('每分钟更新')}
          </div>
          <GroupHealthLegend uptime />
          <div className='group-health-stack'>
            {groups.map((stats) => (
              <GroupHealthPanel
                uptime
                key={`${modelName}:${stats.group}`}
                stats={stats}
                ratio={groupRatio?.[stats.group]}
                metric={resolveGroupHealthMetric(
                  stats,
                  `${stats.group} ${modelName}`,
                  true,
                )}
              />
            ))}
          </div>
          {!groups.length && (
            <div className='group-health-state'>
              {t('当前模型没有可用分组')}
            </div>
          )}
          <GroupHealthScopeNote uptime />
        </>
      )}
    </>
  );

  return (
    <Tabs activeKey={activeTab} onChange={setActiveTab} type='line'>
      <TabPane tab={t('分组价格')} itemKey='pricing'>
        <div style={{ paddingTop: 16 }}>
          <ModelPricingTable {...props} />
        </div>
        {isAuthenticated && (
          <div className='group-health-price-summary'>
            <div className='group-health-price-summary-heading'>
              <span>
                {t('该模型的分组表现')} ·{' '}
                {t('最近 {{hours}} 小时', { hours: UPTIME_WINDOW_HOURS })}
              </span>
              <Button
                size='small'
                theme='borderless'
                onClick={() => setActiveTab('performance')}
              >
                {t('查看时间趋势')}
              </Button>
            </div>
            <GroupHealthStatus
              loading={loading && !data}
              error={error}
              onRetry={refresh}
            />
            <GroupHealthCollectionNote data={data} />
            {!error && data?.enabled === false && (
              <div className='group-health-caption'>
                {t('分组健康统计未启用')}
              </div>
            )}
            {!error &&
              data &&
              data.enabled !== false &&
              groups.map((stats) => (
                <div className='group-health-price-row' key={stats.group}>
                  <span>{stats.group}</span>
                  <GroupHealthSummary
                    uptime
                    stats={stats}
                    metric={resolveGroupHealthMetric(
                      stats,
                      `${stats.group} ${modelName}`,
                      true,
                    )}
                    compact
                  />
                </div>
              ))}
          </div>
        )}
      </TabPane>
      <TabPane tab={t('调用表现')} itemKey='performance'>
        <div style={{ paddingTop: 16 }}>{content}</div>
      </TabPane>
    </Tabs>
  );
};

export default ModelGroupPerformance;
