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
  getCatalogTags,
  catalogTagLabel,
} from '../../../../../helpers/modelCatalogFacets';

import { isImagePriceExpression } from '../../../../../helpers/imagePricing';

import React from 'react';
import { isVideoPriceExpression } from '../../../../../helpers/videoPricingTemplate';
import { Link, useNavigate } from 'react-router-dom';
import { buildModelDetailPath } from '../../../../../helpers/modelDetailNavigation';
import { Checkbox, Pagination, Button } from '@douyinfe/semi-ui';
import { Copy, ScanText, Timer, Gauge } from 'lucide-react';
import { getLobeHubIcon, formatTimeRatioValue } from '../../../../../helpers';
import {
  getModelCatalogPricing,
  isCachePriceItem,
} from '../../../../../helpers/modelCatalogPricing';
import PricingCardSkeleton from './PricingCardSkeleton';
import { useMinimumLoadingTime } from '../../../../../hooks/common/useMinimumLoadingTime';
import { useIsMobile } from '../../../../../hooks/common/useIsMobile';

import { useModelHealthSummaries } from '../../../../../hooks/group-health/useModelHealthSummaries';
import {
  formatHealthLatency,
  getHealthLatency,
  formatHealthReference,
} from '../../../../group-health/healthUtils';
import {
  formatModelContext,
  formatModelContextExact,
  formatModelThroughput,
  shouldNavigateModelCard,
} from '../../../../../helpers/modelPresentation';

