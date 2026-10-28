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

export const PAYMENT_STATUS_CONFIG = {
  success: { type: 'success', label: '成功' },
  pending: { type: 'warning', label: '待支付' },
  expired: { type: 'danger', label: '已过期' },
  cancelled: { type: 'tertiary', label: '已取消' },
};

export function resolveOrderType(record) {
  if (!record) {
    return '';
  }
  if (record.order_type) {
    return record.order_type;
  }
  const tradeNo = String(record.trade_no || '').toLowerCase();
  if (Number(record.amount || 0) === 0 && tradeNo.startsWith('sub')) {
    return 'subscription';
  }
  return record.record_type || 'topup';
}

export function formatPaymentRecordMoney(value, currency = 'CNY') {
  const amount = Number(value || 0);
  if (!Number.isFinite(amount)) {
    return '-';
  }
  const upperCurrency = String(currency || '').toUpperCase();
  const symbolMap = {
    CNY: '¥',
    RMB: '¥',
    USD: 'US$',
    EUR: '€',
    GBP: '£',
  };
  const symbol = symbolMap[upperCurrency] || '';
  return `${symbol}${amount.toFixed(2)}${upperCurrency && !symbol ? ` ${upperCurrency}` : ''}`;
}
