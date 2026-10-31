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
import { usableOfficialExchangeRate } from '../../helpers/modelGroupDiscount';

export default function ModelExchangeRate({ state, t }) {
  const rate = usableOfficialExchangeRate(state.quote);
  return (
    <p
      className='model-detail-caption model-official-exchange-rate'
      role='status'
    >
      {rate ? (
        <>
          {t('参考汇率：1 USD = {{rate}} CNY · {{date}}', {
            rate: rate.toFixed(4),
            date: state.quote.date,
          })}
          {' · '}
          <a
            href='https://www.ecb.europa.eu/stats/policy_and_exchange_rates/euro_reference_exchange_rates/html/index.en.html'
            target='_blank'
            rel='noopener noreferrer'
          >
            ECB
          </a>
          {state.quote.stale && <> · {t('暂用最近有效汇率')}</>}
        </>
      ) : (
        t(
          state.loading
            ? '正在获取参考汇率…'
            : '参考汇率暂不可用，暂停跨币种价格对比',
        )
      )}
    </p>
  );
}
