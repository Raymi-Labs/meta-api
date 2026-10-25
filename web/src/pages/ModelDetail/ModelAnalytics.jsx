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
        <p className='model-detail-caption'>
          {t('平台分组统计，不限于个人请求。')} {t('当前小时尚未结束')} ·{' '}
          {t('每分钟更新')}
        </p>
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
                title={'Uptime · ' + t('成功率')}
                groups={groups}
                metric='success_rate'
                unit='%'
                t={t}
              />
              <ModelMetricChart
                title={t('首字均值')}
                groups={groups}
                metric='avg_ttft_ms'
                unit=' ms'
                t={t}
              />
              <ModelMetricChart
                title={t('输出速度')}
                groups={groups}
                metric='avg_output_tps'
                unit=' tok/s'
                t={t}
              />
            </div>
            {groups.some((group) => group.completion_count > 0) && (
              <ModelMetricChart
                title={t('平均完成')}
                groups={groups}
                metric='avg_completion_ms'
                unit=' ms'
                t={t}
              />
            )}
            <GroupHealthLegend />
          </>
        )}
        <p className='model-detail-caption'>
          {t(
            '成功率为成功次数除以计入统计的次数；服务失败和未归因失败均计入，明确的用户错误排除。',
          )}{' '}
          {t(
            '同组重试合并统计；跨组重试分别记录各组结果。首字均值仅统计成功流式请求，含排队和重试等待。',
          )}
        </p>
        <p className='model-detail-caption'>
          {t(
            '输出速度按成功流式请求的输出 Token 总量除以首个有效输出至完成的总耗时计算；无有效样本时显示 —。',
          )}
        </p>
      </section>
      <section id='usage' className='model-detail-section'>
        <div className='model-detail-section-heading'>
          <h2>{t('分组用量')}</h2>
          <Select
            aria-label={t('用量类型')}
            value={usageMetric}
            onChange={setUsageMetric}
            optionList={[
              { value: 'total_tokens', label: 'Tokens' },
              { value: 'request_count', label: t('请求次数') },
            ]}
          />
        </div>
        <p className='model-detail-caption'>
          {t(
            '沿用上方分组与时间范围。Token 仅包含已采集的成功请求用量，请求数包含计入健康统计的失败；不是个人账单。',
          )}
        </p>
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
        <p className='model-detail-caption'>
          {t('历史缺失数据不补零；新增指标从部署后开始采集。')}
          {ready &&
            groups.map((group) => (
              <span className='model-sample-note' key={group.group}>
                {group.group} ·{' '}
                {t('{{count}} 个有效样本', {
                  count: group.usage_sample_count || 0,
                })}
              </span>
            ))}
        </p>
      </section>
    </>
  );
}
