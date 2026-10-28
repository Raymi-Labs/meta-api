import assert from 'node:assert/strict';
import test from 'node:test';
import {
  resolveOrderType,
  formatPaymentRecordMoney,
  PAYMENT_STATUS_CONFIG,
} from '../src/helpers/paymentRecordView.js';
test('explicit subscription and legacy subscription orders do not become wallet topups', () => {
  assert.equal(
    resolveOrderType({
      order_type: 'subscription',
      record_type: 'topup',
      amount: 0,
    }),
    'subscription',
  );
  assert.equal(
    resolveOrderType({
      record_type: 'topup',
      trade_no: 'SUB-fixture',
      amount: 0,
    }),
    'subscription',
  );
  assert.equal(
    resolveOrderType({ record_type: 'sellable_token_purchase' }),
    'sellable_token_purchase',
  );
  assert.equal(
    resolveOrderType({
      record_type: 'topup',
      trade_no: 'SUB-fixture',
      amount: 10,
    }),
    'topup',
  );
});
test('record money keeps original currency and does not convert CNY into stablecoin units', () => {
  assert.equal(formatPaymentRecordMoney(2, 'CNY'), '¥2.00');
  assert.equal(formatPaymentRecordMoney(2, 'USD'), 'US$2.00');
  assert.equal(formatPaymentRecordMoney(2, 'USDT'), '2.00 USDT');
  assert.equal(formatPaymentRecordMoney(NaN, 'USD'), 'US$0.00');
  assert.equal(formatPaymentRecordMoney('invalid', 'USD'), '-');
});
test('pending and expired payments cannot display as success', () => {
  assert.equal(PAYMENT_STATUS_CONFIG.pending.label, '待支付');
  assert.equal(PAYMENT_STATUS_CONFIG.success.label, '成功');
  assert.equal(PAYMENT_STATUS_CONFIG.expired.label, '已过期');
  assert.equal(PAYMENT_STATUS_CONFIG.cancelled.label, '已取消');
});
