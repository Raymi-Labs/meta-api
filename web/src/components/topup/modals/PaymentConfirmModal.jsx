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
import { Modal, Typography, Card, Spin } from '@douyinfe/semi-ui';
import { CreditCard } from 'lucide-react';
import { getEpayCryptoMethod } from '../../../helpers/epayMethods';
import CryptoPaymentNotice from '../CryptoPaymentNotice';
import PaymentMethodIcon from '../PaymentMethodIcon';

const { Text } = Typography;

const PaymentConfirmModal = ({
  t,
  open,
  onlineTopUp,
  handleCancel,
  confirmLoading,
  topUpCount,
  renderQuotaWithAmount,
  amountLoading,
  renderAmount,
  payWay,
  payMethods = [],
  // 新增：用于显示折扣明细
  amountNumber,
  discountRate,
}) => {
  const payMethod = payMethods.find((method) => method.type === payWay) || {
    type: payWay,
    name:
      payWay === 'alipay'
        ? t('支付宝')
        : payWay === 'wxpay'
          ? t('微信')
          : payWay === 'stripe'
            ? 'Stripe'
            : payWay,
  };
  const hasDiscount =
    discountRate && discountRate > 0 && discountRate < 1 && amountNumber > 0;
  const originalAmount = hasDiscount ? amountNumber / discountRate : 0;
  const discountAmount = hasDiscount ? originalAmount - amountNumber : 0;
  return (
    <Modal
      title={
        <div className='flex items-center'>
          <CreditCard className='mr-2' size={18} />
          {t('充值确认')}
        </div>
      }
      visible={open}
      onOk={onlineTopUp}
      onCancel={handleCancel}
      maskClosable={false}
      size='small'
      centered
      confirmLoading={confirmLoading}
    >
      <div className='space-y-4'>
        <Card className='!rounded-xl !border-0 bg-slate-50 dark:bg-slate-800'>
          <div className='space-y-3'>
            <div className='flex justify-between items-center'>
              <Text strong className='text-slate-700 dark:text-slate-200'>
                {t('充值数量')}：
              </Text>
              <Text className='text-slate-900 dark:text-slate-100'>
                {renderQuotaWithAmount(topUpCount)}
              </Text>
            </div>
            <div className='flex justify-between items-center'>
              <Text strong className='text-slate-700 dark:text-slate-200'>
                {t('预计支付金额')}：
              </Text>
              <div className='flex flex-col items-end'>
                <div className='flex items-baseline space-x-2'>
                  <Text strong className='font-bold' style={{ color: 'red' }}>
                    {renderAmount()}
                  </Text>
                  {hasDiscount && (
                    <Text size='small' className='text-rose-500'>
                      {Math.round(discountRate * 100)}%
                    </Text>
                  )}
                </div>
                <div className='mt-1 min-h-[20px] flex items-center justify-end'>
                  {amountLoading ? (
                    <>
                      <Spin size='small' />
                      <Text
                        size='small'
                        type='tertiary'
                        className='ml-2 text-right'
                      >
                        {t('价格重新计算中...')}
                      </Text>
                    </>
                  ) : null}
                </div>
              </div>
            </div>
            {hasDiscount && !amountLoading && (
              <>
                <div className='flex justify-between items-center'>
                  <Text className='text-slate-500 dark:text-slate-400'>
                    {t('原价')}：
                  </Text>
                  <Text delete className='text-slate-500 dark:text-slate-400'>
                    {renderAmount(originalAmount)}
                  </Text>
                </div>
                <div className='flex justify-between items-center'>
                  <Text className='text-slate-500 dark:text-slate-400'>
                    {t('优惠')}：
                  </Text>
                  <Text className='text-emerald-600 dark:text-emerald-400'>
                    {`- ${renderAmount(discountAmount)}`}
                  </Text>
                </div>
              </>
            )}
            <div className='flex justify-between items-center'>
              <Text strong className='text-slate-700 dark:text-slate-200'>
                {t('支付方式')}：
              </Text>
              <div className='flex items-center'>
                <PaymentMethodIcon
                  method={payMethod}
                  size={16}
                  className='mr-2'
                />
                <Text className='text-slate-900 dark:text-slate-100'>
                  {payMethod.name}
                </Text>
              </div>
            </div>
            <Text type='tertiary' size='small' className='block text-right'>
              {t('最终金额以支付页面显示为准')}
            </Text>
          </div>
        </Card>
        {getEpayCryptoMethod(payWay) && <CryptoPaymentNotice method={payWay} />}
      </div>
    </Modal>
  );
};

export default PaymentConfirmModal;