const PricingCardView = ({
  filteredModels = [],
  loading,
  rowSelection,
  pageSize,
  setPageSize,
  currentPage,
  setCurrentPage,
  selectedGroup,
  groupRatio,
  timeRatioMap,
  copyText,
  currency,
  siteDisplayType,
  tokenUnit,
  displayPrice,
  showRatio,
  t,
  selectedRowKeys = [],
  setSelectedRowKeys,
  catalogState,
}) => {
  const showSkeleton = useMinimumLoadingTime(loading);
  const isMobile = useIsMobile();
  const navigate = useNavigate();
  const paginatedModels = filteredModels.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );

  const health = useModelHealthSummaries(paginatedModels, !loading);
  if (showSkeleton) return <PricingCardSkeleton />;
  return (
    <div>
      <div className='pricing-catalog-grid'>
        {paginatedModels.map((model) => {
          const key = model.key ?? model.model_name ?? model.id;
          const selected = selectedRowKeys.includes(key);
          const { data, items } = getModelCatalogPricing(
            {
              record: model,
              selectedGroup,
              groupRatio,
              timeRatioMap,
              tokenUnit,
              displayPrice,
              currency,
              quotaDisplayType: siteDisplayType,
            },
            t,
          );
          const cardPriceItems = items.filter(
            (item) => !isCachePriceItem(item) && item.key !== 'time-ratio',
          );
          const tags = getCatalogTags(model);
          const icon = model.icon || model.vendor_icon;
          const stats = health.summaries.get(model.model_name);
          const latency = getHealthLatency(stats, 'ttft', true);
          const showTokenMetrics =
            data.isPerToken ||
            (data.isDynamicPricing &&
              !isVideoPriceExpression(model.billing_expr) &&
              !isImagePriceExpression(model.billing_expr));
          return (
            <article
              key={key}
              className={`pricing-model-card${selected ? ' is-selected' : ''}`}
              aria-label={model.display_name || model.model_name}
              onClick={(event) => {
                if (
                  !shouldNavigateModelCard(
                    event,
                    window.getSelection()?.toString(),
                  )
                )
                  return;
                const path = buildModelDetailPath(model.model_name);
                if (event.metaKey || event.ctrlKey || event.shiftKey)
                  window.open(path, '_blank', 'noopener,noreferrer');
                else navigate(path, { state: { catalogState } });
              }}
            >
              <div className='pricing-model-card-header'>
                <div className='pricing-model-icon' aria-hidden='true'>
                  {icon
                    ? getLobeHubIcon(icon, 24, model.vendor_icon)
                    : model.model_name.slice(0, 2).toUpperCase()}
                </div>
                <div className='pricing-model-identity'>
                  <h2>
                    <Link
                      to={buildModelDetailPath(model.model_name)}
                      state={{ catalogState }}
                    >
                      {model.display_name || model.model_name}
                    </Link>
                  </h2>
                </div>
                <Button
                  size='small'
                  theme='borderless'
                  type='tertiary'
                  icon={<Copy size={15} />}
                  aria-label={t('复制模型名称') + ': ' + model.model_name}
                  title={t('复制模型名称')}
                  onClick={() => copyText(model.model_name)}
                />
                {rowSelection && (
                  <Checkbox
                    aria-label={t('选择模型') + ': ' + model.model_name}
                    checked={selected}
                    onChange={(event) => {
                      const keys = event.target.checked
                        ? [...new Set([...selectedRowKeys, key])]
                        : selectedRowKeys.filter((item) => item !== key);
                      setSelectedRowKeys?.(keys);
                      rowSelection.onChange?.(keys, null);
                    }}
                  />
                )}
              </div>
              <dl className='pricing-card-prices'>
                {cardPriceItems.length ? (
                  cardPriceItems.map((item) => (
                    <div key={item.key}>
                      <dt>{item.label}</dt>
                      <dd>
                        <strong>{item.value}</strong>
                        <span>{item.suffix}</span>
                      </dd>
                    </div>
                  ))
                ) : (
                  <div>
                    <dt>{t('动态计费')}</dt>
                    <dd>{t('见价格详情')}</dd>
                  </div>
                )}
              </dl>
              {showTokenMetrics && (
                <dl
                  className='pricing-card-performance'
                  title={`${t('全部分组')} · ${t('最近 {{hours}} 小时', { hours: 24 })} · ${t('平台分组统计，不限于个人请求。')}`}
                >
                  <div>
                    <dt>
                      <ScanText size={13} />
                      {t('上下文长度')}
                    </dt>
                    <dd title={formatModelContextExact(model.context_length)}>
                      {formatModelContext(model.context_length) === '—'
                        ? t('未配置')
                        : formatModelContext(model.context_length)}
                    </dd>
                  </div>
                  {health.state === 'ready' ? (
                    <>
                      <div>
                        <dt>
                          <Timer size={13} />
                          {t('首字均值')}
                        </dt>
                        <dd title={formatHealthReference(latency, t)}>
                          {formatHealthLatency(latency.value) === '—'
                            ? t('暂无数据')
                            : formatHealthLatency(latency.value)}
                          {latency.referenceTs && (
                            <small> · {t('沿用最近有效值')}</small>
                          )}
                        </dd>
                      </div>
                      <div>
                        <dt>
                          <Gauge size={13} />
                          {t('输出速度')}
                        </dt>
                        <dd>
                          {formatModelThroughput(stats?.avg_output_tps) === '—'
                            ? t('暂无数据')
                            : formatModelThroughput(stats?.avg_output_tps)}
                        </dd>
                      </div>
                    </>
                  ) : (
                    <div className='pricing-card-health-status'>
                      <dt>{t('性能对比')}</dt>
                      <dd>
                        {health.state === 'anonymous' ? (
                          <Link
                            to={`/login?next=${encodeURIComponent(buildModelDetailPath(model.model_name))}`}
                          >
                            {t('登录后查看可用分组的调用表现')}
                          </Link>
                        ) : health.state === 'error' ? (
                          <button type='button' onClick={health.retry}>
                            {t('加载失败，请重试')}
                          </button>
                        ) : (
                          t('加载中')
                        )}
                      </dd>
                    </div>
                  )}
                </dl>
              )}
              <p
                className='pricing-model-description'
                aria-hidden={!model.description || undefined}
              >
                {model.description || ''}
              </p>
              <div className='pricing-model-card-footer'>
                <div className='pricing-card-notes'>
                  {showRatio && (
                    <div className='pricing-card-ratios'>
                      {model.quota_type === 0 && (
                        <span>
                          {t('模型倍率')}: {model.model_ratio ?? '—'} ·{' '}
                          {t('补全倍率')}: {model.completion_ratio ?? '—'}
                        </span>
                      )}
                      <span>
                        {t('分组倍率')}: {data.usedGroupRatio}× ·{' '}
                        {t('时间倍率')}: {formatTimeRatioValue(data.timeRatio)}×
                      </span>
                    </div>
                  )}
                </div>
                <div className='pricing-model-tags'>
                  {tags.slice(0, 3).map((tag) => (
                    <span key={tag}>{catalogTagLabel(tag, t, [model])}</span>
                  ))}
                </div>
              </div>
            </article>
          );
        })}
      </div>
      {filteredModels.length > 0 && (
        <div className='pricing-catalog-pagination'>
          <Pagination
            currentPage={currentPage}
            pageSize={pageSize}
            total={filteredModels.length}
            showSizeChanger
            pageSizeOptions={[10, 20, 50, 100]}
            size={isMobile ? 'small' : 'default'}
            onPageChange={setCurrentPage}
            onPageSizeChange={(size) => {
              setPageSize(size);
              setCurrentPage(1);
            }}
          />
        </div>
      )}
    </div>
  );
};
export default PricingCardView;
