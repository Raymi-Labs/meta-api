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
import { Link } from 'react-router-dom';
import { Select } from '@douyinfe/semi-ui';
import {
  GroupHealthStatus,
  GroupHealthCollectionNote,
  GroupHealthLegend,
  selectHealthGroups,
} from '../../components/group-health';
import { selectComparisonGroups } from '../../helpers/modelPresentation';
import ModelMetricChart from './ModelMetricChart';

export default function ModelAnalytics({
  model,
  data,
  health,
  hours,
  setHours,
}) {
  const { t } = data;
  const [selection, setSelection] = useState(null);
  const [usageMetric, setUsageMetric] = useState('total_tokens');
  const available = selectHealthGroups({
    groups: health.data?.groups || [],
    models: [model],
    usableGroup: data.usableGroup,
    exactModel: true,
  });
  const names = available.map((item) => item.group);
  const selected = selectComparisonGroups(
    names,
    selection ??
      [...available]
        .sort((a, b) => b.request_count - a.request_count)
        .slice(0, 4)
        .map((item) => item.group),
  );
  const groups = selected.map((group) =>
    available.find((item) => item.group === group),
  );
  const ready =
    health.isAuthenticated &&
    !health.error &&
    Boolean(health.data) &&
    health.data?.enabled !== false;
  return (
    <>
      <section id='performance' className='model-detail-section'>
        <div className='model-detail-section-heading'>
          <h2>{t('性能对比')}</h2>
          <Select
            aria-label={t('时间范围')}
            value={hours}
            onChange={setHours}
            optionList={[24, 72].map((value) => ({
              value,
              label: t('最近 {{hours}} 小时', { hours: value }),
            }))}
          />
        </div>
        <div className='model-comparison-controls'>
          <label>
            {t('对比分组')}
            <Select
              multiple
              filter
              maxTagCount={2}
              max={4}
              aria-label={t('对比分组')}
              placeholder={t('请选择分组')}
              value={selected}
              onChange={(values) =>
                setSelection(selectComparisonGroups(names, values))
              }
              optionList={names.map((name) => ({ value: name, label: name }))}
            />
          </label>
          <span>{t('最多同时对比 4 个分组；价格与健康记录可逐组查看。')}</span>
        </div>
        <GroupHealthStatus
          loading={health.loading}
          error={health.error}
          onRetry={health.refresh}
        />
        <GroupHealthCollectionNote data={health.data} />
        {!health.isAuthenticated && (
          <p className='model-detail-caption'>
            <Link to='/login'>{t('登录后查看可用分组的调用表现')}</Link>
          </p>
        )}
        {health.data?.enabled === false && (
          <p className='model-detail-caption'>
            {t('管理员暂未开启分组健康展示')}
          </p>
        )}
        {ready && (
          <>
            <div className='model-performance-grid'>
              <ModelMetricChart
                title={'Uptime · ' + t('可用率')}
                groups={groups}
                metric='uptime_rate'
                t={t}
              />
              <ModelMetricChart
                title={t('首字均值') + ' · s'}
                groups={groups}
                metric='avg_ttft_ms'
                t={t}
              />
              <ModelMetricChart
                title={t('输出速度') + ' · tok/s'}
                groups={groups}
                metric='avg_output_tps'
                t={t}
              />
            </div>
            {groups.some((group) => group.completion_count > 0) && (
              <ModelMetricChart
                title={t('平均完成') + ' · s'}
                groups={groups}
                metric='avg_completion_ms'
                t={t}
              />
            )}
            <GroupHealthLegend uptime />
          </>
        )}
      </section>
      <section id='usage' className='model-detail-section'>
        <div className='model-detail-section-heading'>
          <h2>{t('分组用量')}</h2>
          <div
            className='model-usage-switch'
            role='group'
            aria-label={t('用量类型')}
          >
            {[
              { value: 'total_tokens', label: 'Tokens' },
              { value: 'request_count', label: t('请求次数') },
            ].map(({ value, label }) => (
              <button
                key={value}
                type='button'
                aria-pressed={usageMetric === value}
                onClick={() => setUsageMetric(value)}
              >
                {label}
              </button>
            ))}
          </div>
        </div>
        {ready ? (
          <ModelMetricChart
            title={usageMetric === 'total_tokens' ? 'Tokens' : t('请求次数')}
            groups={groups}
            metric={usageMetric}
            bar
            t={t}
          />
        ) : (
          <p className='model-detail-caption'>{t('请先加载分组调用表现')}</p>
        )}
      </section>
    </>
  );
}
