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
import {
  Banner,
  Modal,
  Typography,
  Card,
  Button,
  Select,
  Divider,
  Tooltip,
} from '@douyinfe/semi-ui';
import { Crown, CalendarClock, Package } from 'lucide-react';
import { SiStripe } from 'react-icons/si';
import { IconCreditCard, IconInfoCircle } from '@douyinfe/semi-icons';
import { renderQuota } from '../../../helpers';
import { getPaymentCurrencySymbol } from '../../../helpers/render';
import { getEpayCryptoMethod } from '../../../helpers/epayMethods';
import CryptoPaymentNotice from '../CryptoPaymentNotice';
import {
  formatSubscriptionDuration,
  formatSubscriptionResetPeriod,
} from '../../../helpers/subscriptionFormat';

const { Text } = Typography;

const SubscriptionPurchaseModal = ({
  t,
  visible,
  onCancel,
  selectedPlan,
  paying,
  selectedEpayMethod,
  setSelectedEpayMethod,
  epayMethods = [],
  enableOnlineTopUp = false,
  enableStripeTopUp = false,
  enableCreemTopUp = false,
  purchaseMode = 'stack',
  onChangePurchaseMode,
  purchaseQuantity = 1,
  onChangePurchaseQuantity,
  purchaseQuantityRange = { min: 1, max: 12 },
  canRenew = false,
  renewableSubscriptions = [],
  renewTargetSubscriptionId = 0,
  onChangeRenewTargetSubscriptionId,
  purchaseLimitInfo = null,
  onPayStripe,
  onPayCreem,
  onPayEpay,
}) => {
  const plan = selectedPlan?.plan;
  const totalAmount = Number(plan?.total_amount || 0);
  const symbol = getPaymentCurrencySymbol();
  const minPurchaseQuantity = Math.max(
    1,
    Number(purchaseQuantityRange?.min || 1),
  );
  const maxPurchaseQuantity = Math.max(
    0,
    Number(purchaseQuantityRange?.max || 0),
  );
  const hasPurchasableQuantity = maxPurchaseQuantity >= minPurchaseQuantity;
  const normalizedPurchaseQuantity = hasPurchasableQuantity
    ? Math.min(
        maxPurchaseQuantity,
        Math.max(minPurchaseQuantity, Number(purchaseQuantity || minPurchaseQuantity)),
      )
    : 0;
  const price = plan ? Number(plan.price_amount || 0) : 0;
  const convertedPrice = price * normalizedPurchaseQuantity;
  const displayPrice = convertedPrice.toFixed(
    Number.isInteger(convertedPrice) ? 0 : 2,
  );
  // 仅当网关启用且套餐存在对应支付 ID 时显示。
  const hasStripe = enableStripeTopUp && !!plan?.stripe_price_id;
  const hasCreem = enableCreemTopUp && !!plan?.creem_product_id;
  const hasEpay = enableOnlineTopUp && epayMethods.length > 0;
  const creemMultiBlocked = normalizedPurchaseQuantity > 1;
  const hasAnyPayment = hasStripe || hasCreem || hasEpay;
  const purchaseLimit = Number(purchaseLimitInfo?.purchase_limit || 0);
  const purchaseCount = Number(purchaseLimitInfo?.purchase_count || 0);
  const stackLimit = Number(purchaseLimitInfo?.stack_limit || 0);
  const stackCount = Number(purchaseLimitInfo?.stack_count || 0);
  const purchaseRemainCount =
    purchaseLimit > 0 ? Math.max(0, purchaseLimit - purchaseCount) : -1;
  const stackRemainCount =
    stackLimit > 0 ? Math.max(0, stackLimit - stackCount) : -1;
  const quantityMax = Number(purchaseLimitInfo?.quantity_max || 0);
  const activeQuantity = Number(purchaseLimitInfo?.active_quantity || 0);
  const quantityRemainCount =
    quantityMax > 0 ? Math.max(0, quantityMax - activeQuantity) : -1;
  const purchaseCountAfterStack = purchaseCount + normalizedPurchaseQuantity;
  const stackCountAfterStack = stackCount + normalizedPurchaseQuantity;
  const normalizedMode =
    purchaseMode === 'renew' || purchaseMode === 'renew_extend' || purchaseMode === 'stack'
      ? purchaseMode
      : 'stack';
  const renewMode = normalizedMode === 'renew';
  const renewExtendMode = normalizedMode === 'renew_extend';
  const allowRenewExtend = !canRenew && normalizedPurchaseQuantity > 1;
  const normalizedRenewableSubscriptions = Array.isArray(renewableSubscriptions)
    ? renewableSubscriptions
    : [];
  // 续费目标选项：仅同套餐可续费订阅（父组件已完成过滤/排序）。
  const renewableSubscriptionOptions = normalizedRenewableSubscriptions
    .map((sub) => {
      const id = Number(sub?.id || 0);
      if (id <= 0) return null;
      const endTimeUnix = Number(sub?.end_time || 0);
      const endTimeText =
        endTimeUnix > 0 ? new Date(endTimeUnix * 1000).toLocaleString() : '-';
      const remainingDays =
        endTimeUnix > 0
          ? Math.max(0, Math.ceil((endTimeUnix - Date.now() / 1000) / 86400))
          : 0;
      return {
        value: id,
        label: `${t('订阅')} #${id} · ${t('至')} ${endTimeText} · ${t('剩余')} ${remainingDays}${t('天')}`,
      };
    })
    .filter(Boolean);
  const hasMultipleRenewableSubscriptions = renewableSubscriptionOptions.length > 1;
  const normalizedRenewTargetSubscriptionId = Number(
    renewTargetSubscriptionId || renewableSubscriptionOptions[0]?.value || 0,
  );
  const selectedRenewTargetOption =
    renewableSubscriptionOptions.find(
      (option) => Number(option.value) === normalizedRenewTargetSubscriptionId,
    ) || renewableSubscriptionOptions[0];
  // 仅在 stack 模式校验购买/叠加上限；续费模式不受叠加上限约束。
  const purchaseLimitBlocked =
    purchaseLimit > 0 && purchaseCountAfterStack > purchaseLimit && !renewMode;
  const stackLimitBlocked =
    stackLimit > 0 && stackCountAfterStack > stackLimit && !renewMode;
  const quantityRangeBlocked = !hasPurchasableQuantity;
  const limitBlocked =
    quantityRangeBlocked || purchaseLimitBlocked || stackLimitBlocked;
  const purchaseQuantityOptions = hasPurchasableQuantity
    ? Array.from(
        { length: maxPurchaseQuantity - minPurchaseQuantity + 1 },
        (_, i) => {
          const value = minPurchaseQuantity + i;
          return { value, label: `${value}` };
        },
      )
    : [];
  const purchaseModeOptions = [
    { value: 'stack', label: t('新购并叠加') },
    canRenew
      ? { value: 'renew', label: t('续费当前套餐') }
      : allowRenewExtend
      ? {
          // 复用续费入口语义：当无可续费目标时，顺延单条订阅。
          value: 'renew_extend',
          label: t('续费式购买（无可续费订阅时自动顺延）'),
        }
      : {
          value: 'renew',
          label: `${t('续费当前套餐')} (${t('当前无可续费订阅')})`,
          disabled: true,
        },
  ];
  const stackMultiHint = !renewMode && !renewExtendMode && normalizedPurchaseQuantity > 1;

  return (
    <Modal
      title={
        <div className='flex items-center'>
          <Crown className='mr-2' size={18} />
          {t('购买订阅套餐')}
        </div>
      }
      visible={visible}
      onCancel={onCancel}
      footer={null}
      size='small'
      centered
    >
      {plan ? (
        <div className='space-y-4 pb-10'>
          {/* 套餐信息 */}
          <Card className='!rounded-xl !border-0 bg-slate-50 dark:bg-slate-800'>
            <div className='space-y-3'>
              <div className='flex justify-between items-center'>
                <Text strong className='text-slate-700 dark:text-slate-200'>
                  {t('套餐名称')}：
                </Text>
                <Typography.Text
                  ellipsis={{ rows: 1, showTooltip: true }}
                  className='text-slate-900 dark:text-slate-100'
                  style={{ maxWidth: 200 }}
                >
                  {plan.title}
                </Typography.Text>
              </div>
              <div className='flex justify-between items-center'>
                <Text strong className='text-slate-700 dark:text-slate-200'>
                  {t('有效期')}：
                </Text>
                <div className='flex items-center'>
                  <CalendarClock size={14} className='mr-1 text-slate-500' />
                  <Text className='text-slate-900 dark:text-slate-100'>
                    {formatSubscriptionDuration(plan, t)}
                  </Text>
                </div>
              </div>
              {formatSubscriptionResetPeriod(plan, t) !== t('不重置') && (
                <div className='flex justify-between items-center'>
                  <Text strong className='text-slate-700 dark:text-slate-200'>
                    {t('重置周期')}：
                  </Text>
                  <Text className='text-slate-900 dark:text-slate-100'>
                    {formatSubscriptionResetPeriod(plan, t)}
                  </Text>
                </div>
              )}
              <div className='flex justify-between items-center'>
                <Text strong className='text-slate-700 dark:text-slate-200'>
                  {t('总额度')}：
                </Text>
                <div className='flex items-center'>
                  <Package size={14} className='mr-1 text-slate-500' />
                  {totalAmount > 0 ? (
                    <Tooltip content={`${t('原生额度')}：${totalAmount}`}>
                      <Text className='text-slate-900 dark:text-slate-100'>
                        {renderQuota(totalAmount)}
                      </Text>
                    </Tooltip>
                  ) : (
                    <Text className='text-slate-900 dark:text-slate-100'>
                      {t('不限')}
                    </Text>
                  )}
                </div>
              </div>
              {plan?.upgrade_group ? (
                <div className='flex justify-between items-center'>
                  <Text strong className='text-slate-700 dark:text-slate-200'>
                    {t('升级分组')}：
                  </Text>
                  <Text className='text-slate-900 dark:text-slate-100'>
                    {plan.upgrade_group}
                  </Text>
                </div>
              ) : null}
              <Divider margin={8} />
              <div className='flex justify-between items-center'>
                <Text strong className='text-slate-700 dark:text-slate-200'>
                  {t('应付金额')}：
                </Text>
                <Text strong className='text-xl text-purple-600'>
                  {symbol}
                  {displayPrice}
                </Text>
              </div>
            </div>
          </Card>

          {/* 购买设置：将数量与模式放在同一区域，便于理解。 */}
          <Card className='!rounded-xl !border-0 bg-slate-50 dark:bg-slate-800'>
            <div className='space-y-2.5'>
              <div className='flex items-center gap-2'>
                <div className='text-sm text-slate-500 dark:text-slate-300 whitespace-nowrap'>
                  {t('购买数量')}：
                </div>
                <Select
                  value={normalizedPurchaseQuantity}
                  onChange={(value) =>
                    onChangePurchaseQuantity?.(
                      Math.min(
                        maxPurchaseQuantity,
                        Math.max(
                          minPurchaseQuantity,
                          Number(value || minPurchaseQuantity),
                        ),
                      ),
                    )
                  }
                  style={{ flex: 1, minWidth: 0 }}
                  optionList={purchaseQuantityOptions}
                  disabled={!hasPurchasableQuantity}
                />
              </div>
              <div className='flex items-center gap-2'>
                <div className='text-sm text-slate-500 dark:text-slate-300 whitespace-nowrap'>
                  {t('购买方式')}：
                </div>
                <Select
                  value={normalizedMode}
                  onChange={onChangePurchaseMode}
                  style={{ flex: 1, minWidth: 0 }}
                  optionList={purchaseModeOptions}
                />
              </div>
              {renewMode && canRenew && hasMultipleRenewableSubscriptions && (
                // 同一套餐存在多条订阅时，允许手动选择续费目标。
                <div className='flex items-center gap-2'>
                  <div className='text-sm text-slate-500 dark:text-slate-300 whitespace-nowrap'>
                    {t('续费目标')}：
                  </div>
                  <Select
                    value={normalizedRenewTargetSubscriptionId}
                    onChange={(value) =>
                      onChangeRenewTargetSubscriptionId?.(Number(value || 0))
                    }
                    style={{ flex: 1, minWidth: 0 }}
                    optionList={renewableSubscriptionOptions}
                  />
                </div>
              )}
              {renewMode &&
                canRenew &&
                !hasMultipleRenewableSubscriptions &&
                selectedRenewTargetOption && (
                  // 仅有一条可续费订阅时，直接展示并跳过额外选择。
                  <div className='text-xs text-slate-500 dark:text-slate-400'>
                    {t('续费目标')}：{selectedRenewTargetOption.label}
                  </div>
                )}
              {/* 说明文案单独占行，减少视觉噪音。 */}
              <div className='flex items-center gap-1.5 text-xs text-slate-500 dark:text-slate-400'>
                <IconInfoCircle size={14} />
                <span>
                  {renewMode
                    ? t('续费仅支持同规格套餐（同 plan）')
                    : renewExtendMode
                    ? t('当前无可续费订阅，本次将按单条顺延（等价一次性购买多个周期）')
                    : stackMultiHint
                    ? t('新购多份会创建多条订阅记录')
                    : t('新购将创建新的订阅记录')}
                </span>
              </div>
              {quantityMax > 0 && (
                <div className='text-xs text-slate-500 dark:text-slate-400'>
                  {t('最大购买数量')}：{quantityMax}
                  {t('份')} ({t('剩余')}
                  {quantityRemainCount}
                  {t('份')})
                </div>
              )}
              {purchaseLimit > 0 && (
                <div className='text-xs text-slate-500 dark:text-slate-400'>
                  {t('购买上限')}：{purchaseLimit}
                  {t('份')} ({t('剩余')}
                  {purchaseRemainCount}
                  {t('份')})
                </div>
              )}
              {stackLimit > 0 && (
                <div className='text-xs text-slate-500 dark:text-slate-400'>
                  {t('叠加上限')}：{stackLimit}
                  {t('份')} ({t('剩余')}
                  {stackRemainCount}
                  {t('份')})
                </div>
              )}
              {!hasPurchasableQuantity && (
                <div className='text-xs text-slate-500 dark:text-slate-400'>
                  {t('当前可购买数量为 0，请等待部分订阅到期后再试')}
                </div>
              )}
              {!canRenew && renewMode && (
                <div className='text-xs text-slate-500 dark:text-slate-400'>
                  {t('当前无可续费订阅')}
                </div>
              )}
            </div>
          </Card>

          {/* 支付方式 */}
          {limitBlocked && (
            <Banner
              type='warning'
              description={
                quantityRangeBlocked
                  ? t('当前可购买数量为 0，请等待部分订阅到期后再试')
                  : purchaseLimitBlocked
                  ? `${t('已达到购买上限')} (${purchaseCountAfterStack}/${purchaseLimit})`
                  : `${t('已达到叠加上限')} (${stackCountAfterStack}/${stackLimit})`
              }
              className='!rounded-xl'
              closeIcon={null}
            />
          )}

          {hasAnyPayment ? (
            <div className='space-y-3'>
              <Text size='small' type='tertiary'>
                {t('选择支付方式')}：
              </Text>

              {/* Stripe / Creem */}
              {(hasStripe || hasCreem) && (
                <div className='flex flex-wrap gap-2'>
                  {hasStripe && (
                    <Button
                      theme='light'
                      className='flex-1'
                      icon={<SiStripe size={14} color='#635BFF' />}
                      onClick={() => onPayStripe(false)}
                      loading={paying}
                      disabled={limitBlocked}
                    >
                      Stripe · {t('单次支付')}
                    </Button>
                  )}
                  {hasStripe && plan?.stripe_auto_renew && normalizedMode === 'stack' && (
                    <Button theme='light' className='flex-1' onClick={() => onPayStripe(true)}
                      loading={paying} disabled={limitBlocked}>
                      Stripe · {t('自动续费')}
                    </Button>
                  )}
                  {hasCreem && (
                    <Button
                      theme='light'
                      className='flex-1'
                      icon={<IconCreditCard />}
                      onClick={onPayCreem}
                      loading={paying}
                      disabled={limitBlocked || creemMultiBlocked}
                    >
                      Creem
                    </Button>
                  )}
                </div>
              )}
              {hasCreem && creemMultiBlocked && (
                <Text size='small' type='tertiary'>
                  {t('Creem 当前仅支持单次购买 1 份，如需多份请分次支付')}
                </Text>
              )}

              {/* EPay */}
              {hasEpay && (
                <div className='flex gap-2'>
                  <Select
                    value={selectedEpayMethod}
                    onChange={setSelectedEpayMethod}
                    style={{ flex: 1 }}
                    size='default'
                    placeholder={t('选择支付方式')}
                    optionList={epayMethods.map((m) => ({
                      value: m.type,
                      label: m.name || m.type,
                    }))}
                    disabled={limitBlocked}
                  />
                  <Button
                    theme='solid'
                    type='primary'
                    onClick={onPayEpay}
                    loading={paying}
                    disabled={!selectedEpayMethod || limitBlocked}
                  >
                    {t('支付')}
                  </Button>
                </div>
              )}
              {hasEpay && getEpayCryptoMethod(selectedEpayMethod) && (
                <CryptoPaymentNotice method={selectedEpayMethod} />
              )}
            </div>
          ) : (
            <Banner
              type='info'
              description={t('管理员未开启在线支付功能，请联系管理员配置。')}
              className='!rounded-xl'
              closeIcon={null}
            />
          )}
        </div>
      ) : null}
    </Modal>
  );
};

export default SubscriptionPurchaseModal;
