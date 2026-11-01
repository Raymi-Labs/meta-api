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
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { Button, Select, Spin } from '@douyinfe/semi-ui';
import {
  ArrowLeft,
  ArrowUpRight,
  Copy,
  Plus,
  SlidersHorizontal,
  X,
} from 'lucide-react';
import { getLobeHubIcon } from '../../helpers';
import { useModelPricingData } from '../../hooks/model-pricing/useModelPricingData';
import {
  getCompareModels,
  buildModelComparePath,
  buildModelPlaygroundPath,
  getCompareGroups,
  MAX_COMPARE_MODELS,
} from '../../helpers/modelCompare';
import {
  getComparePricing,
  comparePriceItems,
} from '../../helpers/modelComparePricing';
import {
  buildModelDetailPath,
  sanitizeModelCatalogSnapshot,
} from '../../helpers/modelDetailNavigation';
import {
  MODEL_MODALITY_LABELS,
  formatModelThroughput,
  formatModelContextExact,
} from '../../helpers/modelPresentation';
import {
  MODEL_CAPABILITY_LABELS,
  modelCapabilityStatus,
  formatModelTokenLimit,
  modelAttributeSource,
} from '../../helpers/modelAttributes';
import {
  getDeclaredModelProtocols,
  getModelProtocolLabel,
} from '../../helpers/modelProtocols';
import {
  formatChartDetailValue,
  chartDisplayValue,
  chartMeasurement,
} from '../../helpers/modelChartDisplay';
import { formatHealthReference } from '../../components/group-health/healthUtils';
import ComparisonMetrics, {
  useComparisonHealth,
  comparisonStats,
} from './ComparisonMetrics';
import ModelCostCalculator from './ModelCostCalculator';
import ModelPriceSettings from '../ModelDetail/ModelPriceSettings';
import '../ModelDetail/modelDetail.css';
import './modelCompare.css';

const sourceTitle = (model, field, known, t) =>
  t(modelAttributeSource(model, field, known).label);
