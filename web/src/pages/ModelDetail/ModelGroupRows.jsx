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

import { modelPerformance, modelUptime } from '../../helpers/modelPerformance';
import { supportsTextGenerationMetrics } from '../../helpers/modelHealthMetrics';

import React, { useState } from 'react';
import ModelGroupPrice from './ModelGroupPrice';
import ModelGroupMetric from './ModelGroupMetric';
import { modelGroupHealthState } from '../../helpers/modelGroupHealthState';
import ModelSectionHelp from './ModelSectionHelp';
import ModelGroupTierTable from './ModelGroupTierTable';
import {
  groupTierRows,
  groupComparisonUnavailable,
} from '../../helpers/modelGroupTiers';
import { parseTiersFromExpr } from '../../helpers/render';
import { splitBillingExprAndRequestRules } from '../Setting/Ratio/components/requestRuleExpr';
import { isVideoPriceExpression } from '../../helpers/videoPricingTemplate';
import { isImagePriceExpression } from '../../helpers/imagePricing';
import { Button, Input, Pagination } from '@douyinfe/semi-ui';
import { Copy } from 'lucide-react';
import {
  getModelCatalogPricing,
  isCachePriceItem,
} from '../../helpers/modelCatalogPricing';
import { formatModelThroughput } from '../../helpers/modelPresentation';
import {
  GroupHealthPanel,
  GroupHealthCollectionNote,
} from '../../components/group-health';
import {
  formatHealthLatency,
  formatHealthRate,
  uptimeTone,
  resolveGroupHealthMetric,
} from '../../components/group-health/healthUtils';

