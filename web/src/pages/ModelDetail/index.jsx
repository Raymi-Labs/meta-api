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

import React, { useLayoutEffect, useRef, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { Button, Select, Spin } from '@douyinfe/semi-ui';
import { ArrowLeft, ArrowUpRight, Copy, SlidersHorizontal } from 'lucide-react';
import { getLobeHubIcon } from '../../helpers';
import { useModelPricingData } from '../../hooks/model-pricing/useModelPricingData';
import { getModelCatalogPricing } from '../../helpers/modelCatalogPricing';
import {
  buildModelDetailPath,
  getModelNameFromDetailPath,
  sanitizeModelCatalogSnapshot,
} from '../../helpers/modelDetailNavigation';
import ModelGroupPerformance from '../../components/table/model-pricing/modal/components/ModelGroupPerformance';
import ModelEndpoints from '../../components/table/model-pricing/modal/components/ModelEndpoints';
import DynamicPricingBreakdown from '../../components/table/model-pricing/modal/components/DynamicPricingBreakdown';
import ModelQuickStart from './ModelQuickStart.jsx';
import './modelDetail.css';

export default function ModelDetail() {
  const location = useLocation();
  const catalogState = sanitizeModelCatalogSnapshot(
    location.state?.catalogState,
  );
  const data = useModelPricingData(catalogState);
  const { t } = data;
  const modelName = getModelNameFromDetailPath(location.pathname);
  const model = data.models.find((item) => item.model_name === modelName);
  const pageRef = useRef(null);
  const [priceGroup, setPriceGroup] = useState('all');
  const [settingsOpen, setSettingsOpen] = useState(false);
  useLayoutEffect(() => {
    pageRef.current?.scrollIntoView({ block: 'start' });
    setPriceGroup('all');
  }, [location.pathname]);

  const backLink = (
    <Link
      className='model-detail-back'
      to='/pricing'
      state={{ modelCatalogSnapshot: catalogState }}
    >
      <ArrowLeft size={16} aria-hidden='true' />
      {t('返回模型广场')}
    </Link>
  );
  if (data.loading || data.loadError || !model) {
    return (
      <main className='model-detail-page' ref={pageRef}>
        {backLink}
        <div className='model-detail-state' role='status'>
          {data.loading ? (
            <>
              <Spin size='large' />
              <p>{t('加载中...')}</p>
            </>
          ) : (
            <>
              <h1>
                {t(
                  data.loadError
                    ? '加载失败，请重试'
                    : '该模型不存在或当前不可见',
                )}
              </h1>
              {data.loadError && (
                <Button onClick={data.refresh}>{t('重试')}</Button>
              )}
            </>
          )}
        </div>
      </main>
    );
  }

  const groups = Object.keys(data.usableGroup || {}).filter(
    (group) =>
      group !== 'auto' && group && model.enable_groups?.includes(group),
  );
  const selectedGroup = groups.includes(priceGroup) ? priceGroup : 'all';
  const { data: priceData, items } = getModelCatalogPricing(
    {
      record: model,
      selectedGroup,
      groupRatio: data.groupRatio,
      timeRatioMap: data.timeRatioMap,
      currency: data.currency,
      quotaDisplayType: data.siteDisplayType,
      tokenUnit: data.tokenUnit,
      displayPrice: data.displayPrice,
    },
    t,
  );
  const icon = model.icon || model.vendor_icon;
  const tags = [
    ...new Set(
      (model.tags || '')
        .split(/[,;|]+/)
        .map((tag) => tag.trim())
        .filter(Boolean),
    ),
  ];
  const related =
    model.vendor_id || model.vendor_name
      ? data.models
          .filter(
            (item) =>
              item.model_name !== model.model_name &&
              (model.vendor_id
                ? item.vendor_id === model.vendor_id
                : item.vendor_name === model.vendor_name),
          )
          .slice(0, 6)
      : [];
  const sections = [
    ['pricing', '价格详情'],
    ['endpoints', 'API端点'],
    ['quick-start', '快速开始'],
    ...(related.length ? [['related', '同厂商模型']] : []),
  ];
  const priceMode = data.showWithRecharge ? data.priceConvertMode : 'base';

  return (
    <main className='model-detail-page' ref={pageRef}>
      <div className='model-detail-breadcrumb'>
        {backLink}
        <span>{t('模型详情')}</span>
      </div>
      <header className='model-detail-hero'>
        <div className='model-detail-icon' aria-hidden='true'>
          {icon
            ? getLobeHubIcon(icon, 38)
            : model.model_name.slice(0, 2).toUpperCase()}
        </div>
        <div className='model-detail-identity'>
          <div className='model-detail-title-row'>
            <h1>{model.model_name}</h1>
            <Button
              theme='borderless'
              type='tertiary'
              icon={<Copy size={17} />}
              aria-label={t('复制模型名称')}
              title={t('复制模型名称')}
              onClick={() => data.copyText(model.model_name)}
            />
          </div>
          <div className='model-detail-metadata'>
            <span>{model.vendor_name || t('未知供应商')}</span>
            <span aria-hidden='true'>·</span>
            <span>
              {t(
                priceData.isDynamicPricing
                  ? '动态计费'
                  : model.quota_type === 2
                    ? '按小时计费'
                    : model.quota_type === 1
                      ? '按次计费'
                      : '按量计费',
              )}
            </span>
          </div>
          <p>
            {model.description || model.vendor_description || t('暂无模型描述')}
          </p>
          {tags.length > 0 && (
            <div className='model-detail-tags'>
              {tags.map((tag) => (
                <span key={tag}>{tag}</span>
              ))}
            </div>
          )}
        </div>
        <Link className='model-detail-primary' to='/docs/api-examples'>
          {t('API 文档')}
          <ArrowUpRight size={16} aria-hidden='true' />
        </Link>
      </header>

      <div className='model-detail-layout'>
        <nav className='model-detail-sections' aria-label={t('模型详情')}>
          {sections.map(([id, label]) => (
            <button
              key={id}
              type='button'
              onClick={() =>
                pageRef.current
                  ?.querySelector(`#${id}`)
                  ?.scrollIntoView({ block: 'start' })
              }
            >
              {t(label)}
            </button>
          ))}
        </nav>
        <div className='model-detail-main'>
          <section id='pricing' className='model-detail-section'>
            <div className='model-detail-section-heading'>
              <h2>{t('价格详情')}</h2>
              <div className='model-detail-price-controls'>
                <Select
                  aria-label={t('分组')}
                  value={selectedGroup}
                  onChange={setPriceGroup}
                  optionList={[
                    { value: 'all', label: t('全部分组') },
                    ...groups.map((group) => ({ value: group, label: group })),
                  ]}
                />
                <Button
                  theme='outline'
                  type='tertiary'
                  aria-label={t('按K显示单位')}
                  onClick={() =>
                    data.setTokenUnit(data.tokenUnit === 'M' ? 'K' : 'M')
                  }
                >
                  {data.tokenUnit}
                </Button>
                <Button
                  theme='borderless'
                  type='tertiary'
                  icon={<SlidersHorizontal size={16} />}
                  aria-expanded={settingsOpen}
                  onClick={() => setSettingsOpen(!settingsOpen)}
                >
                  {t('显示设置')}
                </Button>
              </div>
            </div>
            {settingsOpen && (
              <div className='model-detail-settings'>
                <label>
                  <span>{t('价格模式')}</span>
                  <Select
                    aria-label={t('价格模式')}
                    value={priceMode}
                    onChange={(value) => {
                      data.setShowWithRecharge(value !== 'base');
                      if (value !== 'base') data.setPriceConvertMode(value);
                    }}
                    optionList={[
                      {
                        value: 'base',
                        label:
                          data.siteDisplayType === 'CUSTOM'
                            ? t('自定义货币')
                            : 'USD ($)',
                      },
                      { value: 'recharge', label: t('充值价格') },
                      ...(data.availablePlans.length
                        ? [{ value: 'package', label: t('套餐价格') }]
                        : []),
                    ]}
                  />
                </label>
                {priceMode === 'package' && data.availablePlans.length > 0 && (
                  <label>
                    <span>{t('请选择套餐')}</span>
                    <Select
                      aria-label={t('请选择套餐')}
                      value={data.selectedPlanId}
                      onChange={data.setSelectedPlanId}
                      optionList={data.availablePlans.map((plan) => ({
                        value: plan.id,
                        label: plan.title,
                      }))}
                    />
                  </label>
                )}
              </div>
            )}
            <p className='model-detail-caption'>
              {selectedGroup === 'all'
                ? t('全部分组：每个模型展示其最低适用倍率价格')
                : t('当前分组：{{group}} · 价格已包含分组倍率', {
                    group: selectedGroup,
                  })}
              <span>
                {' '}
                ·{' '}
                {priceMode === 'base'
                  ? data.siteDisplayType === 'CUSTOM'
                    ? t('自定义货币')
                    : 'USD'
                  : t(priceMode === 'package' ? '套餐价格' : '充值价格')}
              </span>
            </p>
            {groups.length > 0 ? (
              <dl className='model-detail-prices'>
                {items.map((item) => (
                  <div key={item.key}>
                    <dt>{item.label}</dt>
                    <dd>
                      <strong>{item.value}</strong>
                      <span>{item.suffix}</span>
                    </dd>
                  </div>
                ))}
              </dl>
            ) : (
              <p className='model-detail-caption'>
                {t('当前模型没有可用分组')}
              </p>
            )}
            {priceData.isDynamicPricing && (
              <div className='model-detail-dynamic'>
                <p className='model-detail-caption'>
                  {t('阶梯计费：展示首档参考价，完整规则见详情')}
                </p>
                <h3 className='model-detail-caption'>{t('基础价格')}</h3>
                <DynamicPricingBreakdown
                  billingExpr={model.billing_expr}
                  displayPrice={data.displayPrice}
                  t={t}
                />
              </div>
            )}
            <div className='model-detail-group-prices'>
              <ModelGroupPerformance
                enabled
                modelData={model}
                groupRatio={data.groupRatio}
                timeRatioMap={data.timeRatioMap}
                currency={data.currency}
                siteDisplayType={data.siteDisplayType}
                tokenUnit={data.tokenUnit}
                displayPrice={data.displayPrice}
                showRatio={catalogState.showRatio}
                usableGroup={data.usableGroup}
                autoGroups={data.autoGroups}
                t={t}
              />
            </div>
          </section>
          <section id='endpoints' className='model-detail-section'>
            <ModelEndpoints
              modelData={model}
              endpointMap={data.endpointMap}
              t={t}
            />
            {!model.supported_endpoint_types?.length && (
              <p className='model-detail-caption'>{t('未声明')}</p>
            )}
          </section>
          <section id='quick-start' className='model-detail-section'>
            <h2>{t('快速开始')}</h2>
            <ModelQuickStart
              modelData={model}
              serverAddress={data.statusState?.status?.server_address}
              t={t}
              copyText={data.copyText}
            />
          </section>
          {related.length > 0 && (
            <section id='related' className='model-detail-section'>
              <h2>{t('同厂商模型')}</h2>
              <div className='model-detail-related'>
                {related.map((item) => (
                  <Link
                    key={item.model_name}
                    to={buildModelDetailPath(item.model_name)}
                    state={{ catalogState }}
                  >
                    <span>{item.model_name}</span>
                    <ArrowUpRight size={16} aria-hidden='true' />
                  </Link>
                ))}
              </div>
            </section>
          )}
        </div>
      </div>
    </main>
  );
}