export default function ModelCompare() {
  const location = useLocation();
  const navigate = useNavigate();
  const data = useModelPricingData(location.state?.catalogState);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const navigationState = { catalogState: sanitizeModelCatalogSnapshot(data) };
  const { t } = data;
  const selectedNames = getCompareModels(location.search);
  const selected = selectedNames
    .map((name) => data.models.find((model) => model.model_name === name))
    .filter(Boolean);
  const unavailable = selectedNames.filter(
    (name) => !data.models.some((model) => model.model_name === name),
  );
  const [groupChoice, setGroupChoice] = useState('');
  const [hours, setHours] = useState(24);
  const groups = getCompareGroups(selected, data.usableGroup);
  const group = groups.includes(groupChoice) ? groupChoice : groups[0] || '';
  const health = useComparisonHealth(selected, group, hours);
  const prices = selected.map((model) =>
    getComparePricing(model, data, group, t),
  );
  const items = prices.map(comparePriceItems);
  const priceRows = [
    ...new Map(
      items.flatMap((list) => list.map((item) => [item.key, item.label])),
    ).entries(),
  ];
  const changeModels = (names) =>
    navigate(buildModelComparePath(names), { state: navigationState });
  const removeModel = (name) =>
    changeModels(selectedNames.filter((item) => item !== name));
  const options = data.models
    .filter((model) => !selectedNames.includes(model.model_name))
    .map((model) => ({
      value: model.model_name,
      label: `${model.display_name || model.model_name}${model.display_name && model.display_name !== model.model_name ? ` · ${model.model_name}` : ''}`,
    }));
  const renderAttribute = (model, field) => (
    <span title={sourceTitle(model, field, Number(model[field]) > 0, t)}>
      {formatModelTokenLimit(model[field])}
      <small className='model-compare-source'>
        {sourceTitle(model, field, Number(model[field]) > 0, t)}
      </small>
    </span>
  );
  const modalities = (model, field) =>
    (model[field] || [])
      .map((value) => t(MODEL_MODALITY_LABELS[value] || value))
      .join(' · ') || '—';
  const back = (
    <Link
      className='model-detail-back'
      to='/pricing'
      state={{ modelCatalogSnapshot: navigationState.catalogState }}
    >
      <ArrowLeft size={16} />
      {t('返回模型广场')}
    </Link>
  );
  if (data.loading || data.loadError)
    return (
      <main className='model-detail-page model-compare-page'>
        {back}
        <div className='model-detail-state' role='status'>
          {data.loading ? (
            <Spin />
          ) : (
            <>
              <p>{t('加载失败，请重试')}</p>
              <Button onClick={data.refresh}>{t('重试')}</Button>
            </>
          )}
        </div>
      </main>
    );
  const row = (key, label, render) => (
    <tr key={key}>
      <th scope='row'>{label}</th>
      {selected.map((model, index) => (
        <td key={model.model_name}>{render(model, index)}</td>
      ))}
    </tr>
  );
  return (
    <main className='model-detail-page model-compare-page'>
      {back}
      <header className='model-compare-heading'>
        <p className='model-compare-eyebrow'>{t('模型对比')}</p>
        <h1>
          {selected.length > 1
            ? selected
                .map((model) => model.display_name || model.model_name)
                .join(' vs ')
            : t('选择模型进行对比')}
        </h1>
        <p>{t('并排查看模型价格、属性、能力与实际调用表现。')}</p>
      </header>
      <div className='model-compare-picker'>
        <div className='model-compare-chips'>
          {selectedNames.map((name) => (
            <span className='model-compare-chip' key={name}>
              <span>
                {selected.find((model) => model.model_name === name)
                  ?.display_name || name}
              </span>
              <button
                onClick={() => removeModel(name)}
                aria-label={`${t('移除')} ${name}`}
              >
                <X size={14} />
              </button>
            </span>
          ))}
          <span id='compare-add-label' className='sr-only'>
            {t('添加模型')}
          </span>
          <Select
            filter
            searchPosition='dropdown'
            aria-labelledby='compare-add-label'
            placeholder={`${t('添加模型')} · ${selectedNames.length}/${MAX_COMPARE_MODELS}`}
            value={null}
            disabled={selectedNames.length >= MAX_COMPARE_MODELS}
            onChange={(name) => changeModels([...selectedNames, name])}
            optionList={options}
            prefix={<Plus size={15} />}
          />
        </div>
        <Button
          theme='outline'
          icon={<Copy size={15} />}
          onClick={() =>
            data.copyText(
              new URL(
                buildModelComparePath(selectedNames),
                window.location.origin,
              ).href,
            )
          }
        >
          {t('复制链接')}
        </Button>
      </div>
      {unavailable.length > 0 && (
        <p className='model-compare-warning' role='status'>
          {t('该模型不存在或当前不可见')}：{unavailable.join('、')}
        </p>
      )}
      {!selected.length ? (
        <div className='model-compare-empty'>{t('选择模型进行对比')}</div>
      ) : (
        <>
          <div className='model-compare-controls'>
            <label>
              <span id='compare-group-label'>{t('对比分组')}</span>{' '}
              <Select
                aria-labelledby='compare-group-label'
                value={group || undefined}
                optionList={groups.map((value) => ({ value, label: value }))}
                onChange={setGroupChoice}
              />
            </label>
            <label>
              <span id='compare-hours-label'>{t('时间范围')}</span>{' '}
              <Select
                aria-labelledby='compare-hours-label'
                value={hours}
                onChange={setHours}
                optionList={[24, 72].map((value) => ({
                  value,
                  label: t('最近 {{hours}} 小时', { hours: value }),
                }))}
              />
            </label>
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
            <span>{`${t('价格')} · ${data.currency} · ${data.showWithRecharge ? t(data.priceConvertMode === 'package' && data.availablePlans.length ? '套餐价格' : '充值价格') : t('基础价格')}`}</span>
          </div>
          {settingsOpen && <ModelPriceSettings data={data} />}
          <div
            className='model-compare-cards'
            style={{ '--compare-count': selected.length }}
          >
            {selected.map((model, index) => (
              <article className='model-compare-card' key={model.model_name}>
                <div className='model-compare-card-title'>
                  <span className='model-compare-logo' aria-hidden='true'>
                    {model.icon || model.vendor_icon
                      ? getLobeHubIcon(
                          model.icon || model.vendor_icon,
                          32,
                          model.vendor_icon,
                        )
                      : model.model_name.slice(0, 2).toUpperCase()}
                  </span>
                  <div>
                    <h2>{model.display_name || model.model_name}</h2>
                    <p>{model.vendor_name || t('未知供应商')}</p>
                  </div>
                </div>
                <code>{model.model_name}</code>
                {model.description && (
                  <p
                    className='model-compare-description'
                    title={model.description}
                  >
                    {model.description}
                  </p>
                )}
                <div className='model-compare-card-prices'>
                  {prices[index] ? (
                    items[index].slice(0, 3).map((item) => (
                      <p key={item.key}>
                        <span>{item.label}</span>
                        <strong>
                          {item.value}
                          <small>{item.suffix}</small>
                        </strong>
                      </p>
                    ))
                  ) : (
                    <p>{t('当前分组不可用')}</p>
                  )}
                  {prices[index]?.data.isDynamicPricing && (
                    <small>
                      {t('本站首档：{{tier}}', {
                        tier: prices[index].data.firstTierLabel || '—',
                      })}
                    </small>
                  )}
                </div>
                <div className='model-compare-card-actions'>
                  <Link
                    className='model-action-secondary'
                    to={buildModelDetailPath(model.model_name)}
                    state={navigationState}
                  >
                    {t('查看详情')}
                  </Link>
                  <Link
                    className='model-action-primary'
                    to={buildModelPlaygroundPath(model.model_name, group)}
                  >
                    {t('模型体验')}
                    <ArrowUpRight size={14} />
                  </Link>
                </div>
              </article>
            ))}
          </div>
          <section className='model-detail-section'>
            <h2>{t('价格与规格')}</h2>
            <p className='model-detail-caption'>
              {t('价格已包含各分组适用倍率，实际费用取决于用量及计费规则。')}
            </p>
            <div
              className='model-compare-table-scroll'
              tabIndex={0}
              role='region'
              aria-label={t('价格与规格')}
            >
              <table className='model-compare-table'>
                <thead>
                  <tr>
                    <th scope='col'>{t('模型属性')}</th>
                    {selected.map((model) => (
                      <th scope='col' key={model.model_name}>
                        {model.display_name || model.model_name}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {row('group', t('分组'), (_, index) =>
                    prices[index] ? group : t('当前分组不可用'),
                  )}
                  {priceRows.map(([key, label]) =>
                    row(key, label, (_, index) => {
                      const item = items[index].find(
                        (value) => value.key === key,
                      );
                      return item ? (
                        <>
                          {item.value}
                          <small>{item.suffix}</small>
                          {prices[index]?.data.isDynamicPricing && (
                            <small className='model-compare-source'>
                              {t('本站首档：{{tier}}', {
                                tier: prices[index].data.firstTierLabel || '—',
                              })}
                            </small>
                          )}
                        </>
                      ) : (
                        '—'
                      );
                    }),
                  )}
                  {row('context', t('上下文'), (model) => (
                    <span title={formatModelContextExact(model.context_length)}>
                      {renderAttribute(model, 'context_length')}
                    </span>
                  ))}
                  {row('output', t('最大输出'), (model) =>
                    renderAttribute(model, 'max_output_tokens'),
                  )}
                  {row('input-modality', t('输入模态'), (model) =>
                    modalities(model, 'input_modalities'),
                  )}
                  {row('output-modality', t('输出模态'), (model) =>
                    modalities(model, 'output_modalities'),
                  )}
                  {row('latency', t('首字均值'), (_, index) => (
                    <span
                      title={formatHealthReference(
                        chartMeasurement(
                          comparisonStats(health[index], group),
                          'avg_ttft_ms',
                        ),
                        t,
                      )}
                    >
                      {formatChartDetailValue(
                        chartDisplayValue(
                          comparisonStats(health[index], group),
                          'avg_ttft_ms',
                        ),
                        'avg_ttft_ms',
                      )}
                      {chartMeasurement(
                        comparisonStats(health[index], group),
                        'avg_ttft_ms',
                      )?.referenceTs && <small> · {t('沿用最近有效值')}</small>}
                    </span>
                  ))}
                  {row('throughput', t('输出速度'), (_, index) =>
                    formatModelThroughput(
                      comparisonStats(health[index], group)?.avg_output_tps,
                    ),
                  )}
                  {row(
                    'protocol',
                    t('接口协议'),
                    (model) =>
                      getDeclaredModelProtocols(model)
                        .map(getModelProtocolLabel)
                        .join(' · ') || '—',
                  )}
                  {row(
                    'release',
                    t('发布日期'),
                    (model) => model.release_date || '—',
                  )}
                  {Object.entries(MODEL_CAPABILITY_LABELS).map(([key, label]) =>
                    row(`capability-${key}`, t(label), (model) => (
                      <span
                        className={`model-compare-capability ${model.capabilities?.[key] === true ? 'is-supported' : 'is-muted'}`}
                        title={sourceTitle(
                          model,
                          `capabilities.${key}`,
                          typeof model.capabilities?.[key] === 'boolean',
                          t,
                        )}
                      >
                        {t(modelCapabilityStatus(model.capabilities?.[key]))}
                      </span>
                    )),
                  )}
                </tbody>
              </table>
            </div>
          </section>
          <ComparisonMetrics
            models={selected}
            health={health}
            group={group}
            t={t}
          />
          <ModelCostCalculator
            models={selected}
            prices={prices}
            data={data}
            navigationState={navigationState}
            t={t}
          />
          {selected.some((model) => model.faq?.length) && (
            <section className='model-detail-section model-faq'>
              <h2>{t('常见问题')}</h2>
              {selected.flatMap((model) =>
                (model.faq || []).map((entry, index) => (
                  <details key={`${model.model_name}-${index}`}>
                    <summary>
                      <small>{model.display_name || model.model_name} · </small>
                      {entry.question}
                    </summary>
                    <p style={{ whiteSpace: 'pre-wrap' }}>{entry.answer}</p>
                  </details>
                )),
              )}
            </section>
          )}
        </>
      )}
    </main>
  );
}
