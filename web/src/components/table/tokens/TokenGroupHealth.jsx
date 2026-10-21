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
import { Button, Popover, Spin, Typography } from '@douyinfe/semi-ui';
import {
  IconChevronDown,
  IconChevronUp,
  IconInfoCircle,
} from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { renderRatio } from '../../../helpers';
import {
  GroupHealthCollectionNote,
  GroupHealthLegend,
  GroupHealthPanel,
  GroupHealthScopeNote,
  formatHealthLatency,
  formatHealthRate,
  resolveGroupHealthMetric,
} from '../../group-health';
import {
  getHealthLatency,
  healthTone,
  isHealthSampleLimited,
} from '../../group-health/healthUtils';
import './tokenGroupHealth.css';

const { Text } = Typography;

function TokenGroupHealthMetrics({ stats, metric }) {
  const { t } = useTranslation();
  const latency = getHealthLatency(stats, metric);
  return (
    <span className='token-group-health-metrics'>
      <span>
        {t('成功率')}{' '}
        <strong className={`group-health-text-${healthTone(stats)}`}>
          {formatHealthRate(stats?.success_rate)}
        </strong>
      </span>
      <span>
        {metric === 'completion' ? t('平均完成') : t('首字')}{' '}
        <strong>{formatHealthLatency(latency.value)}</strong>
      </span>
      <span title={t('缓存命中率')}>
        {t('缓存')} <strong>{formatHealthRate(stats?.cache_hit_rate)}</strong>
      </span>
      {isHealthSampleLimited(stats) && (
        <span className='group-health-muted'>{t('样本不足')}</span>
      )}
    </span>
  );
}

// Keep health data out of the option value and label: filtering and the saved
// token group must continue using the configured group, even during refreshes.
export const TokenGroupHealthOption = ({
  item,
  stats,
  loading,
  error,
  collectionEnabled = true,
}) => {
  const { t } = useTranslation();
  const { disabled, selected, focused, value, label, ratio } = item;

  return (
    <div
      role='option'
      aria-selected={Boolean(selected)}
      aria-disabled={Boolean(disabled)}
      className={`${item.className || ''} token-group-health-option`}
      style={{
        ...item.style,
        padding: '7px 12px',
        cursor: disabled ? 'not-allowed' : 'pointer',
        opacity: disabled ? 0.5 : 1,
        backgroundColor: selected
          ? 'var(--semi-color-primary-light-default)'
          : focused
            ? 'var(--semi-color-fill-0)'
            : 'transparent',
      }}
      onClick={disabled ? undefined : item.onClick}
      onMouseEnter={disabled ? undefined : item.onMouseEnter}
    >
      <div className='token-group-health-option-heading'>
        <Text strong className='token-group-health-option-name' title={value}>
          {value}
        </Text>
        {ratio !== undefined && ratio !== null && ratio !== '' ? (
          <span className='shrink-0'>{renderRatio(ratio)}</span>
        ) : null}
      </div>
      <div className='token-group-health-option-details'>
        {label && label !== value ? (
          <span className='token-group-health-option-note' title={label}>
            {label}
          </span>
        ) : null}
        {value === 'auto' ? (
          <Text type='tertiary' size='small'>
            {t('自动选择分组，表现按实际调用分组统计')}
          </Text>
        ) : error ? (
          <Text type='tertiary' size='small'>
            {t('健康数据暂不可用')}
          </Text>
        ) : loading ? (
          <Text type='tertiary' size='small'>
            {t('正在加载健康数据')}
          </Text>
        ) : !collectionEnabled ? (
          <Text type='tertiary' size='small'>
            {t('分组健康统计未启用')}
          </Text>
        ) : (
          <TokenGroupHealthMetrics
            stats={stats}
            metric={resolveGroupHealthMetric(stats, value)}
          />
        )}
      </div>
    </div>
  );
};

export const TokenGroupHealth = ({
  group,
  description,
  stats,
  loading,
  error,
  onRetry,
  collectionEnabled = true,
  collectionData,
}) => {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const detailsId = useId();
  const metric = resolveGroupHealthMetric(stats, group);
  const latency = getHealthLatency(stats, metric);

  if (!group) return null;

  return (
    <div className='token-group-health'>
      {group === 'auto' ? (
        <Text type='secondary' size='small'>
          {t('自动选择分组，表现按实际调用分组统计')}
        </Text>
      ) : loading || error ? (
        <div
          className='token-group-health-line'
          role={error ? 'alert' : 'status'}
        >
          {error ? (
            <>
              <Text type='tertiary' size='small'>
                {t('健康数据暂不可用')}
              </Text>
              <Button size='small' theme='borderless' onClick={onRetry}>
                {t('重试')}
              </Button>
            </>
          ) : (
            <>
              <Spin size='small' />
              <Text type='tertiary' size='small'>
                {t('正在加载健康数据')}
              </Text>
            </>
          )}
        </div>
      ) : !collectionEnabled ? (
        <Text type='secondary' size='small'>
          {t('分组健康统计未启用')}
        </Text>
      ) : (
        <>
          <div className='token-group-health-line'>
            <span aria-live='polite'>
              <TokenGroupHealthMetrics stats={stats} metric={metric} />
            </span>
            <div className='token-group-health-actions'>
              <Button
                size='small'
                theme='borderless'
                type='tertiary'
                icon={expanded ? <IconChevronUp /> : <IconChevronDown />}
                aria-expanded={expanded}
                aria-controls={detailsId}
                onClick={() => setExpanded((current) => !current)}
              >
                {expanded ? t('收起') : t('趋势')}
              </Button>
              <Popover
                trigger='click'
                position='bottomRight'
                content={
                  <div className='token-group-health-info'>
                    {description && description !== group ? (
                      <p className='token-group-health-description'>
                        {description}
                      </p>
                    ) : null}
                    <p>
                      {t('近24小时')} · {t('全部模型')}
                    </p>
                    <p>
                      {t('成功 {{success}} · 失败 {{failure}}', {
                        success: stats?.success_count || 0,
                        failure: stats?.failure_count || 0,
                      })}
                      {' · '}
                      {t('{{count}} 个有效样本', { count: latency.count })}
                    </p>
                    <GroupHealthCollectionNote data={collectionData} />
                    <GroupHealthScopeNote />
                    <p className='group-health-note'>
                      {t(
                        '分组内全部模型的调用统计，具体模型表现可在模型广场查看',
                      )}
                    </p>
                  </div>
                }
              >
                <Button
                  size='small'
                  theme='borderless'
                  type={
                    collectionData?.collection_incomplete
                      ? 'warning'
                      : 'tertiary'
                  }
                  icon={<IconInfoCircle />}
                  aria-label={
                    collectionData?.collection_incomplete
                      ? t('部分统计采集延迟或缺失，当前数据仅供参考。')
                      : t('统计说明')
                  }
                />
              </Popover>
            </div>
          </div>
          <div
            id={detailsId}
            hidden={!expanded}
            className='token-group-health-trend'
          >
            {expanded ? (
              <>
                <GroupHealthLegend />
                <GroupHealthPanel
                  stats={stats || { group, series: [] }}
                  metric={metric}
                  showTitle={false}
                />
                <GroupHealthScopeNote />
              </>
            ) : null}
          </div>
        </>
      )}
    </div>
  );
};
