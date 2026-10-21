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

import React, { useId, useState } from 'react';
import { Button, Spin, Tag } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  HEALTH_MIN_SAMPLES,
  HEALTH_HEALTHY_RATE,
  HEALTH_WARNING_RATE,
  formatHealthLatency,
  formatHealthRate,
  getHealthLatency,
  healthTone,
  isHealthSampleLimited,
} from './healthUtils';
import './style.css';

export {
  formatHealthLatency,
  formatHealthRate,
  resolveGroupHealthMetric,
  selectHealthGroups,
} from './healthUtils';

const timeLabel = (ts) =>
  new Date(ts * 1000).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
  });
const dateLabel = (ts) =>
  new Date(ts * 1000).toLocaleString([], {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });

export function GroupHealthStatus({ loading, error, onRetry }) {
  const { t } = useTranslation();
  if (error)
    return (
      <div className='group-health-state' role='alert'>
        <span>{t('健康数据暂时无法加载')}</span>
        <Button size='small' theme='borderless' onClick={() => onRetry?.()}>
          {t('重试')}
        </Button>
      </div>
    );
  if (loading)
    return (
      <div className='group-health-state' role='status'>
        <Spin size='small' />
        <span>{t('正在加载调用表现')}</span>
      </div>
    );
  return null;
}

export function GroupHealthScopeNote() {
  const { t } = useTranslation();
  return (
    <p className='group-health-note'>
      {t('平台分组统计，不限于个人请求。')}{' '}
      {t(
        '同组重试合并统计；跨组重试分别记录各组结果。首字均值仅统计成功流式请求，含排队和重试等待。',
      )}{' '}
      {t('生图显示成功同步请求的完成耗时，异步提交不计入完成样本。')}{' '}
      {t(
        '缓存命中率为成功请求的缓存读取 Token 总量除以输入 Token 总量，包含未命中请求，缓存写入不算命中；无有效用量时显示 —。',
      )}
    </p>
  );
}

export function GroupHealthCollectionNote({ data }) {
  const { t } = useTranslation();
  if (!data?.collection_incomplete) return null;
  return (
    <p
      className='group-health-note group-health-collection-warning'
      role='status'
    >
      {t('部分统计采集延迟或缺失，当前数据仅供参考。')}
    </p>
  );
}

export function GroupHealthSummary({
  stats,
  metric = 'ttft',
  compact = false,
}) {
  const { t } = useTranslation();
  const latency = getHealthLatency(stats, metric);
  const hasSamples = Number(stats?.request_count) > 0;
  const limited = isHealthSampleLimited(stats);
  if (compact)
    return (
      <span className='group-health-inline'>
        {!hasSamples ? (
          t('暂无样本')
        ) : (
          <>
            <span>
              {t('成功率')}{' '}
              <strong className={`group-health-text-${healthTone(stats)}`}>
                {formatHealthRate(stats.success_rate)}
              </strong>
            </span>
            <span>
              {metric === 'completion' ? t('平均完成') : t('首字均值')}{' '}
              <strong>{formatHealthLatency(latency.value)}</strong>
            </span>
            <span title={t('缓存命中率')}>
              {t('缓存')}{' '}
              <strong>{formatHealthRate(stats.cache_hit_rate)}</strong>
            </span>
            {(limited ||
              (latency.count > 0 && latency.count < HEALTH_MIN_SAMPLES)) && (
              <span className='group-health-muted'>{t('样本不足')}</span>
            )}
          </>
        )}
      </span>
    );
  return (
    <div className='group-health-metrics'>
      <div>
        <span className='group-health-label'>{t('成功率')}</span>
        <strong
          className={`group-health-number group-health-text-${healthTone(stats)}`}
        >
          {formatHealthRate(stats?.success_rate)}
        </strong>
        <span className='group-health-caption'>
          {hasSamples
            ? t('{{count}} 次调用', { count: stats.request_count })
            : t('暂无样本')}
          {limited ? ` · ${t('样本不足')}` : ''}
        </span>
      </div>
      <div>
        <span className='group-health-label'>
          {metric === 'completion' ? t('平均完成耗时') : t('平均首字延迟')}
        </span>
        <strong className='group-health-number'>
          {formatHealthLatency(latency.value)}
        </strong>
        <span className='group-health-caption'>
          {t('{{count}} 个有效样本', { count: latency.count })}
          {latency.count > 0 && latency.count < HEALTH_MIN_SAMPLES
            ? ` · ${t('样本不足')}`
            : ''}
        </span>
      </div>
      <div>
        <span className='group-health-label'>{t('缓存命中率')}</span>
        <strong className='group-health-number'>
          {formatHealthRate(stats?.cache_hit_rate)}
        </strong>
        <span className='group-health-caption'>
          {t('{{count}} 个有效样本', { count: stats?.cache_sample_count || 0 })}
        </span>
      </div>
    </div>
  );
}

