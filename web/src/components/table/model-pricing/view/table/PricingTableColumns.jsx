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

import {
  isImagePriceExpression,
} from '../../../../../helpers/imagePricing';

import React from 'react';
import { isVideoPriceExpression } from '../../../../../helpers/videoPricingTemplate';
import { Link } from 'react-router-dom';
import { buildModelDetailPath } from '../../../../../helpers/modelDetailNavigation';
import { Button } from '@douyinfe/semi-ui';
import { IconCopy } from '@douyinfe/semi-icons';
import { formatTimeRatioValue } from '../../../../../helpers';
import { getModelCatalogPricing } from '../../../../../helpers/modelCatalogPricing';
import ModelProtocolSummary from '../../ModelProtocolSummary';

export const getPricingTableColumns = ({
  t,
  selectedGroup,
  groupRatio,
  timeRatioMap,
  copyText,
  currency,
  tokenUnit,
  displayPrice,
  showRatio,
  siteDisplayType,
  catalogState,
}) => {
  const cache = new WeakMap();
  const pricing = (record) => {
    if (!cache.has(record)) {
      cache.set(
        record,
        getModelCatalogPricing(
          {
            record,
            selectedGroup,
            groupRatio,
            timeRatioMap,
            tokenUnit,
            displayPrice,
            currency,
            quotaDisplayType: siteDisplayType,
          },
          t,
        ),
      );
    }
    return cache.get(record);
  };
  const renderPrice = (record, keys, includeFixed = false) => {
    const { data, items } = pricing(record);
    const item = items.find(
      (item) =>
        keys.includes(item.key) ||
        (includeFixed &&
          ['fixed', 'audio-duration', 'video-second'].includes(item.key)),
    );
    if (!item) return data.isDynamicPricing ? t('见价格详情') : '—';
    return (
      <div className='whitespace-nowrap'>
        <div>{item.value}</div>
        <div className='pricing-group-description'>{item.suffix}</div>
      </div>
    );
  };
  const columns = [
    {
      title: t('模型名称'),
      dataIndex: 'model_name',
      render: (name, record) => (
        <div>
          <div className='flex items-center gap-1'>
            <Link
              className='font-medium'
              to={buildModelDetailPath(name)}
              state={{ catalogState }}
              onClick={(event) => event.stopPropagation()}
            >
              {name}
            </Link>
            <Button
              theme='borderless'
              size='small'
              icon={<IconCopy />}
              aria-label={t('复制模型名称') + ': ' + name}
              onClick={(event) => {
                event.stopPropagation();
                copyText(name);
              }}
            />
          </div>
          <ModelProtocolSummary
            model={record}
            catalogState={catalogState}
            t={t}
          />
          <div className='pricing-group-description'>
            {record.vendor_name} ·{' '}
            {record.billing_mode === 'tiered_expr'
              ? t(
                  isImagePriceExpression(record.billing_expr)
                    ? t('图片输入 / 输出计费')
                    : isVideoPriceExpression(record.billing_expr)
                      ? '视频按秒 / 表达式'
                      : '阶梯计费：展示首档参考价，完整规则见详情',
                )
              : record.quota_type === 0
                ? t('按量计费')
                : record.quota_type === 2
                  ? t('按小时计费')
                  : t('按次计费')}
          </div>
        </div>
      ),
    },
    {
      title: t('输入 / 单价'),
      dataIndex: 'input_price',
      render: (_, record) =>
        renderPrice(record, ['input', 'input-ratio', 'tiered-p'], true),
    },
    {
      title: t('输出'),
      dataIndex: 'output_price',
      render: (_, record) =>
        renderPrice(record, ['completion', 'completion-ratio', 'tiered-c']),
    },
    {
      title: t('缓存读取'),
      dataIndex: 'cache_price',
      render: (_, record) =>
        renderPrice(record, ['cache', 'cache-ratio', 'tiered-cr']),
    },
  ];
  if (showRatio)
    columns.push({
      title: t('倍率'),
      dataIndex: 'ratio',
      render: (_, record) => {
        const { data } = pricing(record);
        return (
          <div>
            {record.quota_type === 0 && (
              <>
                <div>
                  {t('模型倍率')}：{record.model_ratio ?? '—'}
                </div>
                <div>
                  {t('补全倍率')}：{record.completion_ratio ?? '—'}
                </div>
              </>
            )}
            <div>
              {t('分组倍率')}：{data.usedGroupRatio}×
            </div>
            <div>
              {t('时间倍率')}：{formatTimeRatioValue(data.timeRatio)}×
            </div>
          </div>
        );
      },
    });
  columns.push({
    title: t('详情'),
    dataIndex: 'details',
    render: (_, record) => (
      <Link
        to={buildModelDetailPath(record.model_name)}
        state={{ catalogState }}
        onClick={(event) => event.stopPropagation()}
      >
        {t('模型详情')}
      </Link>
    ),
  });
  return columns;
};
