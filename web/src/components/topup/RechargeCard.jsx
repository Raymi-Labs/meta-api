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
import {
  Avatar,
  Typography,
  Tag,
  Card,
  Button,
  Banner,
  Form,
  Space,
  Row,
  Col,
  Spin,
  Skeleton,
  Tooltip,
  Tabs,
  TabPane,
} from '@douyinfe/semi-ui';
import { CreditCard, Coins, Wallet, Sparkles, Receipt } from 'lucide-react';

import { getCurrencyConfig, getQuotaPerUnit } from '../../helpers/render';
import SubscriptionPlansCard from './SubscriptionPlansCard';
import CryptoPaymentNotice from './CryptoPaymentNotice';
import PaymentMethodIcon from './PaymentMethodIcon';
import { getEpayCryptoMethod } from '../../helpers/epayMethods';

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
  selectedPreset,
  selectPresetAmount,
  formatLargeNumber,
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
  renderQuota,
  statusLoading,
  topupInfo,
  overview,
  onOpenHistory,
  onOpenInvoices,
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
  const shouldShowTabs = shouldShowSubscription;

  useEffect(() => {
    if (!shouldShowTabs && activeTab !== 'topup') {
      setActiveTab('topup');
    }
    if (!shouldShowSubscription && activeTab === 'subscription') {
      setActiveTab('topup');
    }
  }, [shouldShowSubscription, shouldShowTabs, activeTab]);
  const hasSelectedPayMethod = Boolean(payWay);
  const topupContent = (
    <Space vertical style={{ width: '100%' }}>
      <div className='wallet-recharge-form'>
        {/* 在线充值表单 */}
        {statusLoading ? (
          <div className='py-8' role='status' aria-label={t('加载中')}>
            <Skeleton.Title active style={{ width: '40%' }} />
            <Skeleton.Paragraph active rows={3} style={{ marginTop: 16 }} />
            <Skeleton.Button active style={{ width: '100%', marginTop: 16 }} />
          </div>
        ) : topupInfoError ? (
          <div role='alert' className='py-8 flex flex-col items-center gap-4'>
            <Text type='secondary'>{t('加载失败')}</Text>
            <Button onClick={onRetryTopupInfo}>{t('重试')}</Button>
          </div>
        ) : enableOnlineTopUp ||
          enableStripeTopUp ||
          enableCreemTopUp ||
          enableWaffoTopUp ? (
          <Form
            getFormApi={(api) => (onlineFormApiRef.current = api)}
            initValues={{ topUpCount: topUpCount }}
          >
            <div className='space-y-6'>
              {(enableOnlineTopUp || enableStripeTopUp || enableWaffoTopUp) && (
                <Row gutter={12}>
                  <Col xs={24} sm={24} md={24} lg={10} xl={10}>
                    <Form.InputNumber
                      field='topUpCount'
                      label={t('充值数量')}
                      disabled={
                        !enableOnlineTopUp &&
                        !enableStripeTopUp &&
                        !enableWaffoTopUp
                      }
                      placeholder={
                        t('充值数量，最低 ') + renderQuotaWithAmount(minTopUp)
                      }
                      value={topUpCount}
                      min={minTopUp}
                      max={999999999}
                      step={1}
                      precision={0}
                      onChange={(value) => {
                        if (value && value >= 1) {
                          setTopUpCount(value);
                          setSelectedPreset(null);
                        }
                      }}
                      onBlur={(e) => {
                        const value = parseInt(e.target.value);
                        if (!value || value < minTopUp) {
                          setTopUpCount(minTopUp);
                          return;
                        }
                      }}
                      formatter={(value) => (value ? `${value}` : '')}
                      parser={(value) =>
                        value ? parseInt(value.replace(/[^\d]/g, '')) : 0
                      }
                      extraText={
                        <div className='min-h-[44px]'>
                          <div className='mb-1'>
                            <Text type='secondary'>
                              {t('到账额度：')}
                              {renderQuota(
                                getCurrencyConfig().type === 'TOKENS'
                                  ? Number(topUpCount || 0)
                                  : Number(topUpCount || 0) * getQuotaPerUnit(),
                              )}
                            </Text>
                          </div>
                          <Text type='secondary'>
                            {t('预计支付金额：')}
                            <span className='wallet-quoted-amount'>
                              {amountLoading
                                ? t('价格重新计算中...')
                                : hasSelectedPayMethod
                                  ? renderAmount()
                                  : t('请选择支付方式')}
                            </span>
                          </Text>
                          {payWay === 'stripe' && (
                            <div
                              className='mt-2 rounded-lg border px-3 py-2'
                              style={{
                                backgroundColor:
                                  'var(--semi-color-danger-light-default)',
                                borderColor: 'var(--semi-color-danger)',
                              }}
                            >
                              <Text type='danger' strong>
                                {t(
                                  '{{method}} 最低支付金额为 US$0.50，请调整充值数量，使预计支付金额不低于 US$0.50。',
                                  {
                                    method:
                                      payMethods.find(
                                        (method) => method.type === 'stripe',
                                      )?.name || 'Stripe',
                                  },
                                )}
                              </Text>
                            </div>
                          )}
                          <div className='mt-1 h-5 flex items-center'>
                            {amountLoading ? (
                              <>
                                <Spin size='small' />
                                <Text
                                  type='quaternary'
                                  size='small'
                                  className='ml-2'
                                >
                                  {t('价格重新计算中...')}
                                </Text>
                              </>
                            ) : null}
                          </div>
                        </div>
                      }
                      style={{ width: '100%' }}
                    />
                  </Col>
                  <Col xs={24} sm={24} md={24} lg={14} xl={14}>
                    <Form.Slot label={t('选择支付方式')}>
                      {payMethods && payMethods.length > 0 ? (
                        <div className='flex flex-col items-start gap-2'>
                          <Space wrap>
                            {payMethods.map((payMethod) => {
                              const minTopupVal =
                                Number(payMethod.min_topup) || 0;
                              const isStripe = payMethod.type === 'stripe';
                              const isWaffo =
                                typeof payMethod.type === 'string' &&
                                (payMethod.type === 'waffo' ||
                                  payMethod.type.startsWith('waffo:'));
                              const isSelected = payWay === payMethod.type;
                              const disabled =
                                (!enableOnlineTopUp && !isStripe && !isWaffo) ||
                                (!enableStripeTopUp && isStripe) ||
                                (!enableWaffoTopUp && isWaffo) ||
                                minTopupVal > Number(topUpCount || 0) ||
                                amountLoading;

                              const buttonEl = (
                                <Button
                                  key={payMethod.type}
                                  theme={isSelected ? 'solid' : 'outline'}
                                  type={isSelected ? 'primary' : 'tertiary'}
                                  onClick={() =>
                                    selectPayMethod(payMethod.type)
                                  }
                                  disabled={disabled}
                                  aria-pressed={isSelected}
                                  icon={
                                    <PaymentMethodIcon method={payMethod} />
                                  }
                                  className='!rounded-lg !px-4 !py-2 transition-colors'
                                >
                                  {payMethod.name}
                                </Button>
                              );

                              return disabled &&
                                minTopupVal > Number(topUpCount || 0) ? (
                                <Tooltip
                                  content={
                                    t('此支付方式最低充值金额为') +
                                    ' ' +
                                    minTopupVal
                                  }
                                  key={payMethod.type}
                                >
                                  {buttonEl}
                                </Tooltip>
                              ) : (
                                <React.Fragment key={payMethod.type}>
                                  {buttonEl}
                                </React.Fragment>
                              );
                            })}
                          </Space>
                          <Text type='tertiary' size='small'>
                            {t('预计支付金额会按当前支付方式实时计算')}
                          </Text>
                        </div>
                      ) : (
                        <div className='text-gray-500 text-sm p-3 bg-gray-50 rounded-lg border border-dashed border-gray-300'>
                          {t('暂无可用的支付方式，请联系管理员配置')}
                        </div>
                      )}
                    </Form.Slot>
                  </Col>
                  {getEpayCryptoMethod(payWay) && (
                    <Col span={24} className='mb-4'>
                      <CryptoPaymentNotice method={payWay} />
                    </Col>
                  )}
                </Row>
              )}

              {(enableOnlineTopUp || enableStripeTopUp || enableWaffoTopUp) && (
                <Form.Slot
                  label={
                    <div className='flex items-center gap-2'>
                      <span>{t('选择充值额度')}</span>
                      {(() => {
                        const { symbol, rate, type } = getCurrencyConfig();
                        if (type === 'USD') return null;

                        return (
                          <span
                            style={{
                              color: 'var(--semi-color-text-2)',
                              fontSize: '12px',
                              fontWeight: 'normal',
                            }}
                          >
                            (1 $ = {rate.toFixed(2)} {symbol})
                          </span>
                        );
                      })()}
                    </div>
                  }
                >
                  <div className='grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-2'>
                    {presetAmounts.map((preset, index) => {
                      const discount =
                        preset.discount ||
                        topupInfo?.discount?.[preset.value] ||
                        1.0;
                      const hasDiscount = discount < 1.0;

                      // 按当前货币模式转换展示的金额与份数。
                      const { symbol, rate, type } = getCurrencyConfig();
                      const statusStr = localStorage.getItem('status');
                      let usdRate = 7; // Default CNY exchange rate
                      try {
                        if (statusStr) {
                          const s = JSON.parse(statusStr);
                          usdRate = s?.usd_exchange_rate || 7;
                        }
                      } catch (e) {}

                      let displayValue = preset.value; // Displayed quantity

                      if (type === 'USD') {
                        // 数量保持 USD。
                      } else if (type === 'CNY') {
                        // 将数量换算为 CNY；价格本身已是 CNY。
                        displayValue = preset.value * usdRate;
                      } else if (type === 'CUSTOM') {
                        // 将数量换算为自定义货币。
                        displayValue = preset.value * rate;
                      }

                      return (
                        <Card
                          key={index}
                          style={{
                            cursor: 'pointer',
                            border:
                              selectedPreset === preset.value
                                ? '2px solid var(--semi-color-primary)'
                                : '1px solid var(--semi-color-border)',
                            height: '100%',
                            width: '100%',
                          }}
                          bodyStyle={{ padding: '12px' }}
                          onClick={() => {
                            selectPresetAmount(preset);
                            onlineFormApiRef.current?.setValue(
                              'topUpCount',
                              preset.value,
                            );
                          }}
                        >
                          <div style={{ textAlign: 'center' }}>
                            <Typography.Title
                              heading={6}
                              style={{ margin: '0 0 8px 0' }}
                            >
                              <Coins size={18} />
                              {formatLargeNumber(displayValue)} {symbol}
                              {hasDiscount && (
                                <Tag style={{ marginLeft: 4 }} color='green'>
                                  {t('折').includes('off')
                                    ? (
                                        (1 - parseFloat(discount)) *
                                        100
                                      ).toFixed(1)
                                    : (discount * 10).toFixed(1)}
                                  {t('折')}
                                </Tag>
                              )}
                            </Typography.Title>
                            <div
                              style={{
                                color: 'var(--semi-color-text-2)',
                                fontSize: '12px',
                                margin: '4px 0',
                              }}
                            >
                              {t('点击后按当前支付方式计算实付金额')}
                            </div>
                          </div>
                        </Card>
                      );
                    })}
                  </div>
                </Form.Slot>
              )}

              {(enableOnlineTopUp || enableStripeTopUp || enableWaffoTopUp) && (
                <div className='flex justify-end'>
                  <Button
                    theme='solid'
                    type='primary'
                    onClick={openPaymentConfirm}
                    disabled={!hasSelectedPayMethod || amountLoading}
                    loading={paymentLoading}
                    icon={<CreditCard size={16} />}
                    className='!rounded-lg !px-5'
                  >
                    {t('充值')}
                  </Button>
                </div>
              )}

              {/* Creem 充值区 */}
              {enableCreemTopUp && creemProducts.length > 0 && (
                <Form.Slot label={t('Creem 充值')}>
                  <div className='grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-3'>
                    {creemProducts.map((product, index) => (
                      <Card
                        key={index}
                        onClick={() => creemPreTopUp(product)}
                        className='cursor-pointer !rounded-2xl transition-all hover:shadow-md border-gray-200 hover:border-gray-300'
                        bodyStyle={{ textAlign: 'center', padding: '16px' }}
                      >
                        <div className='font-medium text-lg mb-2'>
                          {product.name}
                        </div>
                        <div className='text-sm text-gray-600 mb-2'>
                          {t('充值额度')}: {product.quota}
                        </div>
                        <div className='text-lg font-semibold text-blue-600'>
                          {product.currency === 'EUR' ? '€' : '$'}
                          {product.price}
                        </div>
                      </Card>
                    ))}
                  </div>
                </Form.Slot>
              )}
            </div>
          </Form>
        ) : (
          <Banner
            type='info'
            description={t(
              '管理员未开启在线充值功能，请联系管理员开启或使用兑换码充值。',
            )}
            className='!rounded-xl'
            closeIcon={null}
          />
        )}
      </div>
    </Space>
  );

  return (
    <Card className='wallet-recharge-card !rounded-2xl shadow-sm border-0'>
      <div className='wallet-recharge-heading'>
        <div className='flex items-center'>
          <Avatar size='small' color='blue' className='mr-3 shadow-md'>
            <CreditCard size={16} />
          </Avatar>
          <div>
            <Typography.Text className='text-lg font-medium'>
              {t('账户充值')}
            </Typography.Text>
            <div className='text-xs'>{t('多种充值方式，安全便捷')}</div>
          </div>
        </div>
        <Space>
          <Button
            icon={<Receipt size={16} />}
            theme='outline'
            onClick={onOpenInvoices}
          >
            {t('申请发票')}
          </Button>
          <Button
            icon={<Receipt size={16} />}
            theme='solid'
            onClick={onOpenHistory}
          >
            {t('账单')}
          </Button>
        </Space>
      </div>
      <Card className='!rounded-xl w-full' cover={overview}>
        {shouldShowTabs ? (
          <Tabs type='card' activeKey={activeTab} onChange={setActiveTab}>
            <TabPane
              tab={
                <div className='flex items-center gap-2'>
                  <Wallet size={16} />
                  {t('额度充值')}
                </div>
              }
              itemKey='topup'
            >
              <div className='py-2'>{topupContent}</div>
            </TabPane>
            {shouldShowSubscription ? (
              <TabPane
                tab={
                  <div className='flex items-center gap-2'>
                    <Sparkles size={16} />
                    {t('订阅套餐')}
                  </div>
                }
                itemKey='subscription'
              >
                <div className='py-2'>
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
                </div>
              </TabPane>
            ) : null}
          </Tabs>
        ) : (
          topupContent
        )}
      </Card>
    </Card>
  );
};

export default RechargeCard;