export default function ModelGroupRows({
  model,
  groups,
  selectedGroup,
  data,
  health,
}) {
  const { t } = data;
  const isTokenExpression =
    model.billing_mode === 'tiered_expr' &&
    !isVideoPriceExpression(model.billing_expr) &&
    !isImagePriceExpression(model.billing_expr);
  const tiers = isTokenExpression
    ? parseTiersFromExpr(
        splitBillingExprAndRequestRules(model.billing_expr || '').billingExpr,
      )
    : [];

  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [expanded, setExpanded] = useState([]);
  const filtered = groups.filter(
    (name) =>
      (selectedGroup === 'all' || name === selectedGroup) &&
      name.toLowerCase().includes(search.toLowerCase()),
  );
  const currentPage = Math.min(
    page,
    Math.max(1, Math.ceil(filtered.length / 6)),
  );
  const statsByGroup = new Map(
    (health.data?.groups || []).map((stats) => [stats.group, stats]),
  );
  return (
    <div className='model-group-list'>
      {model.no_discount && (
        <p className='model-detail-caption'>
          {t('不参与折扣，按模型基础价计费')}
        </p>
      )}
      {groups.length > 6 && (
        <Input
          aria-label={t('搜索分组')}
          placeholder={t('搜索分组')}
          value={search}
          showClear
          onChange={(value) => {
            setSearch(value);
            setPage(1);
          }}
        />
      )}
      <GroupHealthCollectionNote data={health.data} />
      {filtered.slice((currentPage - 1) * 6, currentPage * 6).map((group) => {
        const getPricing = (tierIndex = 0) =>
          getModelCatalogPricing(
            {
              record: model,
              tierIndex,
              allTiers: true,
              selectedGroup: group,
              groupRatio: data.groupRatio,
              timeRatioMap: data.timeRatioMap,
              currency: data.currency,
              quotaDisplayType: data.siteDisplayType,
              tokenUnit: data.tokenUnit,
              displayPrice: data.displayPrice,
            },
            t,
          );
        const { items, data: pricing } = getPricing();
        const rows =
          !pricing.isDynamicPricing || tiers.length
            ? groupTierRows(model, tiers, getPricing, t)
            : [];
        const officialTier = rows[0]?.officialTier;
        const stats = health.error ? undefined : statsByGroup.get(group);
        const metric = resolveGroupHealthMetric(stats, group, true);
        const performance = modelPerformance(stats, metric);
        const { latency } = performance;
        const showGenerationMetrics = supportsTextGenerationMetrics(model);
        const showLatencyMetric =
          showGenerationMetrics || metric === 'completion';
        const availability = modelUptime(stats);
        const rate = availability.value;
        const referenceVisible =
          availability.external &&
          health.isAuthenticated &&
          !health.error &&
          health.data?.enabled !== false;
        const samples = Math.max(
          Number(stats?.uptime_sample_count) || 0,
          Number(stats?.request_count) || 0,
          Number(stats?.probe_count) || 0,
        );
        const uptimeReady =
          modelGroupHealthState(health, rate, { rate: true, samples }) ===
          'ready';
        const open =
          expanded.includes(group) && uptimeReady && !availability.external;
        const metricProps = { health, modelName: model.model_name, t };
        return (
          <article className='model-group-row' key={group}>
            <header>
              <strong>{group}</strong>
              <span className='model-group-id'>{model.model_name}</span>
              <Button
                size='small'
                theme='borderless'
                type='tertiary'
                aria-label={t('复制模型名称') + ': ' + group}
                icon={<Copy size={13} />}
                onClick={() => data.copyText(model.model_name)}
              />
              {model.official_pricing?.tiers?.length > 0 &&
                rows.length <= 1 &&
                !officialTier && (
                  <ModelSectionHelp label={t('官方参考价') + ': ' + group}>
                    <p>
                      {t(
                        rows[0]?.comparisonUnavailable ||
                          groupComparisonUnavailable(model),
                      )}
                    </p>
                  </ModelSectionHelp>
                )}
            </header>
            <div className='model-group-values'>
              {rows.length > 1 || tiers.length > 1 ? (
                <ModelGroupTierTable rows={rows} model={model} data={data} />
              ) : (
                <div
                  className={`model-group-pricing-values${pricing.videoPricing ? ' model-group-video-prices' : ''}${!items.some(isCachePriceItem) ? ' model-group-no-cache' : ''}`}
                >
                  <div className='model-group-price-cell model-group-price-items'>
                    {items.length
                      ? items
                          .filter((item) => !isCachePriceItem(item))
                          .map((item) => (
                            <ModelGroupPrice
                              key={item.key}
                              item={item}
                              pricing={pricing}
                              model={model}
                              officialTier={officialTier}
                              data={data}
                            />
                          ))
                      : t('见价格详情')}
                  </div>
                  {pricing.videoPricing && (
                    <p className='model-detail-caption model-video-price-note'>
                      {t(
                        '按请求视频时长计费；价格已包含分组和时间倍率，并按当前价格模式换算。',
                      )}
                      {items.some((item) => item.multiple) &&
                        ` ${t('多份生成按份数累加。')}`}
                    </p>
                  )}
                  {items.some(isCachePriceItem) && (
                    <div className='model-group-price-items'>
                      {items.filter(isCachePriceItem).map((item) => (
                        <ModelGroupPrice
                          key={item.key}
                          item={item}
                          pricing={pricing}
                          model={model}
                          officialTier={officialTier}
                          data={data}
                        />
                      ))}
                    </div>
                  )}
                </div>
              )}
              <dl className='model-group-health-values'>
                {showLatencyMetric && (
                  <>
                    <div>
                      <dt>
                        {metric === 'completion'
                          ? t('平均完成')
                          : t(
                              performance.externalLatency
                                ? '首字时间'
                                : '首字均值',
                            )}
                      </dt>
                      <dd>
                        <ModelGroupMetric
                          {...metricProps}
                          value={latency.value}
                          formatted={formatHealthLatency(latency.value)}
                          samples={stats?.request_count}
                          label={
                            group +
                            ' · ' +
                            t(metric === 'completion' ? '平均完成' : '首字均值')
                          }
                        />
                      </dd>
                    </div>
                    {showGenerationMetrics && (
                      <div>
                        <dt>{t('输出速度')}</dt>
                        <dd>
                          <ModelGroupMetric
                            {...metricProps}
                            value={performance.throughput}
                            formatted={formatModelThroughput(
                              performance.throughput,
                            )}
                            samples={stats?.request_count}
                            label={group + ' · ' + t('输出速度')}
                          />
                        </dd>
                      </div>
                    )}
                  </>
                )}
                <div>
                  <dt className='model-section-title-with-help'>
                    Uptime{' '}
                    <small>
                      · {t(referenceVisible ? '参考可用率' : '可用率')}
                      {referenceVisible ? ' · 24h' : ''}
                    </small>
                    {referenceVisible && (
                      <ModelSectionHelp label={t('参考可用率') + ': ' + group}>
                        <p>
                          {t(
                            '模型端点最近24小时的参考可用率，不代表本站分组实测。',
                          )}
                        </p>
                        <p>
                          {t('更新时间：{{time}}', {
                            time: new Date(
                              availability.fetchedAt * 1000,
                            ).toLocaleString(),
                          })}
                        </p>
                      </ModelSectionHelp>
                    )}
                  </dt>
                  <dd>
                    <ModelGroupMetric
                      {...metricProps}
                      rate
                      value={rate}
                      formatted={formatHealthRate(rate)}
                      samples={samples}
                      label={group + ' · Uptime'}
                      tone={availability.external ? 'empty' : uptimeTone(stats)}
                      open={open}
                      onToggle={
                        availability.external
                          ? undefined
                          : () =>
                              setExpanded(
                                open
                                  ? expanded.filter((value) => value !== group)
                                  : [...expanded, group],
                              )
                      }
                    />
                  </dd>
                </div>
              </dl>
            </div>
            {open && stats && !health.error && (
              <GroupHealthPanel
                uptime
                showDetails={false}
                stats={stats}
                metric={metric}
                showTitle={false}
              />
            )}
          </article>
        );
      })}
      {!filtered.length && (
        <p className='model-detail-caption'>{t('暂无数据')}</p>
      )}
      {filtered.length > 6 && (
        <Pagination
          currentPage={currentPage}
          total={filtered.length}
          pageSize={6}
          onPageChange={setPage}
        />
      )}
    </div>
  );
}
