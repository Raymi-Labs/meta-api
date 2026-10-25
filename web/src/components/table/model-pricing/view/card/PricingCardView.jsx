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
import { Checkbox, Pagination, Button } from '@douyinfe/semi-ui';
import { ArrowUpRight, Copy } from 'lucide-react';
import { getLobeHubIcon, formatTimeRatioValue } from '../../../../../helpers';
import { getModelCatalogPricing } from '../../../../../helpers/modelCatalogPricing';
import PricingCardSkeleton from './PricingCardSkeleton';
import { useMinimumLoadingTime } from '../../../../../hooks/common/useMinimumLoadingTime';
import { useIsMobile } from '../../../../../hooks/common/useIsMobile';

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
  openModelDetail,
}) => {
  const showSkeleton = useMinimumLoadingTime(loading);
  const isMobile = useIsMobile();
  if (showSkeleton) return <PricingCardSkeleton skeletonCount={6} />;
  const paginatedModels = filteredModels.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );

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
          return (
            <article
              key={key}
              className={`pricing-model-card${selected ? ' is-selected' : ''}`}
              aria-label={model.model_name}
            >
              <div className='pricing-model-card-header'>
                <div className='pricing-model-icon' aria-hidden='true'>
                  {icon
                    ? getLobeHubIcon(icon, 28)
                    : model.model_name.slice(0, 2).toUpperCase()}
                </div>
                <div className='pricing-model-identity'>
                  <h2>
                    <button
                      type='button'
                      onClick={() => openModelDetail(model)}
                    >
                      {model.model_name}
                    </button>
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
              {model.description && (
                <p className='pricing-model-description'>{model.description}</p>
              )}
              <dl className='pricing-card-prices'>
                {items.length ? (
                  items.map((item) => (
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
                <Button
                  size='small'
                  theme='borderless'
                  type='primary'
                  icon={<ArrowUpRight size={14} />}
                  onClick={() => openModelDetail(model)}
                >
                  {t('价格详情')}
                </Button>
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
