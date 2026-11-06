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
import { useTranslation } from 'react-i18next';
import { getModelCatalogPricing } from '../../helpers/modelCatalogPricing';

// Reuse catalog prices and units; never turn this into an estimate of the
// current request (output length, cache hits and final route are unknown).
export default function PlaygroundPriceSummary({ model, group, catalogState }) {
  const { t } = useTranslation();
  const catalog = catalogState.catalog;
  if (
    catalogState.loading ||
    catalogState.error ||
    !model ||
    group === 'auto' ||
    !model.enable_groups?.includes(group) ||
    !Number.isFinite(catalog?.groupRatio?.[group])
  )
    return null;
  const pricing = getModelCatalogPricing(
    {
      record: model,
      selectedGroup: group,
      groupRatio: catalog.groupRatio,
      timeRatioMap: catalog.timeRatio,
      currency: 'USD',
      quotaDisplayType: 'USD',
      tokenUnit: 'M',
      displayPrice: (value) => `$${Number(value.toFixed(6))}`,
    },
    t,
  );
  const items = pricing.items.slice(0, 2);
  return (
    <div
      className='pg-top-prices'
      title={items
        .map((x) => `${x.label}: ${x.value}${x.suffix || ''}`)
        .join(' · ')}
    >
      {pricing.data.isDynamicPricing ? (
        <span>
          {t('动态计费')} · {t('见价格详情')}
        </span>
      ) : (
        items.map((item) => (
          <span key={item.key}>
            {item.label} <strong>{item.value}</strong>
            <small>{item.suffix}</small>
          </span>
        ))
      )}
    </div>
  );
}
