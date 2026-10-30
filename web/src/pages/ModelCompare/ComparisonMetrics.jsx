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
import { Link, useLocation } from 'react-router-dom';
import { useGroupHealth } from '../../hooks/group-health/useGroupHealth';
import {
  GroupHealthStatus,
  GroupHealthCollectionNote,
  GroupHealthLegend,
} from '../../components/group-health';
import ModelMetricChart from '../ModelDetail/ModelMetricChart';

export function useComparisonHealth(models, group, hours) {
  const options = (index) => ({
    model: models[index]?.model_name,
    group,
    hours,
    enabled: Boolean(group && models[index]?.enable_groups?.includes(group)),
  });
  const a = useGroupHealth(options(0));
  const b = useGroupHealth(options(1));
  const c = useGroupHealth(options(2));
  const d = useGroupHealth(options(3));
  return [a, b, c, d].slice(0, models.length);
}

export function comparisonStats(health, group) {
  return !health?.error && health?.data?.enabled !== false
    ? health?.data?.groups.find((item) => item.group === group)
    : undefined;
}

export default function ComparisonMetrics({ models, health, group, t }) {
  const location = useLocation();
  const series = models.flatMap((model, index) => {
    const stats = comparisonStats(health[index], group);
    return stats ? [{ ...stats, group: model.model_name }] : [];
  });
  const authenticated = health[0]?.isAuthenticated;
  return (
    <>
      <section className='model-detail-section'>
        <h2>{t('性能对比')}</h2>
        <p className='model-detail-caption'>
          {t('平台分组统计，不限于个人请求。')} {t('当前小时尚未结束')} ·{' '}
          {t('每分钟更新')}
        </p>
        {!authenticated && (
          <p>
            <Link
              to={`/login?next=${encodeURIComponent(location.pathname + location.search)}`}
            >
              {t('登录后查看可用分组的调用表现')}
            </Link>
          </p>
        )}
        {health.some((item) => item.data?.enabled === false) && (
          <p>{t('管理员暂未开启分组健康展示')}</p>
        )}
        {models.map((model, index) => (
          <div key={model.model_name}>
            {(health[index].loading || health[index].error) && (
              <small>{model.display_name || model.model_name}</small>
            )}
            <GroupHealthStatus
              loading={health[index].loading}
              error={health[index].error}
              onRetry={health[index].refresh}
            />
            <GroupHealthCollectionNote data={health[index].data} />
          </div>
        ))}
        {authenticated && (
          <div className='model-performance-grid'>
            <ModelMetricChart
              title={'Uptime · ' + t('成功率')}
              groups={series}
              metric='success_rate'
              t={t}
            />
            <ModelMetricChart
              title={t('首字均值') + ' · s'}
              groups={series}
              metric='avg_ttft_ms'
              t={t}
            />
            <ModelMetricChart
              title={t('输出速度') + ' · tok/s'}
              groups={series}
              metric='avg_output_tps'
              t={t}
            />
          </div>
        )}
        {authenticated && <GroupHealthLegend />}
      </section>
      <section className='model-detail-section'>
        <h2>{t('用量')}</h2>
        <p className='model-detail-caption'>
          {t(
            '沿用上方分组与时间范围。Token 仅包含已采集的成功请求用量，请求数包含计入健康统计的失败；不是个人账单。',
          )}
        </p>
        {authenticated && (
          <div className='model-compare-usage'>
            <ModelMetricChart
              title={t('Tokens')}
              groups={series}
              metric='total_tokens'
              bar
              t={t}
            />
            <ModelMetricChart
              title={t('请求次数')}
              groups={series}
              metric='request_count'
              bar
              t={t}
            />
          </div>
        )}
        <p className='model-detail-caption'>
          {t('历史缺失数据不补零；新增指标从部署后开始采集。')}
        </p>
      </section>
    </>
  );
}