export function GroupHealthPanel({
  stats,
  metric = 'ttft',
  showTitle = true,
  ratio,
}) {
  const { t } = useTranslation();
  const detailsId = useId();
  const [selectedTs, setSelectedTs] = useState(null);
  const series = Array.isArray(stats?.series) ? stats.series : [];
  const selected = series.find((bucket) => bucket.ts === selectedTs);
  const tone = healthTone(stats);
  const labels = {
    empty: t('暂无样本'),
    healthy: t('运行稳定'),
    warning: t('部分波动'),
    degraded: t('失败较多'),
  };
  const statusDescription = (value) =>
    `${labels[healthTone(value)]}${isHealthSampleLimited(value) ? ` · ${t('样本不足')}` : ''}`;
  return (
    <section className='group-health-panel'>
      {showTitle && (
        <div className='group-health-panel-header'>
          <strong className='group-health-name'>
            {stats?.group || t('分组')}
          </strong>
          <div className='group-health-tags'>
            {ratio != null && (
              <Tag color='blue' size='small'>
                {ratio}x
              </Tag>
            )}
            <span className={`group-health-badge group-health-badge-${tone}`}>
              {labels[tone]}
            </span>
            {isHealthSampleLimited(stats) && (
              <span className='group-health-badge group-health-badge-limited'>
                {t('样本不足')}
              </span>
            )}
          </div>
        </div>
      )}
      <GroupHealthSummary stats={stats} metric={metric} />
      <div className='group-health-trend-heading'>
        <span>{t('近 24 小时成功率')}</span>
        <span>{t('点击时段查看详情')}</span>
      </div>
      <div className='group-health-bars' aria-label={t('每格代表一小时')}>
        {series.map((bucket) => (
          <button
            key={bucket.ts}
            type='button'
            className={`group-health-bar group-health-bar-${healthTone(bucket)}`}
            aria-label={`${dateLabel(bucket.ts)} · ${t('成功率')} ${formatHealthRate(bucket.success_rate)} · ${t('{{count}} 次调用', { count: bucket.request_count || 0 })} · ${statusDescription(bucket)}`}
            title={`${dateLabel(bucket.ts)} · ${formatHealthRate(bucket.success_rate)} · ${statusDescription(bucket)}`}
            aria-pressed={selectedTs === bucket.ts}
            aria-controls={detailsId}
            onClick={() =>
              setSelectedTs(selectedTs === bucket.ts ? null : bucket.ts)
            }
          />
        ))}
      </div>
      {series.length > 0 && (
        <div className='group-health-axis'>
          <span>{dateLabel(series[0].ts)}</span>
          <span>{t('当前小时')}</span>
        </div>
      )}
      {selected && (
        <div className='group-health-bucket' id={detailsId} role='status'>
          <div className='group-health-bucket-header'>
            <strong>
              {dateLabel(selected.ts)}–{timeLabel(selected.ts + 3600)}
            </strong>
            <span>{labels[healthTone(selected)]}</span>
          </div>
          <GroupHealthSummary stats={selected} metric={metric} compact />
          <div className='group-health-caption'>
            {t('成功 {{success}} · 失败 {{failure}}', {
              success: selected.success_count || 0,
              failure: selected.failure_count || 0,
            })}{' '}
            ·{' '}
            {t('{{count}} 个有效样本', {
              count: getHealthLatency(selected, metric).count,
            })}
            {selected.ts === series[series.length - 1]?.ts
              ? ` · ${t('当前小时尚未结束')}`
              : ''}
          </div>
        </div>
      )}
      {!Number(stats?.request_count) && (
        <p className='group-health-note'>
          {t('该时段暂无调用记录，产生新请求后显示统计。')}
        </p>
      )}
    </section>
  );
}

export function GroupHealthLegend() {
  const { t } = useTranslation();
  return (
    <>
      <div className='group-health-legend'>
        <span>
          <i className='group-health-bar-healthy' />
          {t('成功率 ≥ {{rate}}%', { rate: HEALTH_HEALTHY_RATE })}
        </span>
        <span>
          <i className='group-health-bar-warning' />
          {t('{{min}}% ≤ 成功率 < {{max}}%', {
            min: HEALTH_WARNING_RATE,
            max: HEALTH_HEALTHY_RATE,
          })}
        </span>
        <span>
          <i className='group-health-bar-degraded' />
          {t('成功率 < {{rate}}%', { rate: HEALTH_WARNING_RATE })}
        </span>
        <span>
          <i className='group-health-bar-empty' />
          {t('暂无样本')}
        </span>
      </div>
      <p className='group-health-note'>
        {t('少于 {{count}} 次调用时提示样本不足，颜色仍按实际成功率显示。', {
          count: HEALTH_MIN_SAMPLES,
        })}
      </p>
    </>
  );
}
