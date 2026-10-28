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

import React, { useEffect, useRef, useState } from 'react';
import { Button, Badge, Spin } from '@douyinfe/semi-ui';
import {
  API,
  renderQuota,
  renderQuotaWithAmount,
  timestamp2string,
} from '../../helpers';
import {
  PAYMENT_STATUS_CONFIG,
  resolveOrderType,
  formatPaymentRecordMoney,
} from '../../helpers/paymentRecordView';

export default function WalletRecentPayments({
  revision,
  onHistory,
  onInvoices,
  t,
}) {
  const [rows, setRows] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const requestRef = useRef(0);
  const load = async () => {
    const request = ++requestRef.current;
    setLoading(true);
    setError(false);
    setRows([]);
    try {
      // A wallet summary always shows the current account, including administrators.
      const res = await API.get(
        '/api/user/payment-records/self?p=1&page_size=5',
        { disableDuplicate: true, skipErrorHandler: true },
      );
      if (request !== requestRef.current) return;
      if (!res.data?.success || !Array.isArray(res.data.data?.items))
        throw new Error('Invalid records');
      setRows(res.data.data.items.slice(0, 5));
    } catch {
      if (request === requestRef.current) setError(true);
    } finally {
      if (request === requestRef.current) setLoading(false);
    }
  };
  useEffect(() => {
    load();
    return () => {
      requestRef.current++;
    };
  }, [revision]);
  return (
    <section className='wallet-panel wallet-recent'>
      <div className='wallet-section-header'>
        <h2>{t('支付记录')}</h2>
        <div className='wallet-actions'>
          <Button theme='borderless' onClick={onInvoices}>
            {t('申请发票')}
          </Button>
          <Button theme='borderless' onClick={onHistory}>
            {t('账单')}
          </Button>
        </div>
      </div>
      <p className='wallet-hint'>
        {t('仅展示本人最近 5 条支付记录，完整记录与发票请查看账单。')}
      </p>
      {loading ? (
        <div className='wallet-empty'>
          <Spin />
          <span>{t('加载中...')}</span>
        </div>
      ) : error ? (
        <div className='wallet-empty' role='alert'>
          <p>{t('加载支付记录失败')}</p>
          <Button onClick={load}>{t('重试')}</Button>
        </div>
      ) : !rows.length ? (
        <div className='wallet-empty'>{t('暂无数据')}</div>
      ) : (
        <div className='wallet-records-scroll'>
          <table className='wallet-records'>
            <thead>
              <tr>
                {['时间', '类型 / 商品', '支付金额', '状态'].map((key) => (
                  <th key={key}>{t(key)}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => {
                const walletPurchase =
                  row.record_type === 'sellable_token_purchase';
                const subscription = resolveOrderType(row) === 'subscription';
                const status = PAYMENT_STATUS_CONFIG[row.status] || {
                  type: 'primary',
                  label: row.status || '—',
                };
                const known =
                  row.payment_amount_known !== false &&
                  row.money != null &&
                  Number.isFinite(Number(row.money));
                const settlement =
                  known &&
                  Number(row.paid_money) > 0 &&
                  row.paid_currency &&
                  (row.paid_currency !== (row.currency || 'CNY') ||
                    Math.abs(Number(row.paid_money) - Number(row.money)) >
                      1e-6);
                return (
                  <tr key={`${row.record_type}:${row.order_type}:${row.id}`}>
                    <td>
                      {row.create_time
                        ? timestamp2string(row.create_time)
                        : '—'}
                    </td>
                    <td>
                      <strong>
                        {t(
                          walletPurchase
                            ? '钱包购买'
                            : subscription
                              ? '订阅套餐'
                              : '在线充值',
                        )}
                      </strong>
                      {row.product_name && <small>{row.product_name}</small>}
                      {!walletPurchase &&
                        !subscription &&
                        row.amount != null && (
                          <small>
                            {t('充值额度')}: {renderQuotaWithAmount(row.amount)}
                          </small>
                        )}
                    </td>
                    <td>
                      {walletPurchase
                        ? renderQuota(row.amount)
                        : known
                          ? formatPaymentRecordMoney(
                              row.money,
                              row.currency || 'CNY',
                            )
                          : '—'}
                      {settlement && (
                        <small>
                          {t('渠道结算')}:{' '}
                          {formatPaymentRecordMoney(
                            row.paid_money,
                            row.paid_currency,
                          )}
                        </small>
                      )}
                    </td>
                    <td>
                      <span className='wallet-status'>
                        <Badge dot type={status.type} />
                        {t(status.label)}
                      </span>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
