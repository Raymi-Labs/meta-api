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
  getHealthCounts,
  getHealthLatency,
  healthTone,
  getUptimeRate,
  uptimeTone,
} from './healthUtils';
import './style.css';

export {
  formatHealthLatency,
  formatHealthRate,
  resolveGroupHealthMetric,
  selectHealthGroups,
} from './healthUtils';

export function formatHealthOutcomeDescription(stats, t) {
  const counts = getHealthCounts(stats);
  return [
    t('已观测 {{observed}} 次 · 计入统计 {{included}} 次', {
      observed: counts.observed,
      included: counts.request,
    }),
    t('成功 {{success}} · 服务失败 {{service}} · 未归因失败 {{unknown}}', {
      success: counts.success,
      service: counts.service,
      unknown: counts.unknown,
    }),
    counts.disconnected > 0 &&
      t('客户端断开 {{count}} 次（包含在未归因失败中）', {
        count: counts.disconnected,
      }),
    counts.excluded > 0 &&
      t('已排除用户错误 {{count}} 次', { count: counts.excludedUser }),
  ]
    .filter(Boolean)
    .join(' · ');
}

export function GroupHealthCountNotice({ stats, uptime = false }) {
  const { t } = useTranslation();
  const counts = getHealthCounts(stats);
  const notices = [
    uptime &&
      stats?.probe_count > 0 &&
      t('上游测试 {{success}} / {{count}} 次通过', {
        success: stats.probe_success_count,
        count: stats.probe_count,
      }),
    uptime &&
      stats?.probe_bucket_count > 0 &&
      t('测试补充 {{count}} 个时段', { count: stats.probe_bucket_count }),
    counts.unknown > 0 && t('未归因 {{count}} 次', { count: counts.unknown }),
    counts.excluded > 0 && t('已排除 {{count}} 次', { count: counts.excluded }),
  ].filter(Boolean);
  return notices.length ? (
    <span className='group-health-count-notice group-health-muted'>
      {notices.join(' · ')}
    </span>
  ) : null;
}

export function GroupHealthOutcomeDetails({ stats }) {
  const { t } = useTranslation();
  return (
    <div className='group-health-caption'>
      {formatHealthOutcomeDescription(stats, t)}
      <p className='group-health-note'>
        {t(
          '成功率为成功次数除以计入统计的次数；服务失败和未归因失败均计入，明确的用户错误排除。',
        )}{' '}
        {t(
          '历史未分类失败归入未归因失败；客户端断开包含在未归因失败中，不能视为主动取消。',
        )}
      </p>
    </div>
  );
}

