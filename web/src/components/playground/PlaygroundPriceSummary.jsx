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
import {
  playgroundMoney,
  readPlaygroundCurrency,
} from '../../helpers/playgroundPresentation';

// Reuse catalog prices and units; never turn this into an estimate of the
// current request (output length, cache hits and final route are unknown).
export default function PlaygroundPriceSummary({
  model,
  group,
  catalogState,
  compact = false,
}) {
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
  const currency = readPlaygroundCurrency();
  if (compact) {
    if (pricing.data.isDynamicPricing)
      return <span className='pg-compact-price'>{t('动态计费')}</span>;
    const amounts = items.map((item) => ({
      ...item,
      money: playgroundMoney(
        pricing.data.unitPricesUSD?.[item.key],
        currency.status,
        currency.displayType,
      ),
    }));
    const description = amounts
      .map(
        (item) =>
          `${item.label} ${item.money?.primary || item.value}${item.suffix || ''}`,
      )
      .join(' · ');
    return (
      <span
        className='pg-compact-price'
        title={description}
        aria-label={description}
      >
        {amounts.map((item) => item.money?.primary || item.value).join(' / ')}
      </span>
    );
  }
  return (
    <div
      className='pg-top-prices'
      title={items
        .map((x) => {
          const money = playgroundMoney(
            pricing.data.unitPricesUSD?.[x.key],
            currency.status,
            currency.displayType,
          );
          return `${x.label}: ${money?.primary || x.value} ${money?.cny || ''}${x.suffix || ''}`;
        })
        .join(' · ')}
    >
      {pricing.data.isDynamicPricing ? (
        <span>
          {t('动态计费')} · {t('见价格详情')}
        </span>
      ) : (
        items.map((item) => {
          const money = playgroundMoney(
            pricing.data.unitPricesUSD?.[item.key],
            currency.status,
            currency.displayType,
          );
          return (
            <span key={item.key}>
              {item.label} <strong>{money?.primary || item.value}</strong>
              {money?.cny && (
                <small
                  title={t(
                    '人民币按标准充值比例换算，仅供参考，不含套餐或充值优惠。',
                  )}
                >
                  {money.cny}
                </small>
              )}
              <small>{item.suffix}</small>
            </span>
          );
        })
      )}
    </div>
  );
}
