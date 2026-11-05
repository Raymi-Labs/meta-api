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

import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Typography,
  Tag,
  Card,
  Button,
  Banner,
  Form,
  Skeleton,
  Tabs,
  TabPane,
} from '@douyinfe/semi-ui';
import { ArrowRight, Wallet, Sparkles } from 'lucide-react';
import SubscriptionPlansCard from './SubscriptionPlansCard';
import CryptoPaymentNotice from './CryptoPaymentNotice';
import PaymentMethodIcon from './PaymentMethodIcon';
import PaymentCostReference from './PaymentCostReference';
import { getEpayCryptoMethod } from '../../helpers/epayMethods';
import { formatPaymentMoney } from '../../helpers/paymentCurrency';
import {
  getWalletCostReference,
  isStripeBelowMinimum,
  walletQuoteProvider,
} from '../../helpers/walletPaymentCosts';
import { useWalletComparisonQuotes } from '../../hooks/common/useWalletComparisonQuotes';

const { Text } = Typography;

const RechargeCard = ({
  t,
  enableOnlineTopUp,
  enableStripeTopUp,
  enableCreemTopUp,
  enableWaffoTopUp,
  creemProducts,
  creemPreTopUp,
  presetAmounts,
  selectPresetAmount,
  topUpCount,
  minTopUp,
  renderQuotaWithAmount,
  setTopUpCount,
  setSelectedPreset,
  renderAmount,
  amountLoading,
  payMethods,
  selectPayMethod,
  openPaymentConfirm,
  paymentLoading,
  payWay,
  paymentQuote,
  statusLoading,
  topupInfo,
  overview,
  topupInfoError = false,
  onRetryTopupInfo,
  subscriptionLoading = false,
  subscriptionPlans = [],
  billingPreference,
  onChangeBillingPreference,
  activeSubscriptions = [],
  activeQuantityByPlan = {},
  allSubscriptions = [],
  reloadSubscriptionSelf,
}) => {
  const onlineFormApiRef = useRef(null);
  const [activeTab, setActiveTab] = useState('topup');
  const shouldShowSubscription =
    !subscriptionLoading && subscriptionPlans.length > 0;
  const methods = useMemo(
    () =>
      payMethods.filter((method) => {
        const provider = walletQuoteProvider(method.type);
        return provider === 'stripe'
          ? enableStripeTopUp
          : provider === 'waffo'
            ? enableWaffoTopUp
            : enableOnlineTopUp;
      }),
    [payMethods, enableStripeTopUp, enableWaffoTopUp, enableOnlineTopUp],
  );
  const comparison = useWalletComparisonQuotes(
    topUpCount,
    methods,
    !statusLoading && !topupInfoError,
  );
  const stripeQuote = payWay === 'stripe' ? paymentQuote : comparison.stripe;
  const costReference = getWalletCostReference(
    topupInfo?.cost_reference,
    stripeQuote,
  );
  const selectedMethod = methods.find((method) => method.type === payWay);
  const belowMinimum = isStripeBelowMinimum(payWay, paymentQuote);
  const hasQuotedAmount = Number(paymentQuote?.amount) > 0;
  const hasOnlineMethods =
    enableOnlineTopUp || enableStripeTopUp || enableWaffoTopUp;

  useEffect(() => {
    if (!shouldShowSubscription) setActiveTab('topup');
  }, [shouldShowSubscription]);
  useEffect(() => {
    onlineFormApiRef.current?.setValue('topUpCount', topUpCount);
  }, [topUpCount]);

  const costLabel = (method) => {
    if (method.type === 'stripe')
      return costReference
        ? t('历史参考价差约 {{percent}}% · 非固定费率', {
            percent: costReference.difference_percent.toFixed(2),
          })
        : t('支付币种及换汇金额以收银台为准');
    if (getEpayCryptoMethod(method.type))
      return t('订单按人民币计价，链上金额以收银台为准');
    return walletQuoteProvider(method.type) === 'epay'
      ? t('本站不另加充值手续费')
      : t('费用与金额以收银台为准');
  };

  const topupContent = (
    <div className='wallet-recharge-form'>
      {statusLoading ? (
        <div className='py-8' role='status' aria-label={t('加载中')}>
          <Skeleton.Title active style={{ width: '40%' }} />
          <Skeleton.Paragraph active rows={3} style={{ marginTop: 16 }} />
        </div>
      ) : topupInfoError ? (
        <div role='alert' className='py-8 flex flex-col items-center gap-4'>
          <Text type='secondary'>{t('加载失败')}</Text>
          <Button onClick={onRetryTopupInfo}>{t('重试')}</Button>
        </div>
      ) : hasOnlineMethods || enableCreemTopUp ? (
        <Form
          getFormApi={(api) => (onlineFormApiRef.current = api)}
          initValues={{ topUpCount }}
        >
          {hasOnlineMethods && (
            <>
              <Form.Slot label={t('选择充值额度')}>
                <div className='wallet-preset-grid'>
                  {presetAmounts.map((preset) => {
                    const discount =
                      preset.discount ||
                      topupInfo?.discount?.[preset.value] ||
                      1;
                    return (
                      <button
                        key={preset.value}
                        type='button'
                        className='wallet-preset'
                        aria-pressed={
                          Number(topUpCount) === Number(preset.value)
                        }
                        onClick={() => selectPresetAmount(preset)}
                      >
                        <strong>{renderQuotaWithAmount(preset.value)}</strong>
                        {discount < 1 && (
                          <Tag color='green'>
                            {t('优惠')} {Math.round((1 - discount) * 100)}%
                          </Tag>
                        )}
                      </button>
                    );
                  })}
                </div>
              </Form.Slot>
              <Form.InputNumber
                field='topUpCount'
                label={t('自定义充值额度')}
                min={minTopUp}
                max={999999999}
                step={1}
                precision={0}
                placeholder={
                  t('充值数量，最低 ') + renderQuotaWithAmount(minTopUp)
                }
                onChange={(value) => {
                  if (value && value >= 1) {
                    setTopUpCount(value);
                    setSelectedPreset(null);
                  }
                }}
                onBlur={(event) => {
                  const value = parseInt(event.target.value);
                  if (!value || value < minTopUp) setTopUpCount(minTopUp);
                }}
                style={{ width: '100%' }}
              />
              <Form.Slot label={t('选择支付方式')}>
                <div className='wallet-payment-methods'>
                  {methods.map((method) => {
                    const selected = payWay === method.type;
                    const min = Number(method.min_topup) || 0;
                    const quote = selected
                      ? paymentQuote
                      : comparison[walletQuoteProvider(method.type)];
                    const disabled = min > Number(topUpCount || 0);
                    return (
                      <button
                        key={method.type}
                        type='button'
                        className='wallet-payment-method'
                        aria-pressed={selected}
                        disabled={disabled}
                        onClick={() => selectPayMethod(method.type)}
                      >
                        <span className='wallet-payment-icon'>
                          <PaymentMethodIcon method={method} size={24} />
                        </span>
                        <span className='wallet-payment-description'>
                          <strong>{method.name}</strong>
                          <span>{costLabel(method)}</span>
                          {disabled && (
                            <span>
                              {t('此支付方式最低充值金额为')}{' '}
                              {renderQuotaWithAmount(min)}
                            </span>
                          )}
                        </span>
                        <span className='wallet-payment-price'>
                          <strong>
                            {selected && amountLoading
                              ? t('计算中')
                              : quote
                                ? formatPaymentMoney(
                                    quote.amount,
                                    quote.currency,
                                  )
                                : '—'}
                          </strong>
                          <span>{t('预计支付金额')}</span>
                        </span>
                      </button>
                    );
                  })}
                </div>
                {methods.length === 0 && (
                  <Text type='secondary'>
                    {t('暂无可用的支付方式，请联系管理员配置')}
                  </Text>
                )}
                <PaymentCostReference
                  reference={costReference}
                  t={t}
                  renderQuotaWithAmount={renderQuotaWithAmount}
                />
              </Form.Slot>
              {getEpayCryptoMethod(payWay) && (
                <CryptoPaymentNotice method={payWay} />
              )}
              {belowMinimum && (
                <Banner
                  type='warning'
                  closeIcon={null}
                  description={t(
                    '{{method}} 最低支付金额为 US$0.50，请调整充值数量，使预计支付金额不低于 US$0.50。',
                    { method: selectedMethod?.name || 'Stripe' },
                  )}
                />
              )}
              <div className='wallet-payment-summary' aria-live='polite'>
                <div>
                  <span>{t('到账额度')}</span>
                  <strong>
                    {renderQuotaWithAmount(Number(topUpCount || 0))}
                  </strong>
                </div>
                <div>
                  <span>{t('本站另加手续费')}</span>
                  <span>
                    {selectedMethod
                      ? walletQuoteProvider(payWay) === 'waffo'
                        ? t('以收银台为准')
                        : t('不另收')
                      : '—'}
                  </span>
                </div>
                {payWay === 'stripe' && paymentQuote?.currency === 'USD' && (
                  <div>
                    <span>{t('人民币实付')}</span>
                    <span>{t('以收银台换汇金额为准')}</span>
                  </div>
                )}
                <div className='wallet-payment-total'>
                  <span>{t('预计支付金额')}</span>
                  <strong>
                    {amountLoading
                      ? t('价格重新计算中...')
                      : payWay
                        ? renderAmount()
                        : t('请选择支付方式')}
                  </strong>
                </div>
              </div>
              <Button
                className='wallet-pay-button'
                theme='solid'
                type='primary'
                block
                onClick={openPaymentConfirm}
                disabled={
                  !selectedMethod ||
                  amountLoading ||
                  !hasQuotedAmount ||
                  belowMinimum
                }
                loading={paymentLoading}
              >
                {t('继续支付')}
                {selectedMethod ? ` · ${selectedMethod.name}` : ''}
                <ArrowRight size={16} />
              </Button>
              <p className='wallet-payment-disclaimer'>
                {t('最终金额以支付页面显示为准')}
              </p>
              {!amountLoading && selectedMethod && !hasQuotedAmount && (
                <Button block onClick={openPaymentConfirm}>
                  {t('重新获取报价')}
                </Button>
              )}
            </>
          )}
          {enableCreemTopUp && creemProducts.length > 0 && (
            <Form.Slot label={t('Creem 充值')}>
              <div className='wallet-preset-grid'>
                {creemProducts.map((product) => (
                  <button
                    type='button'
                    className='wallet-preset'
                    key={product.productId}
                    onClick={() => creemPreTopUp(product)}
                  >
                    <strong>{product.name}</strong>
                    <span>
                      {t('充值额度')}: {product.quota}
                    </span>
                    <span>
                      {formatPaymentMoney(
                        product.price,
                        product.currency || 'USD',
                      )}
                    </span>
                  </button>
                ))}
              </div>
            </Form.Slot>
          )}
        </Form>
      ) : (
        <Banner
          type='info'
          closeIcon={null}
          description={t(
            '管理员未开启在线充值功能，请联系管理员开启或使用兑换码充值。',
          )}
        />
      )}
    </div>
  );

  return (
    <div className='wallet-primary-stack'>
      {overview}
      <Card className='wallet-recharge-card' title={<h2>{t('账户充值')}</h2>}>
        {shouldShowSubscription ? (
          <Tabs type='line' activeKey={activeTab} onChange={setActiveTab}>
            <TabPane
              tab={
                <span className='wallet-tab-label'>
                  <Wallet size={16} />
                  {t('额度充值')}
                </span>
              }
              itemKey='topup'
            >
              {topupContent}
            </TabPane>
            <TabPane
              tab={
                <span className='wallet-tab-label'>
                  <Sparkles size={16} />
                  {t('订阅套餐')}
                </span>
              }
              itemKey='subscription'
            >
              <SubscriptionPlansCard
                t={t}
                loading={subscriptionLoading}
                plans={subscriptionPlans}
                payMethods={payMethods}
                enableOnlineTopUp={enableOnlineTopUp}
                enableStripeTopUp={enableStripeTopUp}
                enableCreemTopUp={enableCreemTopUp}
                billingPreference={billingPreference}
                onChangeBillingPreference={onChangeBillingPreference}
                activeSubscriptions={activeSubscriptions}
                activeQuantityByPlan={activeQuantityByPlan}
                allSubscriptions={allSubscriptions}
                reloadSubscriptionSelf={reloadSubscriptionSelf}
                withCard={false}
              />
            </TabPane>
          </Tabs>
        ) : (
          topupContent
        )}
      </Card>
    </div>
  );
};
export default RechargeCard;