const emptyHealthLabel = (stats, t) =>
  getHealthCounts(stats).observed > 0 ? t('无有效统计样本') : t('暂无调用');

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
      {t('统计覆盖当前小时及此前 23 个完整小时，当前小时尚未结束。')}{' '}
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
  uptime = false,
}) {
  const { t } = useTranslation();
  const latency = getHealthLatency(stats, metric);
  const counts = getHealthCounts(stats);
  const hasSamples =
    counts.request > 0 || (uptime && stats?.probe_bucket_count > 0);
  const tone = uptime ? uptimeTone(stats) : healthTone(stats);
  const rate = uptime ? getUptimeRate(stats) : stats?.success_rate;
  const limited = tone === 'limited';
  if (compact)
    return (
      <span
        className='group-health-inline'
        title={formatHealthOutcomeDescription(stats, t)}
      >
        {!hasSamples ? (
          emptyHealthLabel(stats, t)
        ) : (
          <>
            <span>
              {uptime ? t('可用率') : t('成功率')}{' '}
              <strong className={`group-health-text-${tone}`}>
                {formatHealthRate(rate)}
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
        <GroupHealthCountNotice stats={stats} uptime={uptime} />
      </span>
    );
  return (
    <div className='group-health-metrics'>
      <div>
        <span className='group-health-label'>
          {uptime ? t('可用率') : t('成功率')}
        </span>
        <strong className={`group-health-number group-health-text-${tone}`}>
          {formatHealthRate(rate)}
        </strong>
        <span
          className='group-health-caption'
          title={formatHealthOutcomeDescription(stats, t)}
        >
          {counts.request > 0
            ? t('计入统计 {{count}} 次', { count: counts.request })
            : uptime && stats?.probe_bucket_count > 0
              ? t('上游测试')
              : emptyHealthLabel(stats, t)}
          {limited ? ` · ${t('样本不足')}` : ''}
        </span>
        <GroupHealthCountNotice stats={stats} uptime={uptime} />
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
  uptime = false,
}) {
  const { t } = useTranslation();
  const detailsId = useId();
  const [selectedTs, setSelectedTs] = useState(null);
  const series = Array.isArray(stats?.series) ? stats.series : [];
  const selected = series.find((bucket) => bucket.ts === selectedTs);
  const resolveTone = uptime ? uptimeTone : healthTone;
  const resolveRate = uptime ? getUptimeRate : (value) => value?.success_rate;
  const tone = resolveTone(stats);
  const labels = {
    limited: t('样本不足'),
    healthy: t('运行稳定'),
    warning: t('部分波动'),
    degraded: t('失败较多'),
  };
  const statusLabel = (value) =>
    resolveTone(value) === 'empty'
      ? emptyHealthLabel(value, t)
      : uptime && !value?.request_count && value?.probe_bucket_count > 0
        ? t('上游测试')
        : labels[resolveTone(value)];
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
              {statusLabel(stats)}
            </span>
          </div>
        </div>
      )}
      <GroupHealthSummary stats={stats} metric={metric} uptime={uptime} />
      {uptime && <GroupUptimeNote />}
      <details className='group-health-outcomes'>
        <summary>{t('统计明细')}</summary>
        <GroupHealthOutcomeDetails stats={stats} />
      </details>
      <div className='group-health-trend-heading'>
        <span>
          {uptime
            ? t('最近 {{hours}} 个小时段可用率', { hours: series.length || 24 })
            : t('最近 {{hours}} 个小时段成功率', {
                hours: series.length || 24,
              })}
        </span>
        <span>{t('点击时段查看详情')}</span>
      </div>
      <div className='group-health-bars' aria-label={t('每格代表一小时')}>
        {series.map((bucket) => (
          <button
            key={bucket.ts}
            type='button'
            className={`group-health-bar group-health-bar-${resolveTone(bucket)}`}
            aria-label={`${dateLabel(bucket.ts)} · ${uptime ? t('可用率') : t('成功率')} ${formatHealthRate(resolveRate(bucket))} · ${statusLabel(bucket)} · ${formatHealthOutcomeDescription(bucket, t)}`}
            title={`${dateLabel(bucket.ts)} · ${formatHealthRate(resolveRate(bucket))} · ${statusLabel(bucket)} · ${formatHealthOutcomeDescription(bucket, t)}`}
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
            <span>{statusLabel(selected)}</span>
          </div>
          <GroupHealthSummary
            stats={selected}
            metric={metric}
            compact
            uptime={uptime}
          />
          <GroupHealthOutcomeDetails stats={selected} />
          <div className='group-health-caption'>
            {t('{{count}} 个有效样本', {
              count: getHealthLatency(selected, metric).count,
            })}
            {selected.ts === series[series.length - 1]?.ts
              ? ` · ${t('当前小时尚未结束')}`
              : ''}
          </div>
        </div>
      )}
      {!getHealthCounts(stats).observed &&
        !(uptime && stats?.probe_count > 0) && (
          <p className='group-health-note'>
            {t('该时段暂无调用记录，产生新请求后显示统计。')}
          </p>
        )}
    </section>
  );
}

export function GroupHealthLegend({ uptime = false }) {
  const { t } = useTranslation();
  return (
    <>
      <div className='group-health-legend'>
        <span>
          <i className='group-health-bar-healthy' />
          {uptime
            ? `Uptime ≥ ${HEALTH_HEALTHY_RATE}%`
            : t('成功率 ≥ {{rate}}%', { rate: HEALTH_HEALTHY_RATE })}
        </span>
        <span>
          <i className='group-health-bar-warning' />
          {uptime
            ? `${HEALTH_WARNING_RATE}% ≤ Uptime < ${HEALTH_HEALTHY_RATE}%`
            : t('{{min}}% ≤ 成功率 < {{max}}%', {
                min: HEALTH_WARNING_RATE,
                max: HEALTH_HEALTHY_RATE,
              })}
        </span>
        <span>
          <i className='group-health-bar-degraded' />
          {uptime
            ? `Uptime < ${HEALTH_WARNING_RATE}%`
            : t('成功率 < {{rate}}%', { rate: HEALTH_WARNING_RATE })}
        </span>
        <span>
          <i className='group-health-bar-empty' />
          {t('样本不足或无有效统计样本')}
        </span>
      </div>
      <div className='group-health-note'>
        {uptime ? (
          <GroupUptimeNote />
        ) : (
          t(
            '计入统计少于 {{count}} 次时显示灰色；达到 {{count}} 次后按实际成功率着色。',
            {
              count: HEALTH_MIN_SAMPLES,
            },
          )
        )}
      </div>
    </>
  );
}

export function GroupUptimeNote() {
  const { t } = useTranslation();
  return (
    <p className='group-health-note'>
      {t(
        '有效调用不足30次时，按对应上游渠道的测试补充：任一渠道通过即成功，测试可使用同渠道的其他模型；仅有失败即失败。每小时最多补一个样本，真实调用保留权重，无测试沿用样本门槛。历史成功测试按小时补证据，不代表当前模型实测或持续在线。',
      )}
    </p>
  );
}
