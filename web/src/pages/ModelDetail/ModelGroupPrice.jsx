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
import { compareGroupPrice } from '../../helpers/modelGroupDiscount';
import { formatOfficialPrice } from '../../helpers/modelOfficialPricing';

export default function ModelGroupPrice({
  item,
  pricing,
  model,
  officialTier,
  data,
}) {
  const { t } = data;
  const comparison = compareGroupPrice({
    item,
    pricing,
    model,
    officialTier,
    currency: data.currency,
    displayPrice: data.displayPrice,
    tokenUnit: data.tokenUnit,
    quotaDisplayType: data.siteDisplayType,
    exchangeQuote: data.officialExchangeRate?.quote,
  });
  const label =
    comparison?.kind === 'equal'
      ? t('与官方持平')
      : comparison?.kind === 'higher' && comparison.percent === null
        ? t('高于官方')
        : comparison
          ? t(
              comparison.kind === 'lower'
                ? '较官方低 {{percent}}%'
                : '较官方高 {{percent}}%',
              { percent: comparison.percent },
            )
          : '';
  return (
    <span className='model-group-price-item' data-price-key={item.key}>
      <small className='model-group-price-label'>{item.label}</small>
      <span className='model-group-price-amounts'>
        {comparison &&
          (comparison.kind === 'lower' ? (
            <del aria-label={t('官方参考价')}>
              {formatOfficialPrice(comparison.official, data.currency, 6)}
            </del>
          ) : (
            <span
              className='model-group-reference-price'
              aria-label={t('官方参考价')}
            >
              {formatOfficialPrice(comparison.official, data.currency, 6)}
            </span>
          ))}
        <strong>
          {comparison
            ? formatOfficialPrice(comparison.group, data.currency)
            : item.value}
        </strong>
        <small>{item.suffix}</small>
      </span>
      {comparison && (
        <span
          className={`model-group-discount model-group-discount-${comparison.kind}`}
        >
          {label}
        </span>
      )}
    </span>
  );
}
