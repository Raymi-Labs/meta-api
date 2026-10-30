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

import React, { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { ArrowUpRight, Box, RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { getLobeHubIcon } from '../../helpers';
import { formatModelTokenLimit } from '../../helpers/modelAttributes';
import { buildModelDetailPath } from '../../helpers/modelDetailNavigation';
import { getModelCatalogPricing } from '../../helpers/modelCatalogPricing';

export default function ModelInfoCard({
  model,
  modelName,
  group,
  catalogState,
}) {
  const { t } = useTranslation();
  const { catalog, loading, error, reload } = catalogState;
  const pricing = useMemo(() => {
    if (
      !model ||
      group === 'auto' ||
      !model.enable_groups?.includes(group) ||
      !Number.isFinite(catalog?.groupRatio?.[group])
    )
      return null;
    return getModelCatalogPricing(
      {
        record: model,
        selectedGroup: group,
        groupRatio: catalog.groupRatio,
        timeRatioMap: catalog.timeRatio,
        currency: 'USD',
        quotaDisplayType: 'USD',
        tokenUnit: 'M',
        displayPrice: (value) => `$${Number(value).toFixed(6)}`,
      },
      t,
    );
  }, [model, group, catalog, t]);
  const icon = model?.icon || model?.vendor?.icon;
  const rows = [
    {
      key: 'context',
      label: t('上下文长度'),
      value: formatModelTokenLimit(model?.context_length),
    },
    {
      key: 'output-limit',
      label: t('最大输出'),
      value: formatModelTokenLimit(model?.max_output_tokens),
    },
    ...(pricing?.items || []),
  ];
  return (
    <section
      className='pg-settings-section pg-model-card'
      aria-label={t('模型信息')}
    >
      <div className='pg-model-card-heading'>
        <div className='pg-model-logo'>
          {icon ? getLobeHubIcon(icon, 34) : <Box size={30} />}
        </div>
        <div className='pg-model-identity'>
          <h2 title={modelName}>
            {model?.display_name || modelName || t('请选择模型')}
          </h2>
          <span>{model?.vendor?.name || t('未知厂商')}</span>
        </div>
        {modelName && (
          <Link
            to={buildModelDetailPath(modelName)}
            target='_blank'
            rel='noopener noreferrer'
            className='pg-model-details'
          >
            {t('详情')}
            <ArrowUpRight size={13} />
          </Link>
        )}
      </div>
      <dl className='pg-model-facts'>
        {rows.map((row) => {
          const raw = pricing?.data?.unitPricesUSD?.[row.key];
          const value =
            typeof raw === 'number' && Number.isFinite(raw)
              ? `$${Number(raw.toFixed(6))}`
              : row.value;
          return (
            <div key={row.key}>
              <dt>{row.label}</dt>
              <dd>
                {value}
                <small>{row.suffix?.replace(' / 1M Tokens', '/1M')}</small>
              </dd>
            </div>
          );
        })}
      </dl>
      <div className='pg-model-price-note'>
        {loading ? (
          t('加载中...')
        ) : error ? (
          <button onClick={reload}>
            <RefreshCw size={12} />
            {t('加载模型信息失败，点击重试')}
          </button>
        ) : pricing ? (
          <>
            {group} · USD
            {pricing.data.isDynamicPricing && (
              <>
                {' '}
                · {t('动态计费')} ·{' '}
                {pricing.data.firstTierLabel || t('见价格详情')}
              </>
            )}
          </>
        ) : group === 'auto' ? (
          t('自动分组价格以实际路由为准')
        ) : (
          t('暂无当前分组价格')
        )}
      </div>
    </section>
  );
}
