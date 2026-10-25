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
import { Link, useNavigate } from 'react-router-dom';
import { buildModelDetailPath } from '../../../../../helpers/modelDetailNavigation';
import { Checkbox, Pagination, Button } from '@douyinfe/semi-ui';
import { Copy, ScanText, Timer, Gauge } from 'lucide-react';
import { getLobeHubIcon, formatTimeRatioValue } from '../../../../../helpers';
import { getModelCatalogPricing } from '../../../../../helpers/modelCatalogPricing';
import PricingCardSkeleton from './PricingCardSkeleton';
import { useMinimumLoadingTime } from '../../../../../hooks/common/useMinimumLoadingTime';
import { useIsMobile } from '../../../../../hooks/common/useIsMobile';

import { useModelHealthSummaries } from '../../../../../hooks/group-health/useModelHealthSummaries';
import { formatHealthLatency } from '../../../../group-health/healthUtils';
import {
  formatModelContext,
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
  if (showSkeleton) return <PricingCardSkeleton skeletonCount={6} />;
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
          const tags = [
            ...new Set(
              (model.tags || '')
                .split(/[,;|]+/)
                .map((tag) => tag.trim())
                .filter(Boolean),
            ),
          ];
          const icon = model.icon || model.vendor_icon;
          const stats = health.get(model.model_name);
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
                    ? getLobeHubIcon(icon, 28)
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
                  <span>
                    {model.vendor_name || t('未知供应商')} ·{' '}
                    {data.isDynamicPricing
                      ? t('动态计费')
                      : model.quota_type === 2
                        ? t('按小时计费')
                        : model.quota_type === 1
                          ? t('按次计费')
                          : t('按量计费')}
                  </span>
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
                {items.length ? (
                  items
                    .filter(
                      (item) =>
                        !String(item.key).toLowerCase().includes('cache'),
                    )
                    .map((item) => (
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
              <dl
                className='pricing-card-performance'
                title={`${t('全部分组')} · ${t('最近 {{hours}} 小时', { hours: 24 })} · ${t('平台分组统计，不限于个人请求。')}`}
              >
                <div>
                  <dt>
                    <ScanText size={13} />
                    {t('上下文长度')}
                  </dt>
                  <dd>{formatModelContext(model.context_length)}</dd>
                </div>
                <div>
                  <dt>
                    <Timer size={13} />
                    {t('首字均值')}
                  </dt>
                  <dd>{formatHealthLatency(stats?.avg_ttft_ms)}</dd>
                </div>
                <div>
                  <dt>
                    <Gauge size={13} />
                    {t('输出速度')}
                  </dt>
                  <dd>{formatModelThroughput(stats?.avg_output_tps)}</dd>
                </div>
              </dl>
              {model.description && (
                <p className='pricing-model-description'>{model.description}</p>
              )}
              {data.isDynamicPricing && (
                <p className='pricing-card-tier-note'>
                  {t('阶梯计费：展示首档参考价，完整规则见详情')}
                </p>
              )}
              {showRatio && (
                <div className='pricing-card-ratios'>
                  {model.quota_type === 0 && (
                    <span>
                      {t('模型倍率')}: {model.model_ratio ?? '—'} ·{' '}
                      {t('补全倍率')}: {model.completion_ratio ?? '—'}
                    </span>
                  )}
                  <span>
                    {t('分组倍率')}: {data.usedGroupRatio}× · {t('时间倍率')}:{' '}
                    {formatTimeRatioValue(data.timeRatio)}×
                  </span>
                </div>
              )}
              <div className='pricing-model-card-footer'>
                <div className='pricing-model-tags'>
                  {tags.slice(0, 3).map((tag) => (
                    <span key={tag}>{tag}</span>
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
