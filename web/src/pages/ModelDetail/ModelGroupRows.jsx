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
import ModelGroupPrice from './ModelGroupPrice';
import { Link } from 'react-router-dom';
import { Button, Input, Pagination } from '@douyinfe/semi-ui';
import { ChevronDown, Copy } from 'lucide-react';
import {
  getModelCatalogPricing,
  isCachePriceItem,
} from '../../helpers/modelCatalogPricing';
import { formatModelThroughput } from '../../helpers/modelPresentation';
import {
  GroupHealthPanel,
  GroupHealthStatus,
  GroupHealthCollectionNote,
} from '../../components/group-health';
import {
  formatHealthLatency,
  formatHealthRate,
  uptimeTone,
  getUptimeRate,
  resolveGroupHealthMetric,
  getHealthLatency,
  formatHealthReference,
} from '../../components/group-health/healthUtils';

export default function ModelGroupRows({
  model,
  groups,
  selectedGroup,
  officialTier,
  data,
  health,
}) {
  const { t } = data;
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
      <GroupHealthStatus
        loading={health.loading}
        error={health.error}
        onRetry={health.refresh}
      />
      <GroupHealthCollectionNote data={health.data} />
      {!health.isAuthenticated && (
        <p className='model-detail-caption'>
          <Link to='/login'>{t('登录后查看可用分组的调用表现')}</Link>
        </p>
      )}
      {health.data?.enabled === false && (
        <p className='model-detail-caption'>
          {t('管理员暂未开启分组健康展示')}
        </p>
      )}
      {filtered.slice((currentPage - 1) * 6, currentPage * 6).map((group) => {
        const { items, data: pricing } = getModelCatalogPricing(
          {
            record: model,
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
        const stats = health.error ? undefined : statsByGroup.get(group);
        const metric = resolveGroupHealthMetric(stats, group, true);
        const latency = getHealthLatency(stats, metric, true);
        const open = expanded.includes(group);
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
            </header>
            <div className='model-group-values'>
              <div className='model-group-pricing-values'>
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
                  {!items.some(isCachePriceItem) && '—'}
                </div>
              </div>
              <dl className='model-group-health-values'>
                <div>
                  <dt>
                    {metric === 'completion' ? t('平均完成') : t('首字均值')}
                  </dt>
                  <dd title={formatHealthReference(latency, t)}>
                    {formatHealthLatency(latency.value)}
                    {latency.referenceTs && (
                      <small> · {t('沿用最近有效值')}</small>
                    )}
                  </dd>
                </div>
                <div>
                  <dt>{t('输出速度')}</dt>
                  <dd>{formatModelThroughput(stats?.avg_output_tps)}</dd>
                </div>
                <div>
                  <dt>
                    Uptime <small>· {t('可用率')}</small>
                  </dt>
                  <dd>
                    <button
                      type='button'
                      className={`model-group-uptime group-health-text-${uptimeTone(stats)}`}
                      disabled={
                        !stats ||
                        !!health.error ||
                        health.data?.enabled === false
                      }
                      aria-expanded={open}
                      aria-label={`${group} · Uptime`}
                      onClick={() =>
                        setExpanded(
                          open
                            ? expanded.filter((value) => value !== group)
                            : [...expanded, group],
                        )
                      }
                    >
                      <span className='model-uptime-spark' aria-hidden='true'>
                        {[0, 1, 2].map((i) => (
                          <i
                            key={i}
                            className={`group-health-bar-${uptimeTone(stats)}`}
                          />
                        ))}
                      </span>
                      {formatHealthRate(getUptimeRate(stats))}
                      <ChevronDown size={13} />
                    </button>
                  </dd>
                </div>
              </dl>
            </div>
            {open && stats && !health.error && (
              <GroupHealthPanel
                uptime
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
