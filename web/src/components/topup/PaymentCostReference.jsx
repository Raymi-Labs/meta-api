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
import { formatPaymentMoney } from '../../helpers/paymentCurrency';

export default function PaymentCostReference({
  reference,
  t,
  renderQuotaWithAmount,
}) {
  if (!reference) return null;
  const money = (amount, currency = 'CNY') =>
    formatPaymentMoney(amount, currency);
  return (
    <details className='wallet-cost-reference'>
      <summary>{t('费用说明与参考依据')}</summary>
      <p>
        {t('历史参考价差约 {{percent}}%，非固定手续费，也不会额外叠加收取。', {
          percent: reference.difference_percent.toFixed(2),
        })}
      </p>
      <p>
        {t(
          '最近 {{count}} 笔同额成功订单，充值额度 {{amount}}，原币金额 {{usd}}，人民币平均实付 {{paid}}；按当前支付宝报价比较，同额为 {{baseline}}。',
          {
            count: reference.sample_count,
            amount: renderQuotaWithAmount(reference.sample_amount),
            usd: money(reference.reference_usd, 'USD'),
            paid: money(reference.reference_paid),
            baseline: money(reference.baseline_amount),
          },
        )}
      </p>
      <p>
        {t(
          '最新样本日期：{{date}}。此数据是支付通道历史参考，不代表本次换汇报价或商户结算手续费。',
          { date: new Date(reference.observed_at * 1000).toLocaleDateString() },
        )}
      </p>
    </details>
  );
}
