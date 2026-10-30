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
import { ArrowUpRight } from 'lucide-react';
import {
  officialPriceColumns,
  OFFICIAL_PRICE_UNITS,
  formatOfficialPrice,
  officialPricingSource,
} from '../../helpers/modelOfficialPricing';

export default function ModelOfficialPricing({ pricing, t }) {
  const tiers = Array.isArray(pricing?.tiers) ? pricing.tiers : [];
  const columns = officialPriceColumns(tiers);
  const source = officialPricingSource(pricing?.source_url);
  return (
    <section
      id='official-pricing'
      className='model-detail-section model-official-pricing'
    >
      <div className='model-detail-section-heading'>
        <h2>{t('官方参考价')}</h2>
        {source && (
          <a href={source} target='_blank' rel='noopener noreferrer'>
            {t('价格来源')}
            <ArrowUpRight size={15} />
          </a>
        )}
      </div>
      <p className='model-detail-caption'>
        {t('官方参考价仅供对照，实际费用以本站分组价格为准。')}
      </p>
      {tiers.length && columns.length ? (
        <div
          className='model-official-table-wrap'
          tabIndex={0}
          role='region'
          aria-label={t('官方参考价')}
        >
          <table>
            <thead>
              <tr>
                <th>{t('档位')}</th>
                {columns.map(([key, label]) => (
                  <th key={key}>{t(label)}</th>
                ))}
                <th>{t('单位')}</th>
              </tr>
            </thead>
            <tbody>
              {tiers.map((tier, index) => (
                <tr key={index}>
                  <th scope='row'>{tier.label}</th>
                  {columns.map(([key]) => (
                    <td key={key}>
                      {formatOfficialPrice(tier[key], pricing.currency)}
                    </td>
                  ))}
                  <td>
                    {pricing.currency} /{' '}
                    {OFFICIAL_PRICE_UNITS[tier.unit] || tier.unit}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <p className='model-official-empty'>{t('暂无官方参考价')}</p>
      )}
      {pricing?.notes && (
        <p className='model-detail-caption model-official-notes'>
          {pricing.notes}
        </p>
      )}
    </section>
  );
}
